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

func TestAtomicPrivacyAndDiscoveryRevision(t *testing.T) {
 url := os.Getenv("TEST_DATABASE_URL")
 if url == "" {t.Skip("TEST_DATABASE_URL not set")}
 ctx := context.Background()
 db,err:=pgxpool.New(ctx,url)
 if err!=nil {t.Fatal(err)}
 defer db.Close()
 uid:=uuid.New()
 _,err=db.Exec(ctx,`INSERT INTO users(id,oidc_subject,display_name) VALUES($1,$2,'Concurrency Test')`,uid,"conditional-"+uid.String())
 if err!=nil {t.Fatal(err)}
 defer db.Exec(ctx,`DELETE FROM users WHERE id=$1`,uid)
 store:=New(db)
 if _,err=store.Profile(ctx,uid);err!=nil {t.Fatal(err)}
 // A deliberately stale PUBLIC write cannot undo a committed PRIVATE withdrawal.
 original,err:=store.Privacy(ctx,uid)
 if err!=nil {t.Fatal(err)}
 pub,err:=store.UpdatePrivacyConditional(ctx,uid,profiles.PrivacyPatch{Visibility:"public"},original.Revision)
 if err!=nil {t.Fatal(err)}
 priv,err:=store.UpdatePrivacyConditional(ctx,uid,profiles.PrivacyPatch{Visibility:"private"},pub.Revision)
 if err!=nil {t.Fatal(err)}
 if priv.Revision<=pub.Revision {t.Fatalf("revision failed to advance: %d -> %d",pub.Revision,priv.Revision)}
 _,err=store.UpdatePrivacyConditional(ctx,uid,profiles.PrivacyPatch{Visibility:"public"},pub.Revision)
 if !errors.Is(err,profiles.ErrStale) {t.Fatalf("stale PUBLIC should fail: %v",err)}
 now,err:=store.Privacy(ctx,uid)
 if err!=nil||now.Visibility!="private"||now.Revision!=priv.Revision {t.Fatalf("withdrawal lost: %+v %v",now,err)}

 // A successful discovery update, even without a visibility or consent value change,
 // must increment the SAME privacy revision and invalidate older privacy drafts.
 patch:=profiles.DiscoveryPatch{DirectoryDiscoverable:new(bool)}
 discovery,err:=store.UpdateDiscoveryConditional(ctx,uid,patch,priv.Revision)
 if err!=nil {t.Fatal(err)}
 if discovery.Revision<=priv.Revision {t.Fatalf("discovery didn't bump revision: %d",discovery.Revision)}
 _,err=store.UpdatePrivacyConditional(ctx,uid,profiles.PrivacyPatch{Visibility:"public"},priv.Revision)
 if !errors.Is(err,profiles.ErrStale) {t.Fatalf("stale privacy after discovery: %v",err)}

 // Two signed-off writes with the same revision compete; exactly one can commit.
 baseline,err:=store.Privacy(ctx,uid)
 if err!=nil {t.Fatal(err)}
 var wg sync.WaitGroup
 start:=make(chan struct{})
 results:=make(chan error,2)
 for _,visibility:=range []string{"public","private"} {
  wg.Add(1)
  go func(v string){defer wg.Done(); <-start; _,e:=store.UpdatePrivacyConditional(ctx,uid,profiles.PrivacyPatch{Visibility:v},baseline.Revision);results<-e}(visibility)
 }
 close(start);wg.Wait();close(results)
 wins,stales:=0,0
 for e:=range results {if e==nil {wins++} else if errors.Is(e,profiles.ErrStale){stales++} else {t.Fatal(e)}}
 if wins!=1||stales!=1 {t.Fatalf("expected 1 success/1 stale, got %d/%d",wins,stales)}
}
