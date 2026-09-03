package main

import (
	"bytes"
	"math/rand/v2"
	"testing"
	"time"
)

// The pattern alphabet mirrors research/dp_verification.py: two literals, the
// ':' that a real sub claim is full of, and both metacharacters. Candidate
// strings use the literals only — a witness never needs a byte neither pattern
// mentions.
var (
	globPatternAlphabet = []byte("ab:*?")
	globStringAlphabet  = []byte("ab:")
)

// TestGlobsIntersectTable is the worked table: every claim the architecture
// makes about overlap, spelled out on the shapes a GitHub sub claim actually
// takes. Each case is named for what it proves, because a bare pair of globs
// two years from now says nothing about why it was worth writing down.
func TestGlobsIntersectTable(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		// The cases dp_verification.py checks by name.
		{"subset: a wildcard subject swallows an exact one",
			"repo:acme/api:*", "repo:acme/api:ref:refs/heads/main", true},
		{"different org does not overlap",
			"repo:MadAppGang/*", "repo:other/x:ref:refs/heads/main", false},
		{"the default subject list matches every repo in the org",
			"repo:MadAppGang/*", "repo:MadAppGang/billing:ref:refs/heads/main", true},
		{"sibling refs of one repo do not overlap",
			"repo:acme/api:ref:refs/heads/main", "repo:acme/api:ref:refs/heads/dev", false},
		{"crossing wildcards overlap",
			"repo:acme/*:ref:refs/heads/main", "repo:acme/api:*", true},
		{"prefix similarity is not overlap",
			"repo:acme/apiary:*", "repo:acme/api:*", false},
		{"? is length rigid: it cannot match zero bytes",
			"a?", "a", false},
		{"? matches exactly one byte",
			"a?", "ab", true},
		{"?* and *? both require at least one byte, so they meet",
			"?*", "*?", true},
		{"?* cannot match the empty string",
			"?*", "", false},
		{"* matches the empty string",
			"*", "", true},

		// Star-run collapsing, and the two shapes that must NOT collapse.
		{"** collapses to * and still swallows a whole org",
			"repo:acme/**", "repo:acme/api:ref:refs/heads/main", true},
		{"** is still a star against the empty pattern",
			"**", "", true},
		{"*? does not collapse to *: it cannot match the empty string",
			"*?", "", false},
		{"? and * meet on a single byte",
			"?", "*", true},

		// Mid-segment stars: IAM's * is not delimiter anchored, so a star in
		// the middle of a repo name is legal and crosses ':' and '/'.
		{"mid-segment star matches inside a repo name",
			"repo:acme/*ary:*", "repo:acme/apiary:ref:refs/heads/main", true},
		{"mid-segment stars on disjoint repo prefixes do not overlap",
			"repo:acme/api*:*", "repo:acme/svc*:*", false},
		{"a leading star crosses both ':' and '/'",
			"repo:MadAppGang/*", "*:ref:refs/heads/main", true},

		// Realistic shapes.
		{"identical subjects overlap (dev and prod of one project share a repo)",
			"repo:acme/api:ref:refs/heads/main", "repo:acme/api:ref:refs/heads/main", true},
		{"an environment subject does not meet a ref-only wildcard",
			"repo:acme/api:environment:prod", "repo:acme/api:ref:*", false},
		{"a repo-wide wildcard swallows the pull_request subject",
			"repo:acme/api:pull_request", "repo:acme/api:*", true},

		// Degenerate inputs, which the scanner will hand us for free the first
		// time somebody writes an empty subject list.
		{"empty meets empty",
			"", "", true},
		{"empty does not meet a literal",
			"", "a", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := []byte(tc.a), []byte(tc.b)
			if got := globsIntersect(a, b); got != tc.want {
				t.Errorf("globsIntersect(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
			// Overlap is a symmetric relation; the recurrence has separate
			// branches for a[i]=='*' and b[j]=='*', so assert it.
			if got := globsIntersect(b, a); got != tc.want {
				t.Errorf("globsIntersect(%q, %q) = %v, want %v (asymmetric)", tc.b, tc.a, got, tc.want)
			}
			// Whenever we claim overlap we owe the operator a witness, and the
			// independent matcher must accept it against both patterns.
			w, ok := globWitness(a, b)
			if ok != tc.want {
				t.Fatalf("globWitness(%q, %q) ok = %v, want %v", tc.a, tc.b, ok, tc.want)
			}
			if ok {
				if !globMatch(a, w) || !globMatch(b, w) {
					t.Errorf("witness %q from (%q, %q) is not matched by both", w, tc.a, tc.b)
				}
			}
		})
	}
}

// TestGlobsIntersectProperty compares the DP against brute force: enumerate
// every candidate string, ask globMatch whether each is matched by both
// patterns, and check the answer against globsIntersect in both directions.
//
// The string bound is the whole subtlety, and getting it wrong fails correct
// code. A DP path from (0,0) to (m,n) takes at most m+n steps and each step
// emits at most one byte, so two patterns of length <= L that intersect at all
// intersect on some string of length <= 2L. Enumerating to m+n is therefore
// EXHAUSTIVE, and anything shorter is not: "*aaaaa" and "bbbbb*" are both six
// bytes and do intersect, but only on "bbbbbaaaaa" — ten bytes. Stopping at six
// would report a mismatch against a correct answer.
//
// Two regimes, each enumerating to its own 2L: patterns <= 4 against strings
// <= 8 (the configuration research/dp_verification.py already ran over all
// 305371 pairs with zero mismatches), and a smaller number of patterns <= 5
// against strings <= 10, which reaches shapes four bytes cannot express.
func TestGlobsIntersectProperty(t *testing.T) {
	regimes := []struct {
		name       string
		seed       uint64
		maxPatLen  int
		numPats    int
		numStrings int // 3^0 + ... + 3^(2*maxPatLen), for the log line only
	}{
		{"patterns<=4, strings<=8", 0x5EED, 4, 300, 9841},
		{"patterns<=5, strings<=10", 0xC0FFEE, 5, 120, 88573},
	}

	for _, r := range regimes {
		t.Run(r.name, func(t *testing.T) {
			pats := randomGlobPatterns(t, r.seed, r.maxPatLen, r.numPats)
			strs := enumerateGlobStrings(2 * r.maxPatLen)
			if len(strs) != r.numStrings {
				t.Fatalf("enumerated %d strings, expected %d", len(strs), r.numStrings)
			}
			masks := globMatchMasks(pats, strs)

			pairs := 0
			for i := range pats {
				for j := i; j < len(pats); j++ {
					pairs++
					want := masksOverlap(masks[i], masks[j])
					if got := globsIntersect(pats[i], pats[j]); got != want {
						t.Fatalf("globsIntersect(%q, %q) = %v, brute force = %v",
							pats[i], pats[j], got, want)
					}
					if got := globsIntersect(pats[j], pats[i]); got != want {
						t.Fatalf("globsIntersect(%q, %q) = %v, brute force = %v (asymmetric)",
							pats[j], pats[i], got, want)
					}
				}
			}
			t.Logf("checked %d pairs against %d strings", pairs, len(strs))
		})
	}
}

// TestGlobWitness holds globWitness to its contract: ok exactly tracks
// globsIntersect, and every witness is accepted by the independently written
// globMatch against BOTH patterns. That is what makes the reported witness
// something an operator can paste into a trust-policy simulator.
//
// Patterns here run to length 6, past what the property test can enumerate
// exhaustively. That costs nothing: a witness globMatch accepts against both
// patterns *is* a proof of intersection, whatever its length, so this check
// needs no bound on the string space at all.
func TestGlobWitness(t *testing.T) {
	pats := randomGlobPatterns(t, 0xBEEF, 6, 260)

	// Plus the real subject shapes, so witness reconstruction is exercised on
	// input that looks like production and not only on {a,b,:}.
	for _, s := range []string{
		"repo:MadAppGang/*",
		"repo:acme/api:*",
		"repo:acme/api:ref:refs/heads/main",
		"repo:acme/*:ref:refs/heads/main",
		"repo:acme/api:pull_request",
		"repo:acme/api:environment:prod",
		"*",
		"**",
		"?*",
		"*?",
		"",
	} {
		pats = append(pats, []byte(s))
	}

	witnessed, empty := 0, 0
	for i := range pats {
		for j := i; j < len(pats); j++ {
			a, b := pats[i], pats[j]
			inter := globsIntersect(a, b)
			w, ok := globWitness(a, b)
			if ok != inter {
				t.Fatalf("globWitness(%q, %q) ok = %v, globsIntersect = %v", a, b, ok, inter)
			}
			if !ok {
				if w != nil {
					t.Fatalf("globWitness(%q, %q) returned witness %q with ok=false", a, b, w)
				}
				continue
			}
			if !globMatch(a, w) {
				t.Fatalf("witness %q for (%q, %q) is not matched by a", w, a, b)
			}
			if !globMatch(b, w) {
				t.Fatalf("witness %q for (%q, %q) is not matched by b", w, a, b)
			}
			witnessed++
			if len(w) == 0 {
				empty++
			}
		}
	}
	if empty == 0 {
		t.Error("no empty witness produced; the '*' vs '*' termination path went untested")
	}
	t.Logf("verified %d witnesses (%d empty)", witnessed, empty)
}

// TestGlobsIntersectPerformance is the memoisation guard. Two 1024-byte
// star-dense patterns are 512 stars each; the naive recursion branches at every
// one of them and does not return this decade, so a lost memo table shows up
// here as a test binary that never finishes rather than as a wrong answer.
//
// No goroutine-and-timer dance: the package timeout is the signal, and a
// stalled test that has to be killed is a clearer report than a flaky deadline.
func TestGlobsIntersectPerformance(t *testing.T) {
	const half = 512
	a := bytes.Repeat([]byte("*a"), half) // 1024 bytes, leading star
	b := bytes.Repeat([]byte("a*"), half) // 1024 bytes, trailing star

	start := time.Now()
	got := globsIntersect(a, b)
	w, ok := globWitness(a, b)
	elapsed := time.Since(start)

	if !got {
		t.Errorf("globsIntersect on star-dense patterns = false, want true (%d 'a's satisfy both)", half)
	}
	if !ok {
		t.Fatal("globWitness returned ok=false on intersecting star-dense patterns")
	}
	if !globMatch(a, w) || !globMatch(b, w) {
		t.Errorf("witness of length %d is not matched by both patterns", len(w))
	}
	t.Logf("1024x1024 intersection + witness in %s (witness %d bytes)", elapsed, len(w))

	// Generous by three orders of magnitude: the filled table is ~1M cells and
	// takes single-digit milliseconds. Anything near this bound is a table that
	// is not being reused.
	if elapsed > 5*time.Second {
		t.Errorf("took %s; the DP is not memoised", elapsed)
	}
}

// randomGlobPatterns draws distinct patterns of length <= maxLen from a fixed
// seed, so a failure reproduces exactly.
func randomGlobPatterns(t *testing.T, seed uint64, maxLen, count int) [][]byte {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))
	seen := make(map[string]bool, count)
	out := make([][]byte, 0, count)
	for len(out) < count {
		n := rng.IntN(maxLen + 1)
		p := make([]byte, n)
		for i := range p {
			p[i] = globPatternAlphabet[rng.IntN(len(globPatternAlphabet))]
		}
		if seen[string(p)] {
			continue
		}
		seen[string(p)] = true
		out = append(out, p)
	}
	return out
}

// enumerateGlobStrings lists every string of length <= maxLen over {a,b,:}.
func enumerateGlobStrings(maxLen int) [][]byte {
	out := [][]byte{{}}
	frontier := [][]byte{{}}
	for l := 1; l <= maxLen; l++ {
		next := make([][]byte, 0, len(frontier)*len(globStringAlphabet))
		for _, prefix := range frontier {
			for _, c := range globStringAlphabet {
				s := make([]byte, len(prefix)+1)
				copy(s, prefix)
				s[len(prefix)] = c
				next = append(next, s)
			}
		}
		out = append(out, next...)
		frontier = next
	}
	return out
}

// globMatchMasks builds, per pattern, a bitset of which candidate strings it
// matches. Two patterns intersect over the enumerated space iff their bitsets
// share a bit — the same construction dp_verification.py uses, and far cheaper
// than re-matching every string for every pair.
func globMatchMasks(pats, strs [][]byte) [][]uint64 {
	words := (len(strs) + 63) / 64
	masks := make([][]uint64, len(pats))
	for i, p := range pats {
		mask := make([]uint64, words)
		for k, s := range strs {
			if globMatch(p, s) {
				mask[k/64] |= 1 << uint(k%64)
			}
		}
		masks[i] = mask
	}
	return masks
}

func masksOverlap(x, y []uint64) bool {
	for i := range x {
		if x[i]&y[i] != 0 {
			return true
		}
	}
	return false
}
