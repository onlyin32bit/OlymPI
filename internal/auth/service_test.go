package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestPruneFailuresBoundsAndExpiresEntries(t *testing.T) {
	now := time.Now()
	service := &Service{failures: make(map[string]loginFailure)}
	service.failures["expired"] = loginFailure{Count: 1, LastFailed: now.Add(-2 * time.Hour)}
	for i := range 5000 {
		service.failures[fmt.Sprintf("source|user-%d", i)] = loginFailure{Count: 1, LastFailed: now}
	}

	service.pruneFailuresLocked(now)
	if _, ok := service.failures["expired"]; ok {
		t.Fatal("expired login failure was not pruned")
	}
	if len(service.failures) > 4096 {
		t.Fatalf("login failure map exceeded cap: %d", len(service.failures))
	}
}
