package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wyw14/solo-039/internal/cleaning"
	"github.com/wyw14/solo-039/internal/journal"
	"github.com/wyw14/solo-039/internal/model"
)

func newTestPlant(t *testing.T) *Plant {
	t.Helper()
	return newTestPlantAt(t, t.TempDir())
}

func newTestPlantAt(t *testing.T, dir string) *Plant {
	t.Helper()
	p := NewPlant("data")
	p.FileLog = journal.NewFileLog(filepath.Join(dir, "events.jsonl"))
	p.Snapshots = journal.NewSnapshotFile(filepath.Join(dir, "snapshot.json"))
	p.Cleaning.Sink = p.FileLog.Append
	return p
}

func goodRinse() model.RinseRecord {
	return model.RinseRecord{Flow: 12, Conductivity: 3, SamplePass: true}
}

func addEnoughSamples(t *testing.T, p *Plant) {
	t.Helper()
	base := time.Now().Unix()
	for i := 0; i < 3; i++ {
		if _, err := p.AddSample(0.5, base+int64(i)); err != nil {
			t.Fatalf("sample %d rejected: %v", i, err)
		}
	}
}

func TestRouteBlockedWithoutClearance(t *testing.T) {
	p := newTestPlant(t)
	if _, err := p.Start(context.Background(), "premium", "primary"); err != nil {
		t.Fatal(err)
	}
	// Even with composition evidence in place, the missing cleaning clearance
	// must keep the product line shut.
	addEnoughSamples(t, p)
	err := p.RouteProduct("TANK-A", "B-1")
	if err == nil {
		t.Fatal("product line opened without a cleaning clearance")
	}
	if !strings.Contains(err.Error(), "cleaning clearance") {
		t.Fatalf("expected cleaning gate error, got %v", err)
	}
	if p.Product.Route.Opened() {
		t.Fatal("route must stay closed when cleaning evidence is missing")
	}
}

func TestRouteOpensWithMatchingClearance(t *testing.T) {
	p := newTestPlant(t)
	if _, err := p.Start(context.Background(), "premium", "primary"); err != nil {
		t.Fatal(err)
	}
	addEnoughSamples(t, p)
	if _, err := p.ReleaseTankLine(cleaning.ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse(), ApprovedBy: "qc"}); err != nil {
		t.Fatalf("clearance release failed: %v", err)
	}
	if err := p.RouteProduct("TANK-A", "B-1"); err != nil {
		t.Fatalf("matching clearance should open the line: %v", err)
	}
	if !p.Product.Route.Opened() || p.Product.Route.Target() != "TANK-A" {
		t.Fatal("route not opened for cleared line")
	}
}

func TestRouteRejectsLineAndBatchMismatch(t *testing.T) {
	p := newTestPlant(t)
	if _, err := p.Start(context.Background(), "premium", "primary"); err != nil {
		t.Fatal(err)
	}
	addEnoughSamples(t, p)
	if _, err := p.ReleaseTankLine(cleaning.ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse()}); err != nil {
		t.Fatal(err)
	}
	if err := p.RouteProduct("TANK-B", "B-1"); err == nil {
		t.Fatal("line opened by reusing another tank line's clearance")
	}
	if err := p.RouteProduct("TANK-A", "B-2"); err == nil {
		t.Fatal("line opened by reusing another batch's clearance")
	}
}

func TestNewRunInvalidatesClearance(t *testing.T) {
	p := newTestPlant(t)
	if _, err := p.Start(context.Background(), "premium", "primary"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ReleaseTankLine(cleaning.ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse()}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Shutdown(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	// A fresh run starts from Stopped -> Preparing and must not inherit proofs.
	if _, err := p.Start(context.Background(), "premium", "primary"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Cleaning.Authorize("TANK-A", "B-1"); err == nil {
		t.Fatal("clearance carried over into a new campaign run")
	}
}

func TestClearanceSurvivesRestore(t *testing.T) {
	dir := t.TempDir()
	p := newTestPlantAt(t, dir)
	if _, err := p.Start(context.Background(), "premium", "primary"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ReleaseTankLine(cleaning.ReleaseRequest{Line: "TANK-A", Batch: "B-1", Rinse: goodRinse()}); err != nil {
		t.Fatal(err)
	}

	fresh := newTestPlantAt(t, dir)
	if err := fresh.Restore(); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if _, err := fresh.Cleaning.Authorize("TANK-A", "B-1"); err != nil {
		t.Fatalf("clearance not restored: %v", err)
	}
}
