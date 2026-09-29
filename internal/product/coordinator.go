package product

import (
	"errors"
	"github.com/wyw14/solo-039/internal/analyzer"
	"github.com/wyw14/solo-039/internal/campaign"
	"github.com/wyw14/solo-039/internal/journal"
	"github.com/wyw14/solo-039/internal/model"
)

type Coordinator struct {
	Route      *Route
	Campaign   *campaign.State
	Analyzer   *analyzer.State
	Journal    *journal.Store
	Clearances *ClearanceBook
	proof      model.Proof
}

func NewCoordinator(c *campaign.State, a *analyzer.State, j *journal.Store, clearances *ClearanceBook) *Coordinator {
	return &Coordinator{Route: &Route{}, Campaign: c, Analyzer: a, Journal: j, Clearances: clearances}
}

// requireClearance is the cleaning gate. A product line can never open without
// a cleaning release bound to this exact tank line (target) and production
// batch (revision), approved for the target grade.
func (c *Coordinator) requireClearance(plan model.ProductPlan) error {
	if c.Clearances == nil {
		return errors.New("cleaning clearance book is unavailable")
	}
	_, err := c.Clearances.Require(plan.Target, plan.Revision.String(), plan.Grade)
	return err
}

func (c *Coordinator) Publish(plan model.ProductPlan) error {
	if plan.Campaign != c.Campaign.Snapshot().ID {
		return errors.New("campaign mismatch")
	}
	if err := c.requireClearance(plan); err != nil {
		return err
	}
	proof, ok := c.Analyzer.Proof(plan.Revision)
	if !ok {
		return errors.New("composition proof incomplete")
	}
	if err := journal.PersistProof(c.Journal, proof, c.Campaign.Snapshot()); err != nil {
		return err
	}
	c.proof = proof
	return c.Route.Open(plan)
}

func (c *Coordinator) Recover(plan model.ProductPlan) error {
	if err := c.requireClearance(plan); err != nil {
		return err
	}
	if err := journal.RequireProof(c.Journal, plan.Revision.String()); err != nil {
		return err
	}
	return c.Route.Open(plan)
}

func (c *Coordinator) PublishWithProof(plan model.ProductPlan, proof model.Proof) error {
	if plan.Campaign != c.Campaign.Snapshot().ID {
		return errors.New("campaign mismatch")
	}
	if err := c.requireClearance(plan); err != nil {
		return err
	}
	if proof.Revision != plan.Revision || proof.Digest == "" {
		return errors.New("proof does not match product revision")
	}
	if err := journal.PersistProof(c.Journal, proof, c.Campaign.Snapshot()); err != nil {
		return err
	}
	c.proof = proof
	return c.Route.Open(plan)
}

func (c *Coordinator) Proof() model.Proof { return c.proof }
