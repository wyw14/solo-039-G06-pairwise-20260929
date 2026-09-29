package cleaning

import (
	"testing"

	"github.com/wyw14/solo-039/internal/campaign"
	"github.com/wyw14/solo-039/internal/journal"
	"github.com/wyw14/solo-039/internal/model"
)

func goodRinse() model.RinseRecord {
	return model.RinseRecord{Flow: 12, Conductivity: 3, SamplePass: true}
}

func newHarness(grade string) (*Coordinator, model.Campaign) {
	state := campaign.New(grade)
	current := state.Snapshot()
	store := journal.New()
	return NewCoordinator(state, store), current
}

func TestReleaseAcceptsCompleteEvidence(t *testing.T) {
	c, current := newHarness("premium")
	clearance, err := c.Release(ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse(), ApprovedBy: "qc"})
	if err != nil {
		t.Fatalf("release failed: %v", err)
	}
	if clearance.Line != "TANK-A" || clearance.Batch != "B-1" {
		t.Fatal("clearance not bound to line/batch")
	}
	if clearance.TargetGrade != "premium" || clearance.Campaign != current.ID || clearance.Revision != current.Revision {
		t.Fatal("clearance not bound to target grade and generation")
	}
	if _, err := c.Authorize("TANK-A", "B-1"); err != nil {
		t.Fatalf("line should be openable: %v", err)
	}
}

func TestReleaseRejectsIncompleteEvidence(t *testing.T) {
	c, _ := newHarness("premium")
	cases := []struct {
		name  string
		rinse model.RinseRecord
	}{
		{"low flow", model.RinseRecord{Flow: 1, Conductivity: 3, SamplePass: true}}, {"high conductivity", model.RinseRecord{Flow: 12, Conductivity: 99, SamplePass: true}},
		{"sample failed", model.RinseRecord{Flow: 12, Conductivity: 3, SamplePass: false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.Release(ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: tc.rinse}); err == nil {
				t.Fatal("expected release to be rejected")
			}
		})
	}
	// No certificate may have been issued: the line stays shut.
	if _, err := c.Authorize("TANK-A", "B-1"); err == nil {
		t.Fatal("line opened without valid clearance")
	}
}

func TestAuthorizeRejectsMissingClearance(t *testing.T) {
	c, _ := newHarness("premium")
	if _, err := c.Authorize("TANK-A", "B-1"); err == nil {
		t.Fatal("line opened with no cleaning clearance")
	}
}

func TestSwitchingTankLineDoesNotReuseCertificate(t *testing.T) {
	c, _ := newHarness("premium")
	if _, err := c.Release(ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse()}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Authorize("TANK-B", "B-1"); err == nil {
		t.Fatal("clearance was reused for another tank line")
	}
}

func TestSwitchingBatchDoesNotReuseCertificate(t *testing.T) {
	c, _ := newHarness("premium")
	if _, err := c.Release(ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse()}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Authorize("TANK-A", "B-2"); err == nil {
		t.Fatal("clearance was reused for another batch")
	}
}

func TestNewGradeGenerationDoesNotReuseCertificate(t *testing.T) {
	c, current := newHarness("premium")
	if _, err := c.Release(ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse()}); err != nil {
		t.Fatal(err)
	}
	// Grade transition mints a new revision for the same campaign.
	current.Revision = model.NewCampaign("economy").Revision
	current.Grade = "economy"
	campaign.Restore(c.Campaign, current)
	if _, err := c.Authorize("TANK-A", "B-1"); err == nil {
		t.Fatal("clearance survived a grade generation change")
	}
}

func TestRevokeClosesLine(t *testing.T) {
	c, _ := newHarness("premium")
	if _, err := c.Release(ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse()}); err != nil {
		t.Fatal(err)
	}
	if err := c.Revoke("TANK-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Authorize("TANK-A", "B-1"); err == nil {
		t.Fatal("revoked clearance still authorizes the line")
	}
}

func TestInvalidateGenerationRemovesStaleProofs(t *testing.T) {
	c, current := newHarness("premium")
	if _, err := c.Release(ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse()}); err != nil {
		t.Fatal(err)
	}
	// Same grade, fresh revision (a new product generation).
	current.Revision = model.NewCampaign("premium").Revision
	campaign.Restore(c.Campaign, current)
	c.InvalidateGeneration()
	if _, err := c.Authorize("TANK-A", "B-1"); err == nil {
		t.Fatal("stale clearance survived generation invalidation")
	}
}

func TestReleaseIsDurable(t *testing.T) {
	c, _ := newHarness("premium")
	clearance, err := c.Release(ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse()})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.RequireClearanceLive(c.Journal, clearance.ID.String()); err != nil {
		t.Fatalf("clearance not durable: %v", err)
	}
	// Revocation is durable too.
	if err := c.Revoke("TANK-A"); err != nil {
		t.Fatal(err)
	}
	if journal.ClearanceLive(c.Journal, clearance.ID.String()) {
		t.Fatal("revoked clearance still reported live")
	}
}

func TestRestoreRebuildsLiveClearances(t *testing.T) {
	c, current := newHarness("premium")
	keep, err := c.Release(ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse()})
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := c.Release(ReleaseRequest{Line: "TANK-B", Batch: "B-1", Rinse: goodRinse()})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Revoke("TANK-B"); err != nil {
		t.Fatal(err)
	}

	registry, recovery := RestoreFor(c.Journal.Events(), current)
	if _, ok := registry.Get("TANK-A"); !ok {
		t.Fatal("live clearance not restored")
	}
	if _, ok := registry.Get("TANK-B"); ok {
		t.Fatal("revoked clearance was restored")
	}
	if err := recovery.Validate([]model.Clearance{keep}, current); err != nil {
		t.Fatalf("valid snapshot rejected: %v", err)
	}
	if err := recovery.Validate([]model.Clearance{revoked}, current); err == nil {
		t.Fatal("snapshot carrying a revoked clearance was accepted")
	}
}

func TestRestoreDropsClearancesFromAnotherGeneration(t *testing.T) {
	c, _ := newHarness("premium")
	if _, err := c.Release(ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse()}); err != nil {
		t.Fatal(err)
	}
	events := c.Journal.Events()

	// After a grade transition the old certificate must not come back.
	next := c.Campaign.Snapshot()
	next.Grade = "economy"
	next.Revision = model.NewCampaign("economy").Revision
	campaign.Restore(c.Campaign, next)
	registry, _ := RestoreFor(events, next)
	if _, ok := registry.Get("TANK-A"); ok {
		t.Fatal("previous generation clearance restored onto new generation")
	}
}
