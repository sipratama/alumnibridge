package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/identity"
)

type Store struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) Provision(ctx context.Context, subject, email string) (identity.User, error) {
	var user identity.User
	err := s.db.QueryRow(ctx, `INSERT INTO users (oidc_subject, email) VALUES ($1, NULLIF($2, ''))
 ON CONFLICT (oidc_subject) DO UPDATE SET email = COALESCE(EXCLUDED.email, users.email), updated_at = now()
 RETURNING id, COALESCE(email, ''), COALESCE(display_name, '')`, subject, email).Scan(&user.ID, &user.Email, &user.DisplayName)
	if err != nil {
		return identity.User{}, err
	}
	user.Memberships, err = s.memberships(ctx, user.ID)
	return user, err
}

func (s *Store) Onboard(ctx context.Context, userID uuid.UUID, input identity.OnboardingInput) (identity.User, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return identity.User{}, err
	}
	defer tx.Rollback(ctx)

	var lockedID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&lockedID); err != nil {
		return identity.User{}, err
	}

	var programID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT sp.id FROM study_programs sp JOIN institutions i ON i.id = sp.institution_id
 WHERE i.slug = $1 AND sp.slug = $2`, input.Institution, input.StudyProgram).Scan(&programID)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.User{}, identity.ErrProgramNotFound
	}
	if err != nil {
		return identity.User{}, err
	}

	var existingYear *int
	err = tx.QueryRow(ctx, `SELECT graduation_year FROM memberships WHERE user_id=$1 AND study_program_id=$2 AND role=$3`, userID, programID, input.Role).Scan(&existingYear)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return identity.User{}, err
	}
	if err == nil && !sameYear(existingYear, input.GraduationYear) {
		return identity.User{}, identity.ErrMembershipConflict
	}

	_, err = tx.Exec(ctx, `UPDATE users SET display_name=$2, updated_at=now() WHERE id=$1`, userID, input.DisplayName)
	if err != nil {
		return identity.User{}, err
	}
	status := "active"
	if input.Role == "alumni" {
		status = "pending"
	}
	_, err = tx.Exec(ctx, `INSERT INTO memberships (user_id, study_program_id, role, status, graduation_year)
 VALUES ($1,$2,$3,$4,$5) ON CONFLICT (user_id, study_program_id, role) DO NOTHING`, userID, programID, input.Role, status, input.GraduationYear)
	if err != nil {
		return identity.User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return identity.User{}, err
	}
	return s.getByID(ctx, userID)
}

func sameYear(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func (s *Store) getByID(ctx context.Context, id uuid.UUID) (identity.User, error) {
	var user identity.User
	err := s.db.QueryRow(ctx, `SELECT id, COALESCE(email, ''), COALESCE(display_name, '') FROM users WHERE id=$1`, id).Scan(&user.ID, &user.Email, &user.DisplayName)
	if err != nil {
		return identity.User{}, err
	}
	user.Memberships, err = s.memberships(ctx, id)
	return user, err
}

func (s *Store) memberships(ctx context.Context, userID uuid.UUID) ([]identity.Membership, error) {
	rows, err := s.db.Query(ctx, `SELECT m.id, m.role, m.status, i.slug, sp.slug, m.graduation_year
 FROM memberships m JOIN study_programs sp ON sp.id=m.study_program_id JOIN institutions i ON i.id=sp.institution_id
 WHERE m.user_id=$1 ORDER BY m.created_at, m.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]identity.Membership, 0)
	for rows.Next() {
		var m identity.Membership
		if err := rows.Scan(&m.ID, &m.Role, &m.Status, &m.Institution, &m.StudyProgram, &m.GraduationYear); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
