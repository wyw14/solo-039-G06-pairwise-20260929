package cleaning

import (
	"sync"

	"github.com/wyw14/solo-039/internal/model"
)

// Registry holds the current cleaning clearance of each tank line. A line has
// at most one live certificate; issuing a new one supersedes the old one.
type Registry struct {
	mu      sync.RWMutex
	byLine  map[string]model.Clearance
	revoked map[string]struct{}
}

func NewRegistry() *Registry {
	return &Registry{byLine: make(map[string]model.Clearance), revoked: make(map[string]struct{})}
}

// Save installs the certificate as the live clearance of its tank line.
func (r *Registry) Save(c model.Clearance) {
	r.mu.Lock()
	delete(r.revoked, c.ID.String())
	r.byLine[c.Line] = c
	r.mu.Unlock()
}

// Get returns the live certificate of a tank line.
func (r *Registry) Get(line string) (model.Clearance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byLine[line]
	return c, ok
}

// All returns a snapshot of every live certificate, ordered by line.
func (r *Registry) All() []model.Clearance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.Clearance, 0, len(r.byLine))
	for _, c := range r.byLine {
		out = append(out, c)
	}
	return out
}

// Revoke marks a certificate revoked and removes it from live service.
func (r *Registry) Revoke(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for line, c := range r.byLine {
		if c.ID.String() == id {
			delete(r.byLine, line)
			r.revoked[id] = struct{}{}
			return true
		}
	}
	r.revoked[id] = struct{}{}
	return false
}

// IsRevoked reports whether a certificate was explicitly revoked.
func (r *Registry) IsRevoked(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.revoked[id]
	return ok
}

// RevokeGeneration removes every certificate that does not belong to the
// current product revision, so a new grade generation cannot inherit a
// cleaning proof issued for the previous grade. It returns the removed
// certificates so the caller can persist the revocations.
func (r *Registry) RevokeGeneration(revision string) []model.Clearance {
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := make([]model.Clearance, 0)
	for line, c := range r.byLine {
		if c.Revision.String() != revision {
			removed = append(removed, c)
			delete(r.byLine, line)
			r.revoked[c.ID.String()] = struct{}{}
		}
	}
	return removed
}

// Reset empties the registry, returning everything that was live.
func (r *Registry) Reset() []model.Clearance {
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := make([]model.Clearance, 0, len(r.byLine))
	for line, c := range r.byLine {
		removed = append(removed, c)
		r.revoked[c.ID.String()] = struct{}{}
		delete(r.byLine, line)
	}
	return removed
}

// Lines returns the tank lines that currently hold a certificate.
func (r *Registry) Lines() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.byLine))
	for line := range r.byLine {
		out = append(out, line)
	}
	return out
}
