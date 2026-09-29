package journal

import (
	"encoding/json"
	"errors"
	"github.com/wyw14/solo-039/internal/model"
)

func PersistProof(store *Store, proof model.Proof, campaign model.Campaign) error {
	if proof.Digest == "" {
		proof.Digest = model.ProofDigest(proof)
	}
	event := model.NewEvent("proof.durable", campaign, proof.Digest)
	if err := store.Append(event); err != nil {
		return err
	}
	return nil
}

func RequireProof(store *Store, revision string) error {
	event, ok := store.Last("proof.durable")
	if !ok || event.Revision.String() != revision {
		return errors.New("proof is not durable")
	}
	return nil
}

// clearanceEnvelope is the durable, replayable record of a cleaning clearance
// certificate: the full evidence plus a tamper-evident digest.
type clearanceEnvelope struct {
	Clearance model.Clearance `json:"clearance"`
	Digest    string          `json:"digest"`
}

// PersistClearance durably records that a tank line was cleared for a batch
// and target grade, bound to the campaign generation that approved it. It
// returns the appended event so callers can mirror it to a durable file log.
func PersistClearance(store *Store, clearance model.Clearance, campaign model.Campaign) (model.Event, error) {
	envelope := clearanceEnvelope{Clearance: clearance, Digest: model.ClearanceDigest(clearance)}
	data, err := json.Marshal(envelope)
	if err != nil {
		return model.Event{}, err
	}
	event := model.NewEvent("cleaning.clearance", campaign, string(data))
	if err := store.Append(event); err != nil {
		return model.Event{}, err
	}
	return event, nil
}

type clearanceRevocation struct {
	ID string `json:"id"`
}

// RevokeClearance durably records the withdrawal of a clearance certificate.
// It returns the appended event so callers can mirror it to a durable file log.
func RevokeClearance(store *Store, id string, campaign model.Campaign) (model.Event, error) {
	data, err := json.Marshal(clearanceRevocation{ID: id})
	if err != nil {
		return model.Event{}, err
	}
	event := model.NewEvent("cleaning.revoked", campaign, string(data))
	if err := store.Append(event); err != nil {
		return model.Event{}, err
	}
	return event, nil
}

// DecodeClearance extracts a clearance certificate from a durable event,
// verifying its digest. It returns ok=false for unrelated or corrupt events.
func DecodeClearance(event model.Event) (model.Clearance, bool) {
	if event.Kind != "cleaning.clearance" {
		return model.Clearance{}, false
	}
	var envelope clearanceEnvelope
	if err := json.Unmarshal([]byte(event.Payload), &envelope); err != nil {
		return model.Clearance{}, false
	}
	if envelope.Digest == "" || model.ClearanceDigest(envelope.Clearance) != envelope.Digest {
		return model.Clearance{}, false
	}
	return envelope.Clearance, true
}

// DecodeClearanceRevocation returns the revoked certificate id.
func DecodeClearanceRevocation(event model.Event) (string, bool) {
	if event.Kind != "cleaning.revoked" {
		return "", false
	}
	var payload clearanceRevocation
	if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
		return "", false
	}
	return payload.ID, payload.ID != ""
}

// ClearanceLive reports whether a clearance event exists for the id and no
// later revocation of that id was recorded.
func ClearanceLive(store *Store, id string) bool {
	issued := false
	for _, event := range store.Events() {
		switch event.Kind {
		case "cleaning.clearance":
			if clearance, ok := DecodeClearance(event); ok && clearance.ID.String() == id {
				issued = true
			}
		case "cleaning.revoked":
			if revoked, ok := DecodeClearanceRevocation(event); ok && revoked == id {
				issued = false
			}
		}
	}
	return issued
}

// RequireClearanceLive returns an error unless the certificate is durably
// issued and has not been revoked.
func RequireClearanceLive(store *Store, id string) error {
	if !ClearanceLive(store, id) {
		return errors.New("cleaning clearance is not durable")
	}
	return nil
}
