package recommend

import (
	"strconv"
	"strings"
	"testing"
)

// standaloneNumbers extracts the numeric literals of a sentence, skipping
// digits that are part of an identifier -- the 7 in "m7i.large" is a name, not
// a claim. Hand-rolled rather than regexp because this package's import list is
// a review gate.
func standaloneNumbers(s string) []string {
	isDigit := func(b byte) bool { return b >= '0' && b <= '9' }
	isLetter := func(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

	var out []string
	for i := 0; i < len(s); {
		if !isDigit(s[i]) {
			i++
			continue
		}
		attached := i > 0 && isLetter(s[i-1])
		start := i
		for i < len(s) && isDigit(s[i]) {
			i++
		}
		if i+1 < len(s) && s[i] == '.' && isDigit(s[i+1]) {
			i++
			for i < len(s) && isDigit(s[i]) {
				i++
			}
		}
		if i < len(s) && isLetter(s[i]) {
			attached = true
		}
		if !attached {
			out = append(out, s[start:i])
		}
	}
	return out
}

// citableFromSignals is every number a reason is allowed to state, formatted
// exactly as Explain would format it.
func citableFromSignals(s Signals) map[string]bool {
	allowed := map[string]bool{}
	add := func(v float64) {
		allowed[fmt1(v)] = true
		allowed[fmtPct(v)] = true
	}
	add(s.Configured.VCPU)
	add(s.Configured.MemGiB)
	add(s.Configured.Ratio)
	add(s.Ratio.Raw)
	add(s.Ratio.Effective)
	add(s.Ratio.CatalogMin)
	add(s.Ratio.CatalogMax)
	add(s.Coverage)
	add(s.WeightConfigured)
	add(s.WeightActual)
	if s.Actual != nil {
		add(s.Actual.VCPU)
		add(s.Actual.MemGiB)
		add(s.Actual.Ratio)
	}
	allowed[strconv.Itoa(s.ConfiguredTaskCount)] = true
	for _, sv := range s.Services {
		add(sv.CPUAvg)
		add(sv.CPUPeak)
		add(sv.MemAvg)
		add(sv.MemPeak)
		allowed[strconv.Itoa(sv.Datapoints)] = true
	}
	return allowed
}

// TestExplain_OnlyCitesSignals is AC-21. A reason quoting a figure the UI
// cannot show beside it is a figure nobody can check.
func TestExplain_OnlyCitesSignals(t *testing.T) {
	for name, in := range allFixtureInputs() {
		t.Run(name, func(t *testing.T) {
			res := Recommend(in)
			allowed := citableFromSignals(res.Signals)
			for _, c := range res.Ranked {
				if c.Reason == "" {
					t.Errorf("%s carries no reason", c.InstanceType)
					continue
				}
				for _, num := range standaloneNumbers(c.Reason) {
					if !allowed[num] {
						t.Errorf("%s reason cites %q, which is in no Signals field:\n  %s\n  allowed: %v",
							c.InstanceType, num, c.Reason, sortedKeys(allowed))
					}
				}
			}
		})
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// insertion sort keeps the import list at five
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// TestExplain_NamesTheClassificationAndRunnerUp is FR-30's content contract.
func TestExplain_NamesTheClassificationAndRunnerUp(t *testing.T) {
	in := baseInput()
	in.Services = measuredServices()
	// performance-first, because measuredServices sits squarely inside FR-18's
	// burstable box (8 % average CPU on 0.5-vCPU tasks) and the burstable class
	// leaves only one eligible type -- which is C-15 working, not a defect.
	in.Posture = PosturePerformance
	res := Recommend(in)
	if res.Primary == nil || len(res.Ranked) < 2 {
		t.Fatalf("fixture needs at least two ranked candidates, got %d (class %q)", len(res.Ranked), res.Classification)
	}
	reason := res.Primary.Reason

	if !strings.Contains(reason, classDisplay(res.Classification)) {
		t.Errorf("reason does not name the classification %q: %s", res.Classification, reason)
	}
	if !strings.Contains(reason, res.Primary.InstanceType) {
		t.Errorf("reason does not name the chosen type: %s", reason)
	}
	if !strings.Contains(reason, res.Ranked[1].InstanceType) {
		t.Errorf("reason does not name the runner-up %s: %s", res.Ranked[1].InstanceType, reason)
	}
	if !strings.Contains(reason, "GiB per vCPU") {
		t.Errorf("reason does not name the configured ratio: %s", reason)
	}
	if !strings.Contains(reason, "%") {
		t.Errorf("reason does not name the measured utilisation: %s", reason)
	}
}

// TestExplain_SaysWhenNothingWasMeasured is FR-30's second copy variant, and
// FR-27's refusal to substitute a guessed utilisation figure.
func TestExplain_SaysWhenNothingWasMeasured(t *testing.T) {
	res := Recommend(baseInput())
	if res.Primary == nil {
		t.Fatal("no primary")
	}
	if strings.Contains(res.Primary.Reason, "%") {
		t.Errorf("reason cites a percentage with no CloudWatch data: %s", res.Primary.Reason)
	}
	if !strings.Contains(res.Primary.Reason, "nothing has reported utilisation yet") {
		t.Errorf("reason does not say the data is missing: %s", res.Primary.Reason)
	}
}

// TestExplain_SaysWhenTheClampFired is C-10: never silent.
func TestExplain_SaysWhenTheClampFired(t *testing.T) {
	in := allFixtureInputs()["clamped"]
	res := Recommend(in)
	if res.Signals.Ratio.ClampedTo != ClampMax {
		t.Fatalf("fixture drift: clampedTo = %q", res.Signals.Ratio.ClampedTo)
	}
	reason := res.Primary.Reason
	if !strings.Contains(reason, "no instance family provides") {
		t.Errorf("reason does not report the clamp: %s", reason)
	}
	if !strings.Contains(reason, fmt1(res.Signals.Ratio.Raw)) {
		t.Errorf("reason does not state the unclamped ratio %s: %s", fmt1(res.Signals.Ratio.Raw), reason)
	}
	if !strings.Contains(reason, fmt1(res.Signals.Ratio.CatalogMax)) {
		t.Errorf("reason does not state the bound it was capped at: %s", reason)
	}

	// And it says nothing about a clamp when none fired.
	plain := Recommend(baseInput())
	if strings.Contains(plain.Primary.Reason, "no instance family provides") {
		t.Errorf("reason claims a clamp that did not fire: %s", plain.Primary.Reason)
	}
}

func TestStandaloneNumbers(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"memory-heavy — your services request 4.0 GiB per vCPU", []string{"4.0"}},
		{"so r7i.large fits better than m7i.large", nil},
		{"memory peaks at 78% while CPU averages 12%", []string{"78", "12"}},
		{"capped at 16.0.", []string{"16.0"}},
		{"x2i.xlarge and t4g.medium and m7i-flex.large", nil},
		{"no numbers here", nil},
	}
	for _, tc := range cases {
		got := standaloneNumbers(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("standaloneNumbers(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("standaloneNumbers(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}
