package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// MEROKU_LIVE_CANARY=profile:zoneID:zone writes the canary into a real zone,
// asks the public resolvers for it, and removes it.
func TestCanaryLive(t *testing.T) {
	spec := os.Getenv("MEROKU_LIVE_CANARY")
	if spec == "" {
		t.Skip("set MEROKU_LIVE_CANARY=profile:zoneID:zone")
	}
	var c dnsCanary
	parts := strings.SplitN(spec, ":", 3)
	if len(parts) != 3 {
		t.Fatal("MEROKU_LIVE_CANARY must be profile:zoneID:zone")
	}
	c.Profile, c.ZoneID, c.Zone, c.Token = parts[0], parts[1], parts[2], newCanaryToken()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := canaryRecordWriter(ctx, c, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := canaryRecordWriter(context.Background(), c, true); err != nil {
			t.Errorf("removing the canary: %v", err)
		}
	})

	results := canaryCheck(c)
	read, stale := countVerdicts(results)
	t.Logf("%s: %v (read %d, failing %d)", c.Zone, results, read, stale)
	if read < 2 {
		t.Errorf("fewer than two resolvers read the canary")
	}
}
