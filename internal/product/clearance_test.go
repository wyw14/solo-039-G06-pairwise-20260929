package product

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/wyw14/solo-039/internal/analyzer"
	"github.com/wyw14/solo-039/internal/campaign"
	"github.com/wyw14/solo-039/internal/journal"
	"github.com/wyw14/solo-039/internal/model"
)

func newGatedCoordinator(t *testing.T) (*Coordinator, model.Campaign, model.ProductPlan) {
	t.Helper()
	camp := campaign.New("premium")
	analyzerState := analyzer.New()
	store := journal.New()
	book := NewClearanceBook()
	coord := NewCoordinator(camp, analyzerState, store, book)

	current := camp.Snapshot()
	analyzerState.BindFeed(current.Revision)
	calibration := analyzerState.Calibration()
	for i := int64(1); i <= 3; i++ {
		sample := model.Sample{ID: uuid.New(), FeedRevision: current.Revision, Calibration: calibration, At: i, Component: "top", Value: 0.9, Valid: true}
		if !analyzerState.Add(sample) {
			t.Fatalf("sample %d rejected", i)
		}
	}
	plan := model.ProductPlan{Campaign: current.ID, Revision: current.Revision, Grade: current.Grade, Target: "TK-A"}
	return coord, current, plan
}

func approveFor(t *testing.T, coord *Coordinator, line string, batch uuid.UUID, grade string) model.CleaningClearance {
	t.Helper()
	c, err := model.DefaultCleaningLimits().Approve(model.CleaningClearance{
		Line:         line,
		Batch:        batch,
		Campaign:     coord.Campaign.Snapshot().ID,
		TargetGrade:  grade,
		FlushFlow:    12.5,
		Conductivity: 2.0,
		SamplePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := coord.Clearances.Put(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRouteDeniedWithoutClearance(t *testing.T) {
	coord, _, plan := newGatedCoordinator(t)
	if err := coord.Publish(plan); err == nil {
		t.Fatal("line opened with no cleaning clearance")
	}
	if coord.Route.Opened() {
		t.Fatal("route must stay closed when evidence is missing")
	}
}

func TestRouteOpensWithMatchingClearance(t *testing.T) {
	coord, current, plan := newGatedCoordinator(t)
	approveFor(t, coord, plan.Target, current.Revision, plan.Grade)
	if err := coord.Publish(plan); err != nil {
		t.Fatalf("expected route to open with valid clearance: %v", err)
	}
	if !coord.Route.Opened() {
		t.Fatal("route should be open")
	}
}

func TestSwitchingTankCannotReuseCertificate(t *testing.T) {
	coord, current, plan := newGatedCoordinator(t)
	approveFor(t, coord, "TK-A", current.Revision, plan.Grade)

	switched := plan
	switched.Target = "TK-B"
	err := coord.PublishWithProof(switched, mustProof(t, coord, plan))
	if err == nil || !strings.Contains(err.Error(), "no cleaning clearance") {
		t.Fatalf("expected missing-clearance denial for switched tank, got %v", err)
	}
}

func TestNewBatchCannotReuseOldCertificate(t *testing.T) {
	coord, current, plan := newGatedCoordinator(t)
	approveFor(t, coord, plan.Target, current.Revision, plan.Grade)

	// A new campaign generation (new batch) arrives; the old certificate must
	// not follow it even on the same physical tank line.
	next := current
	next.Revision = uuid.New()
	next.Epoch++
	campaign.Restore(coord.Campaign, next)
	plan.Revision = next.Revision
	plan.Campaign = next.ID

	err := coord.Publish(plan)
	if err == nil || !strings.Contains(err.Error(), "another batch") {
		t.Fatalf("expected stale-batch denial, got %v", err)
	}
	if coord.Route.Opened() {
		t.Fatal("stale certificate must not open the line")
	}
}

func TestGradeMismatchDenies(t *testing.T) {
	coord, current, plan := newGatedCoordinator(t)
	approveFor(t, coord, plan.Target, current.Revision, "baseline")
	plan.Grade = "premium"
	if err := coord.Publish(plan); err == nil {
		t.Fatal("certificate approved for another grade opened the line")
	}
}

func TestRecoverRequiresClearanceAndDurableProof(t *testing.T) {
	coord, current, plan := newGatedCoordinator(t)

	// Durable proof exists but no cleaning release: recovery must still fail.
	proof, ok := coord.Analyzer.Proof(current.Revision)
	if !ok {
		t.Fatal("proof unavailable")
	}
	if err := journal.PersistProof(coord.Journal, proof, current); err != nil {
		t.Fatal(err)
	}
	if err := coord.Recover(plan); err == nil {
		t.Fatal("recovery opened line without cleaning clearance")
	}

	approveFor(t, coord, plan.Target, current.Revision, plan.Grade)
	if err := coord.Recover(plan); err != nil {
		t.Fatalf("recovery should succeed with clearance + durable proof: %v", err)
	}
	if !coord.Route.Opened() {
		t.Fatal("route should be open after recovery")
	}
}

func mustProof(t *testing.T, coord *Coordinator, plan model.ProductPlan) model.Proof {
	t.Helper()
	current := coord.Campaign.Snapshot()
	proof, ok := coord.Analyzer.Proof(current.Revision)
	if !ok {
		t.Fatal("proof unavailable")
	}
	return proof
}
