package journal

import (
	"encoding/json"
	"errors"
	"github.com/wyw14/solo-039/internal/model"
)

// PersistClearance durably records an approved tank-line cleaning release as a
// cleaning.cleared event. The JSON payload carries the full certificate so it
// can be rebuilt on recovery and re-verified against its exact line and batch.
func PersistClearance(store *Store, clearance model.CleaningClearance, campaign model.Campaign) (model.Event, error) {
	if !clearance.Approved || clearance.Digest == "" {
		return model.Event{}, errors.New("cleaning clearance is not an approved release")
	}
	if clearance.Batch != campaign.Revision {
		return model.Event{}, errors.New("cleaning clearance batch does not match campaign generation")
	}
	payload, err := json.Marshal(clearance)
	if err != nil {
		return model.Event{}, err
	}
	event := model.NewEvent("cleaning.cleared", campaign, string(payload))
	if err := store.Append(event); err != nil {
		return model.Event{}, err
	}
	return event, nil
}

// FindClearance rebuilds the durable cleaning certificate bound exactly to the
// given tank line and batch (campaign revision). A certificate for another
// line or another batch is never returned — switching tanks cannot reuse it.
func FindClearance(events []model.Event, line, batch string) (model.CleaningClearance, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Kind != "cleaning.cleared" {
			continue
		}
		var clearance model.CleaningClearance
		if err := json.Unmarshal([]byte(event.Payload), &clearance); err != nil {
			continue
		}
		if clearance.Line == line && clearance.Batch.String() == batch {
			return clearance, true
		}
	}
	return model.CleaningClearance{}, false
}

// RequireClearance fails closed when no durable certificate authorizes the
// given tank line, batch and target grade.
func RequireClearance(events []model.Event, line, batch, grade string) (model.CleaningClearance, error) {
	clearance, ok := FindClearance(events, line, batch)
	if !ok {
		return model.CleaningClearance{}, errors.New("durable cleaning clearance not found for tank line")
	}
	if !clearance.Approved || clearance.Line != line || clearance.Batch.String() != batch || clearance.TargetGrade != grade {
		return model.CleaningClearance{}, errors.New("durable cleaning clearance does not authorize this release")
	}
	return clearance, nil
}

// LoadClearances rebuilds every durable cleaning certificate from an event
// history, newest last. Unapproved or malformed records are skipped.
func LoadClearances(events []model.Event) []model.CleaningClearance {
	out := make([]model.CleaningClearance, 0)
	for _, event := range events {
		if event.Kind != "cleaning.cleared" {
			continue
		}
		var clearance model.CleaningClearance
		if err := json.Unmarshal([]byte(event.Payload), &clearance); err != nil {
			continue
		}
		if !clearance.Approved || clearance.Digest == "" {
			continue
		}
		out = append(out, clearance)
	}
	return out
}
