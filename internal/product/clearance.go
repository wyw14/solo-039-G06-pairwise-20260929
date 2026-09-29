package product

import (
	"errors"
	"fmt"
	"github.com/wyw14/solo-039/internal/model"
	"sync"
)

// clearanceKey binds a cleaning certificate to one concrete tank line and one
// concrete production batch. There is deliberately no lookup by line alone: a
// certificate is never valid after a tank change or a batch change.
type clearanceKey struct {
	line  string
	batch string
}

// ClearanceBook holds approved tank-line cleaning releases. A certificate is
// keyed by (line, batch) so it cannot be reused on a different tank line or a
// new batch.
type ClearanceBook struct {
	mu       sync.RWMutex
	releases map[clearanceKey]model.CleaningClearance
}

func NewClearanceBook() *ClearanceBook {
	return &ClearanceBook{releases: make(map[clearanceKey]model.CleaningClearance)}
}

// Put records an already-approved certificate. Unapproved or digest-less
// evidence is rejected so the book can only ever hold a valid release.
func (b *ClearanceBook) Put(c model.CleaningClearance) error {
	if !c.Approved {
		return errors.New("cleaning clearance is not approved")
	}
	if c.Digest == "" {
		return errors.New("cleaning clearance digest is missing")
	}
	key := clearanceKey{line: c.Line, batch: c.Batch.String()}
	b.mu.Lock()
	b.releases[key] = c
	b.mu.Unlock()
	return nil
}

// Get returns the certificate bound exactly to the given tank line and batch.
func (b *ClearanceBook) Get(line string, batch string) (model.CleaningClearance, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	c, ok := b.releases[clearanceKey{line: line, batch: batch}]
	return c, ok
}

// Require enforces the cleaning gate before a product line may open. It fails
// closed: missing evidence, another tank/batch, or a grade mismatch all deny.
func (b *ClearanceBook) Require(line string, batch string, grade string) (model.CleaningClearance, error) {
	c, ok := b.Get(line, batch)
	if !ok {
		if stale := b.staleForLine(line); stale {
			return model.CleaningClearance{}, errors.New("cleaning clearance belongs to another batch; re-clean and re-release the tank line")
		}
		return model.CleaningClearance{}, fmt.Errorf("no cleaning clearance for tank line %q", line)
	}
	if c.TargetGrade != grade {
		return model.CleaningClearance{}, errors.New("cleaning clearance is approved for another target grade")
	}
	return c, nil
}

// staleForLine reports whether a certificate exists for the line but under a
// different batch — the "old certificate follows a tank/batch change" case.
func (b *ClearanceBook) staleForLine(line string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for key := range b.releases {
		if key.line == line {
			return true
		}
	}
	return false
}

// Count reports how many certificates are currently held.
func (b *ClearanceBook) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.releases)
}
