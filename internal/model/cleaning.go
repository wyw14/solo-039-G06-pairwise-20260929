package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"strings"
)

// CleaningLimits defines the release thresholds for a tank line rinse.
type CleaningLimits struct {
	MinFlushFlow    float64
	MaxConductivity float64
}

// DefaultCleaningLimits returns the conservative plant defaults: the rinse
// must move enough fluid and the outlet conductivity must approach clean water.
func DefaultCleaningLimits() CleaningLimits {
	return CleaningLimits{MinFlushFlow: 10.0, MaxConductivity: 5.0}
}

// CleaningClearance is the cleaning release certificate for one tank line.
// It is bound to an exact tank line AND production batch (campaign revision):
// switching the tank or the batch invalidates the certificate, and a stale
// certificate from a previous grade can never be reused.
type CleaningClearance struct {
	ID           uuid.UUID
	Line         string
	Batch        uuid.UUID
	Campaign     uuid.UUID
	TargetGrade  string
	FlushFlow    float64
	Conductivity float64
	SamplePassed bool
	Approved     bool
	Digest       string
	At           int64
}

// Validate enforces that every piece of cleaning evidence is present and
// within release limits. A failed sample can never be approved.
func (l CleaningLimits) Validate(c CleaningClearance) error {
	if strings.TrimSpace(c.Line) == "" {
		return errors.New("tank line is required")
	}
	if c.Batch == uuid.Nil {
		return errors.New("cleaning batch is required")
	}
	if c.Campaign == uuid.Nil {
		return errors.New("cleaning campaign is required")
	}
	if strings.TrimSpace(c.TargetGrade) == "" {
		return errors.New("target grade is required")
	}
	if c.FlushFlow <= 0 {
		return errors.New("rinse flush flow is required")
	}
	if c.FlushFlow < l.MinFlushFlow {
		return fmt.Errorf("rinse flush flow %.3f below minimum %.3f", c.FlushFlow, l.MinFlushFlow)
	}
	if c.Conductivity < 0 {
		return errors.New("outlet conductivity is invalid")
	}
	if c.Conductivity > l.MaxConductivity {
		return fmt.Errorf("outlet conductivity %.3f above limit %.3f", c.Conductivity, l.MaxConductivity)
	}
	if !c.SamplePassed {
		return errors.New("rinse sample result is not passing")
	}
	return nil
}

// Approve marks the certificate released for the target grade once every
// reading clears the limits, and stamps a digest that binds the evidence.
func (l CleaningLimits) Approve(c CleaningClearance) (CleaningClearance, error) {
	if err := l.Validate(c); err != nil {
		return CleaningClearance{}, err
	}
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	c.Approved = true
	if c.Digest == "" {
		c.Digest = CleaningDigest(c)
	}
	return c, nil
}

// Authorizes reports whether this certificate may open the given line for the
// given batch and target grade. All three bindings must match exactly so a
// certificate can never follow a tank change or a batch change.
func (c CleaningClearance) Authorizes(line string, batch uuid.UUID, grade string) bool {
	return c.Approved &&
		c.Line == line &&
		c.Batch == batch &&
		c.TargetGrade == grade
}

// CleaningDigest binds the certificate to its line, batch and measured
// evidence; tampering with any reading changes the digest.
func CleaningDigest(c CleaningClearance) string {
	value := fmt.Sprintf("%s|%s|%s|%s|%.6f|%.6f|%t|%d",
		c.ID, c.Line, c.Batch, c.TargetGrade, c.FlushFlow, c.Conductivity, c.SamplePassed, c.At)
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
