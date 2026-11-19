package main

import (
	"strings"
	"testing"
)

// enhancedPatternBytes measures what AWS measures: the JSON text of the whole
// FilterConfiguration, each pattern escaped inside it, the way
// modules/event_subscriber's filter_configuration_size does. The boundary is
// the one measured live: a 3956-byte plain pattern beside the SYSTEM_METADATA
// filter is accepted, 3957 bytes is refused.
func TestEnhancedPatternBytesMatchesAWS(t *testing.T) {
	pattern := func(n int) string {
		head, tail := `{"detail-type":["p0.lim.`, `"]}`
		return head + strings.Repeat("A", n-len(head)-len(tail)) + tail
	}
	cases := []struct {
		name    string
		pattern string
		want    int
	}{
		{"largest pattern AWS accepted", pattern(3956), 4096},
		{"smallest pattern AWS refused", pattern(3957), 4097},
		// Each " and \ in the pattern costs one more byte; <, > and & are
		// written as six-byte \u escapes, as jsonencode writes them.
		{"quotes, a backslash and <", `{"a":["x\\<"]}`, 84 + 52 + len(`{"a":["x\\<"]}`) + 6 + 5},
	}
	for _, tc := range cases {
		if got := enhancedPatternBytes(tc.pattern); got != tc.want {
			t.Errorf("%s: enhancedPatternBytes(%d-byte pattern) = %d, want %d", tc.name, len(tc.pattern), got, tc.want)
		}
	}
	if got := enhancedPatternBytes(pattern(3956)); got > enhancedPatternMaxBytes {
		t.Errorf("the largest pattern AWS accepted is refused: %d > %d", got, enhancedPatternMaxBytes)
	}
	if got := enhancedPatternBytes(pattern(3957)); got <= enhancedPatternMaxBytes {
		t.Errorf("the smallest pattern AWS refused passes: %d <= %d", got, enhancedPatternMaxBytes)
	}
}
