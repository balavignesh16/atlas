package rca

import "testing"

// Locks in the M2.4 confidence boundaries (LOW < 40 <= MEDIUM < 70 <= HIGH) as
// verified in docs/m24_verification_report.md. A prior uncommitted change during
// M2.7 work silently lowered the LOW/MEDIUM boundary to 30 with no test or
// documented justification; this test prevents that from happening unnoticed
// again.
func TestGetConfidence_Boundaries(t *testing.T) {
	e := &Engine{}

	cases := []struct {
		score    int
		expected string
	}{
		{0, "LOW"},
		{30, "LOW"},
		{39, "LOW"},
		{40, "MEDIUM"},
		{69, "MEDIUM"},
		{70, "HIGH"},
		{100, "HIGH"},
	}

	for _, c := range cases {
		if got := e.getConfidence(c.score); got != c.expected {
			t.Errorf("getConfidence(%d) = %q, want %q", c.score, got, c.expected)
		}
	}
}
