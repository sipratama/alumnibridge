package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
)

type Store struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Store { return &Store{db: db} }
func prefix(s string) string {
	if s == "" {
		return ""
	}
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s) + "%"
}

const searchSQL = `WITH page AS MATERIALIZED (
 SELECT p.user_id,p.slug,p.headline,p.current_company,p.current_job_title,
 p.show_current_company,p.show_current_job_title,p.show_skills,p.show_photo,
 (p.photo_object_key IS NOT NULL) AS has_photo,COALESCE(u.display_name,'') AS display_name
 FROM profiles p JOIN users u ON u.id=p.user_id
 WHERE p.visibility='public' AND p.moderation_status='visible' AND p.directory_discoverable
 AND EXISTS(SELECT 1 FROM memberships WHERE user_id=p.user_id AND role='alumni' AND status='verified')
 AND ($1='' OR lower(regexp_replace(btrim(COALESCE(u.display_name,'')),'[[:space:]]+',' ','g')) LIKE $1 ESCAPE '\')
 AND ($2='' OR CASE WHEN p.show_current_company THEN lower(regexp_replace(btrim(p.current_company),'[[:space:]]+',' ','g')) LIKE $2 ESCAPE '\' ELSE false END)
 AND ($3='' OR CASE WHEN p.show_current_job_title THEN lower(regexp_replace(btrim(p.current_job_title),'[[:space:]]+',' ','g')) LIKE $3 ESCAPE '\' ELSE false END)
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

func (s *Store) Search(ctx context.Context, spec directory.Spec) ([]directory.Entry, error) {
	spec, err := directory.Normalize(spec)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, searchSQL, prefix(spec.Query), prefix(spec.Company), prefix(spec.JobTitle), spec.Skills,
		spec.Institution, spec.StudyProgram, spec.GraduationYear, spec.After, spec.Limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]directory.Entry, 0, spec.Limit+1)
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
	return out, rows.Err()
}
