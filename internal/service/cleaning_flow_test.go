package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

func startedPlant(t *testing.T) *Plant {
	t.Helper()
	plant := NewPlant(t.TempDir())
	if _, err := plant.Start(context.Background(), "premium", "primary"); err != nil {
		t.Fatalf("plant start: %v", err)
	}
	base := time.Now().Unix()
	for i := int64(0); i < 3; i++ {
		if _, err := plant.AddSample(0.9, base+i); err != nil {
			t.Fatalf("sample %d: %v", i, err)
		}
	}
	return plant
}

func TestRouteBlockedUntilLineCleared(t *testing.T) {
	plant := startedPlant(t)

	// No cleaning release recorded: the product line must stay shut.
	err := plant.RouteProduct("TK-A")
	if err == nil || !strings.Contains(err.Error(), "cleaning clearance") {
		t.Fatalf("expected cleaning-clearance denial, got %v", err)
	}
	if len(plant.ProductTargets()) != 0 {
		t.Fatal("line must not be opened without a cleaning release")
	}

	// Record the rinse evidence and release the line for this batch/grade.
	c, err := plant.ApproveLineClearance("TK-A", 12.0, 2.0, true)
	if err != nil {
		t.Fatalf("approve clearance: %v", err)
	}
	if c.Batch != plant.Campaign.Snapshot().Revision {
		t.Fatal("clearance must bind the current batch")
	}

	if err := plant.RouteProduct("TK-A"); err != nil {
		t.Fatalf("route after valid clearance: %v", err)
	}
	if targets := plant.ProductTargets(); len(targets) != 1 || targets[0] != "TK-A" {
		t.Fatalf("line TK-A should be open, got %v", targets)
	}
}

func TestWeakEvidenceCannotRelease(t *testing.T) {
	plant := startedPlant(t)
	if _, err := plant.ApproveLineClearance("TK-A", 2.0, 2.0, true); err == nil {
		t.Fatal("rinse flow below minimum must reject the release")
	}
	if _, err := plant.ApproveLineClearance("TK-A", 12.0, 99.0, true); err == nil {
		t.Fatal("high outlet conductivity must reject the release")
	}
	if _, err := plant.ApproveLineClearance("TK-A", 12.0, 2.0, false); err == nil {
		t.Fatal("a failed rinse sample must reject the release")
	}
	if err := plant.RouteProduct("TK-A"); err == nil {
		t.Fatal("line opened despite no valid cleaning release")
	}
}

func TestSwitchingTankDoesNotReuseCertificate(t *testing.T) {
	plant := startedPlant(t)
	if _, err := plant.ApproveLineClearance("TK-A", 12.0, 2.0, true); err != nil {
		t.Fatal(err)
	}
	if err := plant.RouteProduct("TK-A"); err != nil {
		t.Fatalf("route TK-A: %v", err)
	}
	// Same batch, different physical tank line: no transferable certificate.
	if err := plant.RouteProduct("TK-B"); err == nil {
		t.Fatal("switching tanks reused the old cleaning certificate")
	}
}

func TestNewBatchDoesNotReuseOldCertificate(t *testing.T) {
	plant := startedPlant(t)
	if _, err := plant.ApproveLineClearance("TK-A", 12.0, 2.0, true); err != nil {
		t.Fatal(err)
	}
	if err := plant.RouteProduct("TK-A"); err != nil {
		t.Fatalf("route TK-A: %v", err)
	}

	// Grade transition creates a new batch generation; the old release must
	// not authorize the same tank for the new batch.
	if _, err := plant.Transition(context.Background(), "elite", "primary"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	err := plant.RouteProduct("TK-A")
	if err == nil || !strings.Contains(err.Error(), "another batch") {
		t.Fatalf("expected stale-batch denial after new generation, got %v", err)
	}
}

func TestClearanceRebuiltAfterRestore(t *testing.T) {
	dir := t.TempDir()
	plant := NewPlant(dir)
	if _, err := plant.Start(context.Background(), "premium", "primary"); err != nil {
		t.Fatal(err)
	}
	c, err := plant.ApproveLineClearance("TK-A", 12.0, 2.0, true)
	if err != nil {
		t.Fatal(err)
	}

	restored := NewPlant(dir)
	if err := restored.Restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, ok := restored.LineClearance("TK-A", c.Batch.String())
	if !ok {
		t.Fatal("durable cleaning certificate was not rebuilt after restart")
	}
	if got.Digest != c.Digest {
		t.Fatal("rebuilt certificate digest differs from the durable record")
	}
}
