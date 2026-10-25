package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func membershipSQLState(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

func TestTrustedMembershipVersusLifecycleWithdrawal(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	t.Run("withdrawal_first_denies_membership_write", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		f := newLifecycleFixture(t, ctx, url)
		program := uuid.New()
		if _, err := f.db.Exec(ctx, `INSERT INTO study_programs(id,institution_id,slug,name)
			SELECT $1,institution_id,$2,'Lifecycle fixture' FROM study_programs LIMIT 1`, program, "lifecycle-"+program.String()); err != nil {
			t.Fatal(err)
		}
		defer f.db.Exec(context.Background(), `DELETE FROM study_programs WHERE id=$1`, program)
		if err := commitSyntheticWithdrawal(ctx, f.db, f); err != nil {
			t.Fatal(err)
		}
		var revisionBefore int64
		if err := f.db.QueryRow(ctx, `SELECT privacy_revision FROM profiles WHERE user_id=$1`, f.owner).Scan(&revisionBefore); err != nil {
			t.Fatal(err)
		}
		_, err := f.db.Exec(ctx, `UPDATE memberships SET graduation_year=2021 WHERE user_id=$1`, f.owner)
		if !membershipSQLState(err, "23514") {
			t.Fatalf("membership write after deleting status: %v; want 23514", err)
		}
		_, err = f.db.Exec(ctx, `INSERT INTO memberships(user_id,study_program_id,role,status)
			SELECT $1,id,'student','active' FROM study_programs WHERE slug='mathematics-education' LIMIT 1`, f.owner)
		if !membershipSQLState(err, "23514") {
			t.Fatalf("membership insert after deleting status: %v; want 23514", err)
		}
		_, err = f.db.Exec(ctx, `INSERT INTO memberships(user_id,study_program_id,role,status,graduation_year)
			VALUES($1,$2,'alumni','verified',2022)`, f.owner, program)
		if !membershipSQLState(err, "23514") {
			t.Fatalf("verified insertion after deleting status: %v; want 23514", err)
		}
		_, err = f.db.Exec(ctx, `DELETE FROM memberships WHERE user_id=$1`, f.owner)
		if !membershipSQLState(err, "23514") {
			t.Fatalf("membership delete after deleting status: %v; want 23514", err)
		}
		var year int
		if err := f.db.QueryRow(ctx, `SELECT graduation_year FROM memberships WHERE user_id=$1`, f.owner).Scan(&year); err != nil || year != 2020 {
			t.Fatalf("denied membership changed: year=%d err=%v", year, err)
		}
		var revisionAfter int64
		if err := f.db.QueryRow(ctx, `SELECT privacy_revision FROM profiles WHERE user_id=$1`, f.owner).Scan(&revisionAfter); err != nil || revisionAfter != revisionBefore {
			t.Fatalf("denied write advanced privacy revision: %d -> %d err=%v", revisionBefore, revisionAfter, err)
		}
		assertLifecycleWithdrawal(t, ctx, f, f.content)
		// User deletion still cascades through memberships after the parent row
		// disappears inside the deleting transaction.
		if _, err := f.db.Exec(ctx, `DELETE FROM users WHERE id=$1`, f.owner); err != nil {
			t.Fatalf("account cascade after guard: %v", err)
		}
	})

	t.Run("membership_first_serializes_before_withdrawal", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		f := newLifecycleFixture(t, ctx, url)
		writerDB, _ := oneConnection(t, ctx, url)
		transitionDB, transitionPID := oneConnection(t, ctx, url)
		// The existing application onboarding writer already uses this parent-first order.
		writer, err := writerDB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer writer.Rollback(ctx)
		if _, err := writer.Exec(ctx, `SELECT id FROM users WHERE id=$1 AND access_status='active' FOR UPDATE`, f.owner); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Exec(ctx, `UPDATE memberships SET graduation_year=2021 WHERE user_id=$1`, f.owner); err != nil {
			t.Fatal(err)
		}
		transitionDone := make(chan error, 1)
		go func() { transitionDone <- commitSyntheticWithdrawal(ctx, transitionDB, f) }()
		waitForBackendLock(t, ctx, f.db, transitionPID)
		if err := writer.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-transitionDone; err != nil {
			t.Fatal(err)
		}
		var year int
		if err := f.db.QueryRow(ctx, `SELECT graduation_year FROM memberships WHERE user_id=$1`, f.owner).Scan(&year); err != nil || year != 2021 {
			t.Fatalf("serialized membership year=%d err=%v", year, err)
		}
		assertLifecycleWithdrawal(t, ctx, f, f.content)
	})

	t.Run("membership_first_without_parent_lock_deadlocks_and_full_retry_denies", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		f := newLifecycleFixture(t, ctx, url)
		writerDB, _ := oneConnection(t, ctx, url)
		transitionDB, transitionPID := oneConnection(t, ctx, url)
		writer, err := writerDB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer writer.Rollback(ctx)
		if _, err := writer.Exec(ctx, `SET LOCAL deadlock_timeout='100ms'`); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Exec(ctx, `SELECT id FROM memberships WHERE user_id=$1 FOR UPDATE`, f.owner); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Exec(ctx, `SELECT user_id FROM profiles WHERE user_id=$1 FOR UPDATE`, f.owner); err != nil {
			t.Fatal(err)
		}
		transitionDone := make(chan error, 1)
		go func() { transitionDone <- commitSyntheticWithdrawal(ctx, transitionDB, f) }()
		waitForBackendLock(t, ctx, f.db, transitionPID) // Transition owns user and waits for profile.
		_, err = writer.Exec(ctx, `UPDATE memberships SET graduation_year=2021 WHERE user_id=$1`, f.owner)
		if !membershipSQLState(err, "40P01") {
			t.Fatalf("expected membership/user lock inversion 40P01: %v", err)
		}
		if err := writer.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-transitionDone; err != nil {
			t.Fatalf("transition after aborted membership transaction: %v", err)
		}
		// A retry starts a new transaction and re-reads current status. The
		// trigger rejects the write instead of replaying one failed statement.
		retry, err := writerDB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer retry.Rollback(ctx)
		_, err = retry.Exec(ctx, `UPDATE memberships SET graduation_year=2021 WHERE user_id=$1`, f.owner)
		if !membershipSQLState(err, "23514") {
			t.Fatalf("whole-transaction retry after withdrawal: %v; want 23514", err)
		}
		if err := retry.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		var year int
		if err := f.db.QueryRow(ctx, `SELECT graduation_year FROM memberships WHERE user_id=$1`, f.owner).Scan(&year); err != nil || year != 2020 {
			t.Fatalf("deadlock/retry changed membership: year=%d err=%v", year, err)
		}
		assertLifecycleWithdrawal(t, ctx, f, f.content)
	})
}
