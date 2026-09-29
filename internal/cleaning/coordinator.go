package cleaning

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/wyw14/solo-039/internal/campaign"
	"github.com/wyw14/solo-039/internal/journal"
	"github.com/wyw14/solo-039/internal/model"
)

// ReleaseRequest carries the rinse evidence and the binding for a cleaning
// release: which tank line was flushed, which production batch it is cleared
// for, and the measured rinse results.
type ReleaseRequest struct {
	Line       string
	Batch      string
	Rinse      model.RinseRecord
	ApprovedBy string
}

// Coordinator issues tank-line cleaning clearances and gates product routing
// on them. No product line may open without a live certificate that matches
// the line, batch and current product generation.
type Coordinator struct {
	Registry *Registry
	Campaign *campaign.State
	Journal  *journal.Store
	Limits   model.CleaningLimits
	Audit    *Audit
	// Sink, when set, receives each durable cleaning event so it can be mirrored to
	// a durable file log. A sink failure rolls the release back.
	Sink func(model.Event) error
	last model.Clearance
}

func NewCoordinator(c *campaign.State, j *journal.Store) *Coordinator {
	return &Coordinator{
		Registry: NewRegistry(),
		Campaign: c,
		Journal:  j,
		Limits:   model.DefaultCleaningLimits(),
		Audit:    NewAudit(),
	}
}

// Release evaluates the rinse evidence and, only when it is complete and
// within limits, approves a certificate bound to the tank line, batch and
// target grade of the current campaign.
func (c *Coordinator) Release(req ReleaseRequest) (model.Clearance, error) {
	current := c.Campaign.Snapshot()
	if err := model.ValidateLine(req.Line); err != nil {
		return model.Clearance{}, err
	}
	if err := model.ValidateBatch(req.Batch); err != nil {
		return model.Clearance{}, err
	}
	if err := c.Limits.Acceptable(req.Rinse); err != nil {
		c.Audit.Record("release", req.Line, req.Batch, model.Clearance{}, false, err.Error())
		return model.Clearance{}, err
	}
	clearance := model.Clearance{
		ID:          uuid.New(),
		Line:        req.Line,
		Batch:       req.Batch,
		TargetGrade: current.Grade,
		Campaign:    current.ID,
		Revision:    current.Revision,
		Rinse:       req.Rinse,
		ApprovedBy:  req.ApprovedBy,
		At:          time.Now().Unix(),
	}
	if err := clearance.Validate(); err != nil {
		c.Audit.Record("release", req.Line, req.Batch, clearance, false, err.Error())
		return model.Clearance{}, err
	}
	event, err := journal.PersistClearance(c.Journal, clearance, current)
	if err != nil {
		c.Audit.Record("release", req.Line, req.Batch, clearance, false, err.Error())
		return model.Clearance{}, err
	}
	if err := c.sink(event); err != nil {
		c.Audit.Record("release", req.Line, req.Batch, clearance, false, err.Error())
		return model.Clearance{}, err
	}
	c.Registry.Save(clearance)
	c.last = clearance
	c.Audit.Record("release", req.Line, req.Batch, clearance, true, "rinse evidence accepted")
	return clearance, nil
}

// sink mirrors a durable cleaning event to the configured durable file log.
func (c *Coordinator) sink(event model.Event) error {
	if c.Sink == nil {
		return nil
	}
	return c.Sink(event)
}

// Authorize is the gate called before a product line is opened. It requires a
// live certificate whose every binding matches the requested line, batch and
// the current campaign generation. A missing, revoked or stale certificate
// keeps the line shut.
func (c *Coordinator) Authorize(line, batch string) (model.Clearance, error) {
	current := c.Campaign.Snapshot()
	clearance, ok := c.Registry.Get(line)
	if !ok {
		err := errors.New("tank line has no cleaning clearance")
		c.Audit.Record("authorize", line, batch, model.Clearance{}, false, err.Error())
		return model.Clearance{}, err
	}
	if c.Registry.IsRevoked(clearance.ID.String()) {
		err := errors.New("cleaning clearance has been revoked")
		c.Audit.Record("authorize", line, batch, clearance, false, err.Error())
		return model.Clearance{}, err
	}
	if clearance.Batch != batch {
		err := errors.New("cleaning clearance is bound to another batch")
		c.Audit.Record("authorize", line, batch, clearance, false, err.Error())
		return model.Clearance{}, err
	}
	if clearance.TargetGrade != current.Grade {
		err := errors.New("cleaning clearance was released for another grade")
		c.Audit.Record("authorize", line, batch, clearance, false, err.Error())
		return model.Clearance{}, err
	}
	if clearance.Campaign != current.ID || clearance.Revision != current.Revision {
		err := errors.New("cleaning clearance belongs to another product generation")
		c.Audit.Record("authorize", line, batch, clearance, false, err.Error())
		return model.Clearance{}, err
	}
	if err := journal.RequireClearanceLive(c.Journal, clearance.ID.String()); err != nil {
		c.Audit.Record("authorize", line, batch, clearance, false, err.Error())
		return model.Clearance{}, err
	}
	c.Audit.Record("authorize", line, batch, clearance, true, "line cleared for product")
	return clearance, nil
}

// Revoke withdraws the live certificate of a tank line and records the
// withdrawal. A revoked certificate can never authorize a line again.
func (c *Coordinator) Revoke(line string) error {
	clearance, ok := c.Registry.Get(line)
	if !ok {
		return errors.New("tank line has no cleaning clearance")
	}
	if err := c.revokeOne(clearance, c.Campaign.Snapshot()); err != nil {
		return err
	}
	c.Audit.Record("revoke", line, clearance.Batch, clearance, false, "clearance revoked")
	return nil
}

// revokeOne persists and applies a single withdrawal.
func (c *Coordinator) revokeOne(clearance model.Clearance, current model.Campaign) error {
	event, err := journal.RevokeClearance(c.Journal, clearance.ID.String(), current)
	if err != nil {
		return err
	}
	if err := c.sink(event); err != nil {
		return err
	}
	c.Registry.Revoke(clearance.ID.String())
	return nil
}

// InvalidateGeneration retires every certificate that predates the current
// product revision. It is used on grade transition so a new grade cannot
// inherit the previous grade's cleaning proof.
func (c *Coordinator) InvalidateGeneration() []model.Clearance {
	current := c.Campaign.Snapshot()
	removed := c.Registry.RevokeGeneration(current.Revision.String())
	for _, clearance := range removed {
		_ = c.revokeOne(clearance, current)
		c.Audit.Record("revoke", clearance.Line, clearance.Batch, clearance, false, "superseded by new product generation")
	}
	return removed
}

// InvalidateAll retires every live certificate. It is used when a campaign is
// started, so a new run always begins with clean lines that must be released
// again rather than inheriting proofs from a previous run.
func (c *Coordinator) InvalidateAll() []model.Clearance {
	current := c.Campaign.Snapshot()
	removed := c.Registry.Reset()
	for _, clearance := range removed {
		_ = c.revokeOne(clearance, current)
		c.Audit.Record("revoke", clearance.Line, clearance.Batch, clearance, false, "superseded by new campaign run")
	}
	return removed
}

// Last returns the most recently issued certificate.
func (c *Coordinator) Last() model.Clearance { return c.last }
