package cleaning

import (
	"sync"
	"time"

	"github.com/wyw14/solo-039/internal/model"
)

// Decision is one recorded cleaning release or line-opening verdict.
type Decision struct {
	Kind      string // "release" or "authorize"
	Line      string
	Batch     string
	Clearance model.Clearance
	Approved  bool
	At        time.Time
	Reason    string
}

// Audit keeps the cleaning decision trail in memory.
type Audit struct {
	mu        sync.Mutex
	decisions []Decision
}

func NewAudit() *Audit { return &Audit{decisions: make([]Decision, 0, 32)} }

func (a *Audit) Record(kind, line, batch string, clearance model.Clearance, approved bool, reason string) {
	a.mu.Lock()
	a.decisions = append(a.decisions, Decision{Kind: kind, Line: line, Batch: batch, Clearance: clearance, Approved: approved, At: time.Now(), Reason: reason})
	a.mu.Unlock()
}

// Latest returns the most recent decision.
func (a *Audit) Latest() (Decision, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.decisions) == 0 {
		return Decision{}, false
	}
	return a.decisions[len(a.decisions)-1], true
}

// ForLine returns every decision recorded for a tank line.
func (a *Audit) ForLine(line string) []Decision {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Decision, 0)
	for _, decision := range a.decisions {
		if decision.Line == line {
			out = append(out, decision)
		}
	}
	return out
}

// Count returns the number of recorded decisions.
func (a *Audit) Count() int { a.mu.Lock(); defer a.mu.Unlock(); return len(a.decisions) }
