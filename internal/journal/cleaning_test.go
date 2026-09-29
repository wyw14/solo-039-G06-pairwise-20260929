package journal

import (
	"testing"

	"github.com/google/uuid"
	"github.com/wyw14/solo-039/internal/model"
)

func mustParseUUID(t *testing.T, value string) uuid.UUID {
	t.Helper()
	parsed, err := uuid.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func sampleClearance(t *testing.T, line, batch, camp, grade string) model.CleaningClearance {
	t.Helper()
	c, err := model.DefaultCleaningLimits().Approve(model.CleaningClearance{
		Line:         line,
		Batch:        mustParseUUID(t, batch),
		Campaign:     mustParseUUID(t, camp),
		TargetGrade:  grade,
		FlushFlow:    12.0,
		Conductivity: 1.5,
		SamplePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPersistAndFindClearance(t *testing.T) {
	store := New()
	batch := "22222222-2222-2222-2222-222222222222"
	campID := "11111111-1111-1111-1111-111111111111"
	camp := model.Campaign{ID: mustParseUUID(t, campID), Revision: mustParseUUID(t, batch), Epoch: 1, Fencing: 1}
	c := sampleClearance(t, "TK-A", batch, campID, "premium")

	if _, err := PersistClearance(store, c, camp); err != nil {
		t.Fatal(err)
	}
	got, ok := FindClearance(store.Events(), "TK-A", batch)
	if !ok {
		t.Fatal("clearance not rebuilt from durable history")
	}
	if got.Digest != c.Digest || got.TargetGrade != "premium" {
		t.Fatal("rebuilt clearance does not match the persisted certificate")
	}
}

func TestFindClearanceIsBoundToLineAndBatch(t *testing.T) {
	store := New()
	batch := "22222222-2222-2222-2222-222222222222"
	campID := "11111111-1111-1111-1111-111111111111"
	camp := model.Campaign{ID: mustParseUUID(t, campID), Revision: mustParseUUID(t, batch), Epoch: 1, Fencing: 1}
	c := sampleClearance(t, "TK-A", batch, campID, "premium")
	if _, err := PersistClearance(store, c, camp); err != nil {
		t.Fatal(err)
	}
	events := store.Events()

	if _, ok := FindClearance(events, "TK-B", batch); ok {
		t.Error("a switched tank line must not resolve the old certificate")
	}
	if _, ok := FindClearance(events, "TK-A", "33333333-3333-3333-3333-333333333333"); ok {
		t.Error("a new batch must not resolve the old certificate")
	}
	if _, err := RequireClearance(events, "TK-A", batch, "baseline"); err == nil {
		t.Error("grade mismatch must fail closed")
	}
	if len(LoadClearances(events)) != 1 {
		t.Error("durable history must rebuild exactly one certificate")
	}
}

func TestPersistClearanceRejectsBatchMismatch(t *testing.T) {
	store := New()
	batch := "22222222-2222-2222-2222-222222222222"
	campID := "11111111-1111-1111-1111-111111111111"
	camp := model.Campaign{ID: mustParseUUID(t, campID), Revision: mustParseUUID(t, batch), Epoch: 1, Fencing: 1}
	c := sampleClearance(t, "TK-A", "33333333-3333-3333-3333-333333333333", campID, "premium")
	if _, err := PersistClearance(store, c, camp); err == nil {
		t.Fatal("a certificate bound to another batch generation must not persist")
	}
}
