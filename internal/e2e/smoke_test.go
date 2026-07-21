package e2e

import (
	"net/http"
	"testing"
)

// TestSmoke_HealthAndBasicRouting is the harness's own basic sanity check:
// it proves the assembled app (real router + real Postgres + real
// migrations/seed + httptest server) actually responds over HTTP. It makes
// no assertion about exact seed-data counts or seeded guilds' pristine
// state -- that check runs once in runTests (see validateSeedData in
// harness_test.go), at the one moment it's guaranteed pristine, rather than
// depending on this test running before any other. This test is therefore
// safe to run in any position, including under `go test -shuffle=on`.
func TestSmoke_HealthAndBasicRouting(t *testing.T) {
	status, raw := doJSON(t, http.MethodGet, "/health", nil, nil)
	requireStatus(t, status, http.StatusOK, raw)

	status, raw = doJSON(t, http.MethodGet, "/items?limit=1", nil, nil)
	requireStatus(t, status, http.StatusOK, raw)

	status, raw = doJSON(t, http.MethodGet, "/auctions", nil, nil)
	requireStatus(t, status, http.StatusOK, raw)
}
