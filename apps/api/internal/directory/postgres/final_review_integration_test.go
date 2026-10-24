package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sipratama/alumnibridge/apps/api/internal/directory"
)

// Opt-in differential verification, run alone on an empty disposable database.
// Compare full ordered public JSON, rather than only EXPLAIN cardinalities.
func TestFinalReviewSearchEquivalenceAndPlans(t *testing.T) {
	if os.Getenv("DIRECTORY_FINAL_REVIEW") != "1" {
		t.Skip("opt-in full-result SQL differential and planner assessment")
	}
	db, ctx := reviewDatabase(t)
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 0 {
		t.Fatal("requires empty disposable database", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for _, size := range []int{1000, 10000, 25000} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			seedPerformance(t, ctx, tx, size)
			cases := []struct {
				name string
				spec directory.Spec
			}{
				{"browse", directory.Spec{Limit: 20}},
				{"maximum_page", directory.Spec{Limit: 50}},
				{"name", directory.Spec{Query: "SITI NUR", Limit: 20}},
				{"unicode_name", directory.Spec{Query: "Ⓐyu", Limit: 20}},
				{"name_no_match", directory.Spec{Query: "nonexistent", Limit: 20}},
				{"company", directory.Spec{Company: "nusantara", Limit: 20}},
				{"company_low_selectivity", directory.Spec{Company: "acme", Limit: 20}},
				{"hidden_company", directory.Spec{Company: "secret", Limit: 20}},
				{"title_low_selectivity", directory.Spec{JobTitle: "teacher", Limit: 20}},
				{"academic", directory.Spec{Institution: "uhamka", StudyProgram: "mathematics-education", GraduationYear: 2020, Limit: 20}},
				{"skills", directory.Spec{Skills: []string{"Golang"}, Limit: 20}},
				{"combined", directory.Spec{Query: "siti", Company: "nusantara", Institution: "uhamka", Skills: []string{"Golang"}, Limit: 20}},
				{"late", directory.Spec{After: fmt.Sprintf("perf-%08d", size*3/4), Limit: 20}},
				{"tail", directory.Spec{After: fmt.Sprintf("perf-%08d", size-20), Limit: 20}},
			}
			for _, target := range []int{100, 1000} {
				// Both setting and all rows are confined to the disposable database.
				if _, err = tx.Exec(ctx, fmt.Sprintf("SET LOCAL default_statistics_target=%d", target)); err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(ctx, `ANALYZE users; ANALYZE profiles; ANALYZE memberships; ANALYZE profile_skills; ANALYZE profile_directory_affiliations`); err != nil {
					t.Fatal(err)
				}
				for _, tc := range cases {
					args := searchArgs(t, tc.spec)
					before := reviewResults(t, ctx, tx, phaseBSearchSQL, args)
					after := reviewResults(t, ctx, tx, searchSQL, args)
					if !reflect.DeepEqual(before, after) {
						t.Fatalf("full public DTO mismatch: %s target=%d", tc.name, target)
					}
					if tc.name == "hidden_company" && len(after) != 0 {
						t.Fatal("hidden company influenced results")
					}
					for _, query := range []struct{ name, sql string }{{"before", phaseBSearchSQL}, {"after", searchSQL}} {
						var raw []byte
						if err = tx.QueryRow(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+query.sql, args...).Scan(&raw); err != nil {
							t.Fatal(err)
						}
						var plans []map[string]any
						if err = json.Unmarshal(raw, &plans); err != nil {
							t.Fatal(err)
						}
						root := plans[0]
						node := root["Plan"].(map[string]any)
						var scans []map[string]any
						var walk func(map[string]any)
						walk = func(n map[string]any) {
							if n["Relation Name"] != nil || n["Node Type"] == "Subquery Scan" || n["Sort Method"] != nil {
								scans = append(scans, map[string]any{"node": n["Node Type"], "relation": n["Relation Name"], "index": n["Index Name"], "rows": n["Actual Rows"], "removed": n["Rows Removed by Filter"], "loops": n["Actual Loops"], "sort": n["Sort Method"]})
							}
							if children, ok := n["Plans"].([]any); ok {
								for _, c := range children {
									walk(c.(map[string]any))
								}
							}
						}
						walk(node)
						result := map[string]any{"size": size, "statistics_target": target, "case": tc.name, "variant": query.name, "execution_ms": root["Execution Time"], "planning_ms": root["Planning Time"], "rows": len(after), "hit_blocks": node["Shared Hit Blocks"], "read_blocks": node["Shared Read Blocks"], "scans": scans}
						data, _ := json.Marshal(result)
						t.Log(string(data))
					}
				}
			}
			if size == 25000 {
				// EXPLAIN is not evidence of a cached statement's execution
				// latency. Exercise actual bound executions in both plan modes.
				for _, mode := range []string{"force_generic_plan", "force_custom_plan"} {
					if _, err = tx.Exec(ctx, "SET LOCAL plan_cache_mode="+mode); err != nil {
						t.Fatal(err)
					}
					for _, tc := range cases {
						switch tc.name {
						case "name", "name_no_match", "company_low_selectivity", "title_low_selectivity", "late":
						default:
							continue
						}
						var previous []string
						for _, query := range []struct{ name, sql string }{{"before", phaseBSearchSQL}, {"after", searchSQL}} {
							start := time.Now()
							out := reviewResults(t, ctx, tx, query.sql, searchArgs(t, tc.spec))
							elapsed := time.Since(start)
							if query.name == "before" {
								previous = out
							} else if !reflect.DeepEqual(previous, out) {
								t.Fatal("cached plan DTO mismatch", tc.name, mode)
							}
							t.Logf("cached execution size=%d mode=%s case=%s variant=%s elapsed=%s rows=%d", size, mode, tc.name, query.name, elapsed, len(out))
						}
					}
				}
			}
		})
	}
}

func reviewResults(t *testing.T, ctx context.Context, tx pgx.Tx, sql string, args []any) []string {
	t.Helper()
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		out = append(out, string(raw))
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
