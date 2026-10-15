package messaging

import (
	"encoding/json"
	"net/http"
)

// Grants.
const (
	GrantRead  = "read"
	GrantWrite = "write"
)

type grantEntry struct {
	Grant json.RawMessage `json:"grant"`
	Scope string          `json:"scope"`
}

// HasGrant reports whether the router forwarded the given string grant on this
// request (X-OpenHost-Permissions).
func HasGrant(r *http.Request, grant string) bool {
	var entries []grantEntry
	if err := json.Unmarshal([]byte(r.Header.Get("X-OpenHost-Permissions")), &entries); err != nil {
		return false
	}
	for _, e := range entries {
		var s string
		if json.Unmarshal(e.Grant, &s) == nil && s == grant {
			return true
		}
	}
	return false
}

// RequireGrants wraps a provider's service handler: GET and HEAD need "read",
// everything else "write". Requests that lack it get the openhost 403.
func RequireGrants(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		need := GrantWrite
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			need = GrantRead
		}
		if !HasGrant(r, need) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{
				"error":          ErrPermissionRequired,
				"required_grant": map[string]string{"grant": need, "scope": "global"},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
