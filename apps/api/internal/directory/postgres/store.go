package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
)

type Store struct {
	db               *pgxpool.Pool
	statementTimeout time.Duration
}

func New(db *pgxpool.Pool) *Store { return NewWithTimeout(db, 1500*time.Millisecond) }
func NewWithTimeout(db *pgxpool.Pool, timeout time.Duration) *Store {
	return &Store{db: db, statementTimeout: timeout}
}
func prefix(s string) string {
	if s == "" {
		return ""
	}
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s) + "%"
}

// OFFSET 0 keeps the eligible subquery boundary without materializing every
// candidate. Its C ordering lets the outer bounded page stop early. PostgreSQL
// 17 plans and privacy regressions are recorded in the Phase C assessment.
const searchSQL = `WITH eligible AS NOT MATERIALIZED (
 SELECT p.user_id,p.slug,p.headline,p.current_company,p.current_job_title,
 p.show_current_company,p.show_current_job_title,p.show_skills,p.show_photo,
 (p.photo_object_key IS NOT NULL) AS has_photo,COALESCE(u.display_name,'') AS display_name
 FROM profiles p
 JOIN LATERAL (SELECT display_name FROM users WHERE id=p.user_id OFFSET 0) u ON true
 JOIN LATERAL (SELECT 1 FROM memberships WHERE user_id=p.user_id AND role='alumni' AND status='verified' LIMIT 1) verified ON true
 WHERE p.visibility='public' AND p.moderation_status='visible' AND p.directory_discoverable
 AND p.slug COLLATE "C">$8::text COLLATE "C"
 ORDER BY p.slug COLLATE "C" OFFSET 0
), page AS MATERIALIZED (
 SELECT * FROM eligible p WHERE true
 AND ($1='' OR btrim(regexp_replace(CASE WHEN p.display_name !~ '[^\x01-\x7F]' THEN translate(p.display_name,E'ABCDEFGHIJKLMNOPQRSTUVWXYZ\t\n\r\f\013','abcdefghijklmnopqrstuvwxyz     ') ELSE translate(p.display_name,$10,$11) END,' +',' ','g')) COLLATE "C" LIKE $1 ESCAPE '\')
 AND ($2='' OR CASE WHEN p.show_current_company THEN btrim(regexp_replace(CASE WHEN p.current_company !~ '[^\x01-\x7F]' THEN translate(p.current_company,E'ABCDEFGHIJKLMNOPQRSTUVWXYZ\t\n\r\f\013','abcdefghijklmnopqrstuvwxyz     ') ELSE translate(p.current_company,$10,$11) END,' +',' ','g')) COLLATE "C" LIKE $2 ESCAPE '\' ELSE false END)
 AND ($3='' OR CASE WHEN p.show_current_job_title THEN btrim(regexp_replace(CASE WHEN p.current_job_title !~ '[^\x01-\x7F]' THEN translate(p.current_job_title,E'ABCDEFGHIJKLMNOPQRSTUVWXYZ\t\n\r\f\013','abcdefghijklmnopqrstuvwxyz     ') ELSE translate(p.current_job_title,$10,$11) END,' +',' ','g')) COLLATE "C" LIKE $3 ESCAPE '\' ELSE false END)
 AND (cardinality($4::text[])=0 OR (p.show_skills AND NOT EXISTS (
    SELECT 1 FROM unnest($4::text[]) wanted(name) WHERE NOT EXISTS (
      SELECT 1 FROM profile_skills ps JOIN skills s ON s.id=ps.skill_id
      WHERE ps.user_id=p.user_id AND s.normalized_name=wanted.name))))
 AND (($5='' AND $6='' AND $7::integer=0) OR EXISTS (
    SELECT 1 FROM memberships m JOIN study_programs sp ON sp.id=m.study_program_id
    JOIN institutions i ON i.id=sp.institution_id JOIN profile_directory_affiliations a ON a.membership_id=m.id
    WHERE m.user_id=p.user_id AND m.role='alumni' AND m.status='verified'
    AND ($5='' OR (a.show_institution AND i.slug=$5))
    AND ($6='' OR (a.show_study_program AND sp.slug=$6))
    AND ($7::integer=0 OR (a.show_graduation_year AND m.graduation_year=$7))))
 AND p.slug COLLATE "C">$8::text COLLATE "C"
 ORDER BY p.slug COLLATE "C" LIMIT $9
)
SELECT jsonb_strip_nulls(jsonb_build_object(
 'slug',p.slug,'display_name',p.display_name,'headline',p.headline,'verified_alumni',true,
 'current_company',CASE WHEN p.show_current_company THEN p.current_company END,
 'current_job_title',CASE WHEN p.show_current_job_title THEN p.current_job_title END,
 'photo_url',CASE WHEN p.show_photo AND p.has_photo THEN '/api/v1/profiles/'||p.slug||'/photo' END,
 'skills',CASE WHEN p.show_skills THEN (SELECT COALESCE(jsonb_agg(s.name ORDER BY s.normalized_name COLLATE "C",s.id),'[]') FROM
    (SELECT s.name,s.normalized_name,s.id FROM profile_skills ps JOIN skills s ON s.id=ps.skill_id
     WHERE ps.user_id=p.user_id ORDER BY s.normalized_name COLLATE "C",s.id LIMIT 100) s) END,
 'affiliations',(SELECT jsonb_agg(a.data ORDER BY a.institution COLLATE "C",a.program COLLATE "C",a.grad_year) FROM (
    SELECT i.slug institution,sp.slug program,m.graduation_year AS grad_year,jsonb_strip_nulls(jsonb_build_object(
      'institution',jsonb_build_object('slug',i.slug,'name',i.name),
      'study_program',CASE WHEN a.show_study_program THEN jsonb_build_object('slug',sp.slug,'name',sp.name) END,
      'graduation_year',CASE WHEN a.show_graduation_year THEN m.graduation_year END)) data
    FROM memberships m JOIN study_programs sp ON sp.id=m.study_program_id JOIN institutions i ON i.id=sp.institution_id
    JOIN profile_directory_affiliations a ON a.membership_id=m.id
    WHERE m.user_id=p.user_id AND m.role='alumni' AND m.status='verified' AND a.show_institution
    ORDER BY i.slug COLLATE "C",sp.slug COLLATE "C",m.graduation_year LIMIT 20) a)
)) FROM page p ORDER BY p.slug COLLATE "C"`

func (s *Store) Search(ctx context.Context, spec directory.Spec) (out []directory.Entry, err error) {
	defer func() {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.Code == "57014" && ctx.Err() == nil {
			err = directory.ErrDatabaseTimeout
		}
	}()
	spec, err = directory.Normalize(spec)
	if err != nil {
		return nil, err
	}
	// SET LOCAL never contaminates pooled connections. Rollback uses a separate,
	// short cleanup budget even when the caller has disconnected.
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if _, err = tx.Exec(ctx, `SELECT set_config('statement_timeout',$1,true)`, strconv.FormatInt(max(1, s.statementTimeout.Milliseconds()), 10)); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, searchSQL, prefix(spec.Query), prefix(spec.Company), prefix(spec.JobTitle), spec.Skills,
		spec.Institution, spec.StudyProgram, spec.GraduationYear, spec.After, spec.Limit+1, normalizationFrom, normalizationTo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out = make([]directory.Entry, 0, spec.Limit+1)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item directory.Entry
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
