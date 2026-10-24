package postgres

import (
    "context"
    "os"
    "testing"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
)

// Requires schema migrations through 000007 and an isolated TEST_DATABASE_URL.
// These checks are intentionally at the SQL boundary, not a mock store.
func TestResourceRevisionMigration(t *testing.T) {
    url := os.Getenv("TEST_DATABASE_URL")
    if url == "" { t.Skip("TEST_DATABASE_URL not set") }
    ctx := context.Background()
    db, err := pgxpool.New(ctx, url)
    if err != nil { t.Fatal(err) }
    defer db.Close()

    owner := uuid.New()
    _, err = db.Exec(ctx, `INSERT INTO users(id, oidc_subject, display_name)
        VALUES ($1, $2, $3)`, owner, "revision-"+owner.String(), "Revision Tester")
    if err != nil { t.Fatal(err) }
    defer db.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner)

    s := New(db)
    if _, err := s.Profile(ctx, owner); err != nil { t.Fatal(err) }

    read := func() (int64, int64) {
        t.Helper()
        var profile, privacy int64
        if err := db.QueryRow(ctx, `SELECT content_revision, privacy_revision
            FROM profiles WHERE user_id=$1`, owner).Scan(&profile, &privacy); err != nil { t.Fatal(err) }
        return profile, privacy
    }

    originalContent, originalPrivacy := read()
    if originalContent != 1 || originalPrivacy != 1 {
        t.Fatalf("new profile revisions should initialize to 1: %d %d", originalContent, originalPrivacy)
    }

    // Professional data only: increments content, not privacy.
    if _, err := db.Exec(ctx, `UPDATE profiles SET headline='Revision updated' WHERE user_id=$1`, owner); err != nil { t.Fatal(err) }
    content, privacy := read()
    if content != originalContent+1 || privacy != originalPrivacy {
        t.Fatalf("content bump: content=%d privacy=%d", content, privacy)
    }

    // Privacy flag only: increments shared privacy counter.
    if _, err := db.Exec(ctx, `UPDATE profiles SET show_bio=true WHERE user_id=$1`, owner); err != nil { t.Fatal(err) }
    content, privacy = read()
    if content != originalContent+1 || privacy != originalPrivacy+1 {
        t.Fatalf("privacy bump: content=%d privacy=%d", content, privacy)
    }

    // A no-op update MUST NOT increment either content or privacy.
    if _, err := db.Exec(ctx, `UPDATE profiles SET updated_at=now() WHERE user_id=$1`, owner); err != nil { t.Fatal(err) }
    unchangedContent, unchangedPrivacy := read()
    if unchangedContent != content || unchangedPrivacy != privacy {
        t.Fatalf("no-op bump: content=%d privacy=%d", unchangedContent, unchangedPrivacy)
    }

    // Existing M4 PRIVATE-withdrawal trigger and new revision trigger coexist.
    if _, err := db.Exec(ctx, `UPDATE profiles SET visibility='public' WHERE user_id=$1`, owner); err != nil { t.Fatal(err) }
    if _, err := db.Exec(ctx, `UPDATE profiles SET directory_discoverable=true,
        directory_consented_at=now() WHERE user_id=$1`, owner); err != nil { t.Fatal(err) }
    _, beforePrivate := read()
    if _, err := db.Exec(ctx, `UPDATE profiles SET visibility='private' WHERE user_id=$1`, owner); err != nil { t.Fatal(err) }
    var visible string
    var listed bool
    var consented *string
    err = db.QueryRow(ctx, `SELECT visibility, directory_discoverable,
        directory_consented_at::text FROM profiles WHERE user_id=$1`, owner).Scan(&visible, &listed, &consented)
    if err != nil { t.Fatal(err) }
    _, afterPrivate := read()
    if visible != "private" || listed || consented != nil || afterPrivate <= beforePrivate {
        t.Fatalf("withdrawal not revisioned atomically: visibility=%s discoverable=%v from=%d to=%d", visible, listed, beforePrivate, afterPrivate)
    }

    // Per-item revisions follow successful updates and roll back on SQL errors.
    experience := uuid.New()
    _, err = db.Exec(ctx, `INSERT INTO profile_experiences(id,user_id,company,title,start_date)
        VALUES($1,$2,'Test Co','Engineer','2024-01-01')`, experience, owner)
    if err != nil { t.Fatal(err) }
    var rev int64
    if err = db.QueryRow(ctx, `SELECT revision FROM profile_experiences WHERE id=$1`, experience).Scan(&rev); err != nil { t.Fatal(err) }
    if rev != 1 { t.Fatalf("experience initial revision=%d", rev) }
    if _, err = db.Exec(ctx, `UPDATE profile_experiences SET company='Updated Co' WHERE id=$1`, experience); err != nil { t.Fatal(err) }
    if err = db.QueryRow(ctx, `SELECT revision FROM profile_experiences WHERE id=$1`, experience).Scan(&rev); err != nil { t.Fatal(err) }
    if rev != 2 { t.Fatalf("experience revision=%d", rev) }

    // Ensure a rejected statement does not alter a revision.
    if _, err = db.Exec(ctx, `UPDATE profile_experiences SET end_date='2020-01-01' WHERE id=$1`, experience); err == nil {
        t.Fatal("invalid experience update unexpectedly accepted")
    }
    if err = db.QueryRow(ctx, `SELECT revision FROM profile_experiences WHERE id=$1`, experience).Scan(&rev); err != nil { t.Fatal(err) }
    if rev != 2 { t.Fatalf("failed statement changed revision: %d", rev) }

    education := uuid.New()
    if _, err := db.Exec(ctx, `INSERT INTO profile_educations(id,user_id,institution_name)
        VALUES($1,$2,'Test University')`, education, owner); err != nil { t.Fatal(err) }
    if _, err := db.Exec(ctx, `UPDATE profile_educations SET degree='B.Sc.' WHERE id=$1`, education); err != nil { t.Fatal(err) }
    if err := db.QueryRow(ctx, `SELECT revision FROM profile_educations WHERE id=$1`, education).Scan(&rev); err != nil { t.Fatal(err) }
    if rev != 2 { t.Fatalf("education revision=%d", rev) }
}
