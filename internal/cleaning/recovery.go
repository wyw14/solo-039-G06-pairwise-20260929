package cleaning

import (
	"errors"

	"github.com/wyw14/solo-039/internal/journal"
	"github.com/wyw14/solo-039/internal/model"
)

// Restore rebuilds the live clearance registry by replaying the durable
// cleaning events in order. Revocations and superseding certificates are
// honoured, so a proof withdrawn before a restart cannot be brought back.
func Restore(registry *Registry, events []model.Event) {
	live := make(map[string]model.Clearance) // certificate id -> clearance
	lineOwner := make(map[string]string)     // line -> live certificate id
	for _, event := range events {
		switch event.Kind {
		case "cleaning.clearance":
			if clearance, ok := journal.DecodeClearance(event); ok {
				live[clearance.ID.String()] = clearance
				lineOwner[clearance.Line] = clearance.ID.String()
			}
		case "cleaning.revoked":
			if id, ok := journal.DecodeClearanceRevocation(event); ok {
				if clearance, exists := live[id]; exists {
					delete(lineOwner, clearance.Line)
					delete(live, id)
				}
			}
		}
	}
	for _, id := range lineOwner {
		registry.Save(live[id])
	}
}

// BuildRecovery replays the journal onto a fresh registry for the current
// campaign and reports which clearances are still live for its generation.
func BuildRecovery(events []model.Event, current model.Campaign) Recovery {
	fresh := NewRegistry()
	Restore(fresh, events)
	live := fresh.All()
	out := Recovery{Events: append([]model.Event(nil), events...)}
	for _, clearance := range live {
		out.Restored = append(out.Restored, clearance)
		if clearance.Matches(clearance.Line, clearance.Batch, current) {
			out.GenerationLive = append(out.GenerationLive, clearance)
		}
	}
	return out
}

// Recovery is the replay result used during restart validation.
type Recovery struct {
	Events         []model.Event
	Restored       []model.Clearance
	GenerationLive []model.Clearance
}

// HasLive reports whether a specific line currently holds a live certificate.
func (r Recovery) HasLive(line string) bool {
	for _, clearance := range r.Restored {
		if clearance.Line == line {
			return true
		}
	}
	return false
}

// Validate ensures every snapshot certificate is backed by a durable,
// non-revoked event that still belongs to the restored campaign generation.
// A snapshot that advertises a missing or stale clearance must not be trusted
// to open a line.
func (r Recovery) Validate(snapshot []model.Clearance, current model.Campaign) error {
	live := make(map[string]bool, len(r.GenerationLive))
	for _, clearance := range r.GenerationLive {
		live[clearance.ID.String()] = true
	}
	for _, clearance := range snapshot {
		if !live[clearance.ID.String()] {
			return errors.New("snapshot clearance has no live cleaning proof for this generation")
		}
	}
	return nil
}

// RestoreFor replays the durable events and returns a registry containing only
// the clearances that are still live for the restored campaign generation,
// together with the full recovery report.
func RestoreFor(events []model.Event, current model.Campaign) (*Registry, Recovery) {
	recovery := BuildRecovery(events, current)
	registry := NewRegistry()
	for _, clearance := range recovery.GenerationLive {
		registry.Save(clearance)
	}
	return registry, recovery
}
