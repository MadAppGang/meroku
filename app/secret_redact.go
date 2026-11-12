package main

import (
	"net/url"
	"sort"
	"strings"
	"sync"
)

// Secret redaction.
//
// Values meroku holds in memory that must never reach a file, a log, the plan
// export or the Fix-with-AI briefing are registered here: the Neon user key,
// the Neon OAuth token, owner passwords and the Postgres credentials JSON.
// redactSecrets replaces every registered value, and its URL-encoded forms,
// with redactedMarker.

const redactedMarker = "[REDACTED]"

// minSecretLen is the shortest value registerSecret accepts. Shorter values
// would match ordinary text and redact it.
const minSecretLen = 8

var secretRegistry struct {
	mu     sync.RWMutex
	values map[string]struct{}
	// sorted is values longest first, so a secret that contains another is
	// replaced whole. Rebuilt on every registration.
	sorted []string
}

// registerSecret adds v, its query-escaped and its path-escaped forms to the
// values redactSecrets removes. Values shorter than minSecretLen are ignored.
func registerSecret(v string) {
	if len(v) < minSecretLen {
		return
	}
	secretRegistry.mu.Lock()
	defer secretRegistry.mu.Unlock()
	if secretRegistry.values == nil {
		secretRegistry.values = map[string]struct{}{}
	}
	added := false
	for _, form := range []string{v, url.QueryEscape(v), url.PathEscape(v)} {
		if _, ok := secretRegistry.values[form]; !ok {
			secretRegistry.values[form] = struct{}{}
			added = true
		}
	}
	if !added {
		return
	}
	sorted := make([]string, 0, len(secretRegistry.values))
	for s := range secretRegistry.values {
		sorted = append(sorted, s)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if len(sorted[i]) != len(sorted[j]) {
			return len(sorted[i]) > len(sorted[j])
		}
		return sorted[i] < sorted[j]
	})
	secretRegistry.sorted = sorted
}

// redactSecrets returns s with every registered secret replaced by
// redactedMarker.
func redactSecrets(s string) string {
	secretRegistry.mu.RLock()
	defer secretRegistry.mu.RUnlock()
	for _, secret := range secretRegistry.sorted {
		if strings.Contains(s, secret) {
			s = strings.ReplaceAll(s, secret, redactedMarker)
		}
	}
	return s
}
