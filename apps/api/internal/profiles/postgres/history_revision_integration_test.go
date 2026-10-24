package postgres

import (
 "context"
 "errors"
 "os"
 "sync"
 "testing"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
 "github.com/sipratama/alumnibridge/apps/api/internal/profiles"
)

// Verifies PostgreSQL, not a mocked revision store. CI provisions a disposable database
// and applies migrations through 000007 before running this test.
func TestConditionalProfileExperienceEducation(t *testing.T) {
 url:=os.Getenv("TEST_DATABASE_URL");if url==""{t.Skip("TEST_DATABASE_URL not set")}
 ctx:=context.Background()
 db,err:=pgxpool.New(ctx,url);if err!=nil{t.Fatal(err)};defer db.Close()
 owner:=uuid.New()
 _,err=db.Exec(ctx,`INSERT INTO users(id,oidc_subject,display_name) VALUES($1,$2,'Revision Owner')`,owner,"issue11-"+owner.String())
 if err!=nil{t.Fatal(err)};defer db.Exec(ctx,`DELETE FROM users WHERE id=$1`,owner)
 s:=New(db)
 initial,err:=s.Profile(ctx,owner);if err!=nil{t.Fatal(err)}
 h1:="Headline A";h2:="Headline B"
 one,err:=s.UpdateProfileRevision(ctx,owner,initial.Revision,func(p profiles.Profile)(profiles.Profile,error){
  return profiles.ApplyProfilePatch(p,profiles.ProfilePatch{Headline:&h1})
 })
 if err!=nil{t.Fatal(err)}
 if one.Revision<=initial.Revision{t.Fatal("content revision did not advance")}
 _,err=s.UpdateProfileRevision(ctx,owner,initial.Revision,func(p profiles.Profile)(profiles.Profile,error){
  return profiles.ApplyProfilePatch(p,profiles.ProfilePatch{Headline:&h2})
 })
 if !errors.Is(err,profiles.ErrStale){t.Fatalf("stale profile write: %v",err)}
 persisted,err:=s.Profile(ctx,owner)
 if err!=nil||persisted.Headline!=h1{t.Fatalf("stale profile overwritten: %+v %v",persisted,err)}

 experience,err:=s.SaveExperience(ctx,owner,profiles.Experience{
  Company:"Acme",JobTitle:"Engineer",StartDate:"2024-01-01",
 },false)
 if err!=nil{t.Fatal(err)}
 var experienceRevision int64
 err=db.QueryRow(ctx,`SELECT revision FROM profile_experiences WHERE id=$1`,experience.ID).Scan(&experienceRevision)
 if err!=nil{t.Fatal(err)}
 experience.Company="Updated Acme"
 updatedExperience,err:=s.SaveExperienceRevision(ctx,owner,experience,experienceRevision)
 if err!=nil{t.Fatal(err)}
 if updatedExperience.Revision<=experienceRevision{t.Fatal("experience revision not advanced")}
 experience.Company="Stale overwrite"
 _,err=s.SaveExperienceRevision(ctx,owner,experience,experienceRevision)
 if !errors.Is(err,profiles.ErrStale){t.Fatalf("stale experience update: %v",err)}
 if err=s.DeleteExperienceRevision(ctx,owner,experience.ID,experienceRevision);!errors.Is(err,profiles.ErrStale){t.Fatalf("stale experience delete: %v",err)}
 if err=s.DeleteExperienceRevision(ctx,owner,experience.ID,updatedExperience.Revision);err!=nil{t.Fatal(err)}

 education,err:=s.SaveEducation(ctx,owner,profiles.Education{Institution:"University"},false)
 if err!=nil{t.Fatal(err)}
 var educationRevision int64
 err=db.QueryRow(ctx,`SELECT revision FROM profile_educations WHERE id=$1`,education.ID).Scan(&educationRevision)
 if err!=nil{t.Fatal(err)}
 education.Degree="BSc"
 updatedEducation,err:=s.SaveEducationRevision(ctx,owner,education,educationRevision)
 if err!=nil{t.Fatal(err)}
 if updatedEducation.Revision<=educationRevision{t.Fatal("education revision not advanced")}
 education.Degree="Stale degree"
 _,err=s.SaveEducationRevision(ctx,owner,education,educationRevision)
 if !errors.Is(err,profiles.ErrStale){t.Fatalf("stale education update: %v",err)}
 if err=s.DeleteEducationRevision(ctx,owner,education.ID,educationRevision);!errors.Is(err,profiles.ErrStale){t.Fatalf("stale education delete: %v",err)}
 if err=s.DeleteEducationRevision(ctx,owner,education.ID,updatedEducation.Revision);err!=nil{t.Fatal(err)}

 // Concurrent same-version professional updates: exactly one commits.
 base,err:=s.Profile(ctx,owner);if err!=nil{t.Fatal(err)}
 start:=make(chan struct{});results:=make(chan error,2);var wg sync.WaitGroup
 for _,headline:=range []string{"Concurrent A","Concurrent B"} {
  wg.Add(1)
  go func(value string){defer wg.Done();<-start
   _,e:=s.UpdateProfileRevision(ctx,owner,base.Revision,func(p profiles.Profile)(profiles.Profile,error){return profiles.ApplyProfilePatch(p,profiles.ProfilePatch{Headline:&value})})
   results<-e
  }(headline)
 }
 close(start);wg.Wait();close(results)
 success,stale:=0,0
 for err:=range results{if err==nil{success++}else if errors.Is(err,profiles.ErrStale){stale++}else{t.Fatal(err)}}
 if success!=1||stale!=1{t.Fatalf("expected one update and one stale: %d %d",success,stale)}
}
