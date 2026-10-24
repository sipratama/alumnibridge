package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

func TestAppendAndSkillSetConcurrency(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	a, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	owner := uuid.New()
	if _, err := a.Exec(ctx, `INSERT INTO users(id,oidc_subject) VALUES($1,$2)`, owner, "collection-"+owner.String()); err != nil {
		t.Fatal(err)
	}
	defer a.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner)
	storeA, storeB := New(a), New(b)

	// Independent append operations do not replace or erase another row.
	var wg sync.WaitGroup
	experienceIDs := make(chan uuid.UUID, 2)
	errs := make(chan error, 2)
	for i, store := range []*Store{storeA, storeB} {
		wg.Add(1)
		go func(n int, s *Store) {
			defer wg.Done()
			x, e := s.SaveExperience(ctx, owner, profiles.Experience{Company: fmt.Sprintf("Company %d", n), JobTitle: "Engineer", StartDate: "2024-01-01"}, false)
			experienceIDs <- x.ID
			errs <- e
		}(i, store)
	}
	wg.Wait()
	close(experienceIDs)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	unique := map[uuid.UUID]bool{}
	for id := range experienceIDs {
		unique[id] = true
	}
	if len(unique) != 2 {
		t.Fatalf("append lost an experience: %v", unique)
	}

	// At 99 items, the same owner lock admits exactly one more append.
	if _, err := a.Exec(ctx, `INSERT INTO profile_experiences(user_id,company,title,start_date)
		SELECT $1,'Fixture','Engineer','2024-01-01' FROM generate_series(1,97)`, owner); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs = make(chan error, 2)
	for _, store := range []*Store{storeA, storeB} {
		wg.Add(1)
		go func(s *Store) {
			defer wg.Done()
			<-start
			_, e := s.SaveExperience(ctx, owner, profiles.Experience{Company: "Capacity", JobTitle: "Engineer", StartDate: "2024-01-01"}, false)
			errs <- e
		}(store)
	}
	close(start)
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, profiles.ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("experience capacity: success=%d conflict=%d", success, conflict)
	}
	var count int
	if err := a.QueryRow(ctx, `SELECT count(*) FROM profile_experiences WHERE user_id=$1`, owner).Scan(&count); err != nil || count != 100 {
		t.Fatalf("experience count=%d %v", count, err)
	}

	if _, err := a.Exec(ctx, `INSERT INTO profile_educations(user_id,institution_name)
		SELECT $1,'Fixture University' FROM generate_series(1,99)`, owner); err != nil {
		t.Fatal(err)
	}
	errs = make(chan error, 2)
	for _, store := range []*Store{storeA, storeB} {
		wg.Add(1)
		go func(s *Store) {
			defer wg.Done()
			_, e := s.SaveEducation(ctx, owner, profiles.Education{Institution: "New University"}, false)
			errs <- e
		}(store)
	}
	wg.Wait()
	close(errs)
	success, conflict = 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, profiles.ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("education capacity: success=%d conflict=%d", success, conflict)
	}
	if err := a.QueryRow(ctx, `SELECT count(*) FROM profile_educations WHERE user_id=$1`, owner).Scan(&count); err != nil || count != 100 {
		t.Fatalf("education count=%d %v", count, err)
	}

	// Same normalized skill is an idempotent set add, even across connections.
	skills := make(chan profiles.Skill, 2)
	errs = make(chan error, 2)
	for _, store := range []*Store{storeA, storeB} {
		wg.Add(1)
		go func(s *Store) {
			defer wg.Done()
			x, e := s.AddSkill(ctx, owner, "Concurrent Go", "concurrent-go-"+owner.String())
			skills <- x
			errs <- e
		}(store)
	}
	wg.Wait()
	close(skills)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var link uuid.UUID
	for x := range skills {
		if link != uuid.Nil && x.ID != link {
			t.Fatalf("duplicate skill links: %s %s", link, x.ID)
		}
		link = x.ID
	}
	if err := a.QueryRow(ctx, `SELECT count(*) FROM profile_skills WHERE user_id=$1`, owner).Scan(&count); err != nil || count != 1 {
		t.Fatalf("skill set count=%d %v", count, err)
	}

	// Delete holds the owner lock; a later add waits, then recreates the link.
	tx, err := a.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var locked uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, owner).Scan(&locked); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM profile_skills WHERE id=$1 AND user_id=$2`, link, owner); err != nil {
		t.Fatal(err)
	}
	addDone := make(chan profiles.Skill, 1)
	addErr := make(chan error, 1)
	go func() {
		x, e := storeB.AddSkill(ctx, owner, "Concurrent Go", "concurrent-go-"+owner.String())
		addDone <- x
		addErr <- e
	}()
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	newLink := <-addDone
	if err := <-addErr; err != nil {
		t.Fatal(err)
	}
	if newLink.ID == link {
		t.Fatal("delete/re-add reused deleted item ID")
	}
	if err := storeA.DeleteSkill(ctx, owner, link); !errors.Is(err, profiles.ErrNotFound) {
		t.Fatalf("old link deleted new link: %v", err)
	}
	if err := a.QueryRow(ctx, `SELECT count(*) FROM profile_skills WHERE user_id=$1`, owner).Scan(&count); err != nil || count != 1 {
		t.Fatalf("re-add lost: %d %v", count, err)
	}
	if err := storeA.DeleteSkill(ctx, owner, newLink.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.QueryRow(ctx, `SELECT count(*) FROM profile_skills WHERE user_id=$1`, owner).Scan(&count); err != nil || count != 0 {
		t.Fatalf("delete left link: %d %v", count, err)
	}
}
