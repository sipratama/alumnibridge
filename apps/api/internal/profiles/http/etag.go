package profileshttp

import (
    "errors"
    "fmt"
    "net/http"
    "strconv"
    "strings"
)

// Strong ETags represent the version of exactly one owner-editable resource.
// They are validators, not authorization credentials.
var (
    errMissingPrecondition = errors.New("missing If-Match precondition")
    errInvalidPrecondition = errors.New("invalid If-Match precondition")
)

// Version values are scoped to endpoint resource classes; never reuse a
// privacy ETag for profile content or child history writes.
func resourceETag(resource string, revision int64) string {
    return fmt.Sprintf("\"ab-%s-v%d\"", resource, revision)
}

// parseExpectedRevision rejects weak validators, wildcard, comma-separated
// validators, alternative resources, malformed revisions, and duplicate headers.
// The full HTTP handler must translate missing to 428 and malformed to 400;
// stale revision is detected atomically in the PostgreSQL transaction (412).
func parseExpectedRevision(r *http.Request, resource string) (int64, error) {
    vals := r.Header.Values("If-Match")
    if len(vals) == 0 {
        return 0, errMissingPrecondition
    }
    if len(vals) != 1 {
        return 0, errInvalidPrecondition
    }
    value := vals[0]
    prefix := "\"ab-" + resource + "-v"
    if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, "\"") {
        return 0, errInvalidPrecondition
    }
    number := strings.TrimSuffix(strings.TrimPrefix(value, prefix), "\"")
    if number == "" || number[0] == '0' {
        return 0, errInvalidPrecondition
    }
    for _, c := range number {
        if c < '0' || c > '9' {
            return 0, errInvalidPrecondition
        }
    }
    revision, err := strconv.ParseInt(number, 10, 64)
    if err != nil || revision < 1 {
        return 0, errInvalidPrecondition
    }
    return revision, nil
}

// requireRevision enforces conditional writes on authenticated owner resources.
// Missing validators return 428; malformed or cross-resource values return 400.
func requireRevision(r *http.Request, resource string) (int64, error) {
 rev, err := parseExpectedRevision(r, resource)
 if err != nil {return 0, err}
 return rev,nil
}
