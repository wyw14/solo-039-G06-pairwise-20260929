package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// RinseRecord is the cleaning evidence collected while flushing a tank line:
// the cumulative rinse volume/flow, the conductivity measured at the line
// outlet and the QC sample taken from that outlet.
type RinseRecord struct {
	Flow         float64 // cumulative rinse volume (or flow integral)
	Conductivity float64 // outlet conductivity, microSiemens per unit volume
	SamplePass   bool    // QC sampling verdict on the outlet rinse
	Sample       Sample  // optional sample detail backing SamplePass
}

// Clearance is the cleaning release certificate. It is bound to one concrete
// tank line, one production batch and one target grade, and carries the
// campaign generation it was issued under. A certificate never transfers:
// switching the tank line, the batch or the product generation invalidates it.
type Clearance struct {
	ID          uuid.UUID
	Line        string // tank line the certificate is bound to
	Batch       string // production batch the certificate is bound to
	TargetGrade string // grade the line is cleared to run
	Campaign    uuid.UUID
	Revision    uuid.UUID
	Rinse       RinseRecord
	ApprovedBy  string
	At          int64
}

// CleaningLimits define when the rinse evidence is good enough to release a
// tank line for a target grade.
type CleaningLimits struct {
	MinRinseFlow      float64
	MaxConductivity   float64
	RequireSamplePass bool
}

// DefaultCleaningLimits returns the plant default cleaning acceptance window.
func DefaultCleaningLimits() CleaningLimits {
	return CleaningLimits{MinRinseFlow: 10, MaxConductivity: 5, RequireSamplePass: true}
}

// ValidateLine verifies the tank line identity is recorded.
func ValidateLine(line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return errors.New("tank line is required")
	}
	if len(line) > 48 {
		return errors.New("tank line is too long")
	}
	return nil
}

// ValidateBatch verifies the production batch identity is recorded.
func ValidateBatch(batch string) error {
	batch = strings.TrimSpace(batch)
	if batch == "" {
		return errors.New("batch is required")
	}
	if len(batch) > 48 {
		return errors.New("batch is too long")
	}
	return nil
}

// Validate checks the certificate identity bindings.
func (c Clearance) Validate() error {
	if c.ID == uuid.Nil {
		return errors.New("clearance identity is required")
	}
	if c.Campaign == uuid.Nil || c.Revision == uuid.Nil {
		return errors.New("clearance generation is required")
	}
	if err := ValidateLine(c.Line); err != nil {
		return err
	}
	if err := ValidateBatch(c.Batch); err != nil {
		return err
	}
	if err := ValidateGrade(c.TargetGrade); err != nil {
		return fmt.Errorf("clearance target grade: %w", err)
	}
	if c.At <= 0 {
		return errors.New("clearance timestamp is required")
	}
	return nil
}

// Acceptable evaluates the rinse evidence against the cleaning limits.
func (l CleaningLimits) Acceptable(record RinseRecord) error {
	if record.Flow < l.MinRinseFlow {
		return errors.New("rinse flow below cleaning minimum")
	}
	if record.Conductivity < 0 {
		return errors.New("outlet conductivity is not numeric")
	}
	if record.Conductivity > l.MaxConductivity {
		return errors.New("outlet conductivity above cleaning limit")
	}
	if l.RequireSamplePass && !record.SamplePass {
		return errors.New("rinse sample did not pass")
	}
	return nil
}

// Matches reports whether the certificate may be used to open the given tank
// line for a batch and product generation. Every binding must agree; an old
// certificate from another line, batch or generation must not be reused.
func (c Clearance) Matches(line, batch string, campaign Campaign) bool {
	return c.Line == line &&
		c.Batch == batch &&
		c.TargetGrade == campaign.Grade &&
		c.Campaign == campaign.ID &&
		c.Revision == campaign.Revision
}

// ClearanceDigest is a tamper-evident summary covering every binding and the
// rinse evidence, so a persisted certificate cannot be silently altered.
func ClearanceDigest(c Clearance) string {
	value := fmt.Sprintf(
		"%s|%s|%s|%s|%s|%s|%.6f|%.6f|%t|%s|%d",
		c.ID, c.Line, c.Batch, c.TargetGrade, c.Campaign, c.Revision,
		c.Rinse.Flow, c.Rinse.Conductivity, c.Rinse.SamplePass, c.ApprovedBy, c.At,
	)
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
