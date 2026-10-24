package profileshttp

import (
 "errors"
 "net/http"
 "testing"
)

func TestResourceETag(t *testing.T) {
 r, _ := http.NewRequest("PATCH","/",nil)
 r.Header.Set("If-Match",resourceETag("privacy",42))
 got, err := parseExpectedRevision(r,"privacy")
 if err != nil || got != 42 { t.Fatalf("revision: %d %v",got,err) }
}
func TestETagErrors(t *testing.T) {
 r, _ := http.NewRequest("PATCH","/",nil)
 if _, err := parseExpectedRevision(r,"privacy"); !errors.Is(err,errMissingPrecondition) {t.Fatal(err)}
 for _,v := range []string{"*","W/\"ab-privacy-v1\"","\"ab-profile-v1\"","\"ab-privacy-v0\"","\"ab-privacy-v01\"","\"ab-privacy-v1\",\"ab-privacy-v2\""} {
   r.Header.Set("If-Match",v)
   if _,err := parseExpectedRevision(r,"privacy"); !errors.Is(err,errInvalidPrecondition) {t.Fatalf("%s %v",v,err)}
 }
 r.Header.Set("If-Match",resourceETag("privacy",1))
 r.Header.Add("If-Match",resourceETag("privacy",2))
 if _,err := parseExpectedRevision(r,"privacy"); !errors.Is(err,errInvalidPrecondition) {t.Fatal(err)}
}
