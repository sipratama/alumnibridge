package postgres

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
)

//go:embed testdata/phase_b_search.sql
var phaseBSearchSQL string

// Explicit opt-in: synthetic rows live only in a rolled-back transaction. Use a
// dedicated empty migrated database, never an application database.
func TestDirectoryExplain(t *testing.T) {
	if os.Getenv("DIRECTORY_EXPLAIN") != "1" {
		t.Skip("opt-in synthetic performance assessment")
	}
	db, ctx := reviewDatabase(t)
	var users int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users); err != nil || users != 0 {
		t.Fatal("requires empty disposable database", err)
	}
	for _, size := range []int{1000, 10000, 25000} {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer tx.Rollback(context.Background())
			seedPerformance(t, ctx, tx, size)
			for _, item := range []struct {
				name string
				spec directory.Spec
			}{
				{"browse", directory.Spec{Limit: 20}},
				{"name", directory.Spec{Query: "siti nur", Limit: 20}},
				{"company", directory.Spec{Company: "nusantara", Limit: 20}},
				{"hidden_company", directory.Spec{Company: "secret", Limit: 20}},
				{"absent_company", directory.Spec{Company: "absent", Limit: 20}},
				{"job", directory.Spec{JobTitle: "engineer", Limit: 20}},
				{"academic", directory.Spec{Institution: "uhamka", StudyProgram: "mathematics-education", GraduationYear: 2020, Limit: 20}},
				{"skills", directory.Spec{Skills: []string{"golang"}, Limit: 20}},
				{"combined", directory.Spec{Query: "siti", Company: "nusantara", Skills: []string{"golang"}, Institution: "uhamka", Limit: 20}},
				{"next", directory.Spec{After: "perf-00000500", Limit: 20}},
				{"no_match", directory.Spec{Query: "nonexistent", Limit: 20}},
			} {
				// Isolate eligibility, ASCII normalization, ordered streaming,
				// and the indexed lateral probes used by the shipped query.
				materialized := strings.Replace(searchSQL, "eligible AS NOT MATERIALIZED", "eligible AS MATERIALIZED", 1)
				materialized = strings.Replace(materialized, ` ORDER BY p.slug COLLATE "C" OFFSET 0`, "", 1)
				materialized = strings.Replace(materialized, ` FROM profiles p
 JOIN LATERAL (SELECT display_name FROM users WHERE id=p.user_id OFFSET 0) u ON true
 JOIN LATERAL (SELECT 1 FROM memberships WHERE user_id=p.user_id AND role='alumni' AND status='verified' LIMIT 1) verified ON true`, ` FROM profiles p JOIN users u ON u.id=p.user_id`, 1)
				materialized = strings.Replace(materialized, ` AND p.slug COLLATE "C">$8::text COLLATE "C"`, ` AND EXISTS(SELECT 1 FROM memberships WHERE user_id=p.user_id AND role='alumni' AND status='verified')
 AND p.slug COLLATE "C">$8::text COLLATE "C"`, 1)
				ordered := strings.Replace(materialized, "eligible AS MATERIALIZED", "eligible AS NOT MATERIALIZED", 1)
				ordered = strings.Replace(ordered, "), page AS MATERIALIZED", ` ORDER BY p.slug COLLATE "C" OFFSET 0
), page AS MATERIALIZED`, 1)
				eligibleOnly := materialized
				for _, value := range []string{"p.display_name", "p.current_company", "p.current_job_title"} {
					start := "CASE WHEN " + value + " !~"
					if index := strings.Index(eligibleOnly, start); index >= 0 {
						end := index + strings.Index(eligibleOnly[index:], " END") + 4
						eligibleOnly = eligibleOnly[:index] + "translate(" + value + ",$10,$11)" + eligibleOnly[end:]
					}
				}
				for _, query := range []struct{ name, sql string }{{"before", phaseBSearchSQL}, {"eligible_only", eligibleOnly}, {"ascii_fence", materialized}, {"ordered", ordered}, {"after", searchSQL}} {
					var raw []byte
					if err := tx.QueryRow(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query.sql, searchArgs(t, item.spec)...).Scan(&raw); err != nil {
						t.Fatal(err)
					}
					var plan []map[string]any
					if err := json.Unmarshal(raw, &plan); err != nil {
						t.Fatal(err)
					}
					root := plan[0]
					node := root["Plan"].(map[string]any)
					stats := map[string]any{"size": size, "query": item.name, "variant": query.name, "execution_ms": root["Execution Time"], "planning_ms": root["Planning Time"], "rows": node["Actual Rows"], "hit_blocks": node["Shared Hit Blocks"], "read_blocks": node["Shared Read Blocks"]}
					var scans []map[string]any
					var walk func(map[string]any)
					walk = func(n map[string]any) {
						if n["Relation Name"] != nil || n["Sort Method"] != nil || (n["Node Type"] == "CTE Scan" || n["Node Type"] == "Subquery Scan") {
							scans = append(scans, map[string]any{"node": n["Node Type"], "cte": n["CTE Name"], "relation": n["Relation Name"], "index": n["Index Name"], "rows": n["Actual Rows"], "removed": n["Rows Removed by Filter"], "loops": n["Actual Loops"], "sort": n["Sort Method"], "total_ms": n["Actual Total Time"]})
						}
						if children, ok := n["Plans"].([]any); ok {
							for _, c := range children {
								walk(c.(map[string]any))
							}
						}
					}
					walk(node)
					stats["scans"] = scans
					data, _ := json.Marshal(stats)
					t.Log(string(data))
					if dir := os.Getenv("DIRECTORY_EXPLAIN_OUTPUT"); dir != "" {
						// Operator-selected output directory; no request values enter filenames.
						name := dir + "/" + query.name + "-" + item.name + "-" + jsonNumber(size) + ".json"
						if err := os.WriteFile(name, raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
		}()
	}
}

func jsonNumber(n int) string { b, _ := json.Marshal(n); return string(b) }

func searchArgs(t *testing.T, s directory.Spec) []any {
	var err error
	s, err = directory.Normalize(s)
	if err != nil {
		t.Fatal("invalid static performance fixture", err)
	}
	return []any{prefix(s.Query), prefix(s.Company), prefix(s.JobTitle), s.Skills, s.Institution, s.StudyProgram, s.GraduationYear, s.After, s.Limit + 1, normalizationFrom, normalizationTo}
}

func seedPerformance(t *testing.T, ctx context.Context, tx pgx.Tx, size int) {
	t.Helper()
	statements := []string{
		`CREATE TEMP TABLE perf_fixture ON COMMIT DROP AS SELECT n,md5('perf-user-'||n)::uuid id,md5('perf-member-'||n)::uuid member_id,'perf-'||lpad(n::text,8,'0') slug FROM generate_series(1,$1::integer) n`,
		`INSERT INTO users(id,oidc_subject,display_name) SELECT id,slug,CASE n%4 WHEN 0 THEN 'Siti  Nur ' WHEN 1 THEN 'Budi Santoso ' WHEN 2 THEN 'Ⓐyu Putri ' ELSE 'Agus Wijaya ' END||n FROM perf_fixture`,
		`INSERT INTO memberships(id,user_id,study_program_id,role,status,graduation_year) SELECT f.member_id,f.id,sp.id,CASE WHEN n%13=0 THEN 'student' ELSE 'alumni' END,CASE WHEN n%13=0 THEN 'active' WHEN n%7=0 THEN 'pending' ELSE 'verified' END,CASE WHEN n%13=0 THEN NULL ELSE 2020 END FROM perf_fixture f CROSS JOIN study_programs sp WHERE sp.slug='mathematics-education'`,
		`INSERT INTO profile_slug_history(slug,user_id) SELECT slug,id FROM perf_fixture`,
		`INSERT INTO profiles(user_id,slug,visibility,moderation_status,directory_discoverable,directory_consented_at,current_company,current_job_title,show_current_company,show_current_job_title,show_skills) SELECT id,slug,CASE WHEN n%3=0 THEN 'public' ELSE 'private' END,CASE WHEN n%11=0 THEN 'hidden' ELSE 'visible' END,n%3=0 AND n%5<>0,CASE WHEN n%3=0 AND n%5<>0 THEN now() END,CASE WHEN n%2=1 THEN 'Secret hidden company' WHEN n%4=0 THEN 'Nusantara  Digital' ELSE 'Acme' END,CASE WHEN n%4=0 THEN 'Engineer' ELSE 'Teacher' END,n%2=0,n%2=0,n%2=0 FROM perf_fixture`,
		`INSERT INTO profile_directory_affiliations(membership_id,show_institution,show_study_program,show_graduation_year) SELECT member_id,true,n%2=0,n%2=0 FROM perf_fixture WHERE n%5=0 OR n%8=0`,
		`INSERT INTO skills(id,name,normalized_name) VALUES(md5('perf-skill')::uuid,'Golang','golang')`,
		`INSERT INTO profile_skills(user_id,skill_id) SELECT id,md5('perf-skill')::uuid FROM perf_fixture WHERE n%4=0`,
		`ANALYZE`,
	}
	for i, sql := range statements {
		var err error
		if i == 0 {
			_, err = tx.Exec(ctx, sql, size)
		} else {
			_, err = tx.Exec(ctx, sql)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}
