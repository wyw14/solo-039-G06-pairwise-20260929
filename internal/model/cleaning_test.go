package model

import (
	"testing"
)

func validClearance() CleaningClearance {
	return CleaningClearance{
		Line:         "TK-A",
		Batch:        [16]byte{1},
		Campaign:     [16]byte{2},
		TargetGrade:  "premium",
		FlushFlow:    12.5,
		Conductivity: 2.0,
		SamplePassed: true,
	}
}

func TestCleaningLimitsAcceptsCompliantEvidence(t *testing.T) {
	limits := DefaultCleaningLimits()
	c, err := limits.Approve(validClearance())
	if err != nil {
		t.Fatalf("expected approval, got %v", err)
	}
	if !c.Approved || c.Digest == "" {
		t.Fatal("approved clearance must be flagged and carry a digest")
	}
}

func TestCleaningLimitsRejectsWeakEvidence(t *testing.T) {
	limits := DefaultCleaningLimits()
	cases := map[string]func(*CleaningClearance){
		"missing line":       func(c *CleaningClearance) { c.Line = "" },
		"missing batch":      func(c *CleaningClearance) { c.Batch = [16]byte{} },
		"missing grade":      func(c *CleaningClearance) { c.TargetGrade = "" },
		"zero flush flow":    func(c *CleaningClearance) { c.FlushFlow = 0 },
		"flow below minimum": func(c *CleaningClearance) { c.FlushFlow = limits.MinFlushFlow - 0.01 },
		"conductivity high":  func(c *CleaningClearance) { c.Conductivity = limits.MaxConductivity + 0.01 },
		"sample failed":      func(c *CleaningClearance) { c.SamplePassed = false },
	}
	for name, mutate := range cases {
		c := validClearance()
		mutate(&c)
		if _, err := limits.Approve(c); err == nil {
			t.Errorf("case %q: expected rejection", name)
		}
	}
}

func TestClearanceAuthorizesExactBinding(t *testing.T) {
	limits := DefaultCleaningLimits()
	c, err := limits.Approve(validClearance())
	if err != nil {
		t.Fatal(err)
	}
	other := [16]byte{9}
	if !c.Authorizes("TK-A", c.Batch, "premium") {
		t.Fatal("exact line/batch/grade must authorize")
	}
	if c.Authorizes("TK-B", c.Batch, "premium") {
		t.Error("switched tank line must not authorize")
	}
	if c.Authorizes("TK-A", other, "premium") {
		t.Error("new batch must not reuse the old certificate")
	}
	if c.Authorizes("TK-A", c.Batch, "baseline") {
		t.Error("different target grade must not authorize")
	}
}

func TestCleaningDigestBindsEvidence(t *testing.T) {
	limits := DefaultCleaningLimits()
	a, _ := limits.Approve(validClearance())
	b := validClearance()
	b.Conductivity = 3.5
	b, _ = limits.Approve(b)
	if a.Digest == b.Digest {
		t.Error("different readings must produce different digests")
	}
}
