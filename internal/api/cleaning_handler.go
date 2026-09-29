package api

import (
	"net/http"

	"github.com/wyw14/solo-039/internal/cleaning"
	"github.com/wyw14/solo-039/internal/model"
)

// releaseClearance records rinse flow, outlet conductivity and the sample
// result, then approves the tank line for the target grade when the evidence
// is complete and within limits.
func (s *Server) releaseClearance(w http.ResponseWriter, r *http.Request) {
	var input clearanceRequest
	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	samplePass := false
	if input.SamplePass != nil {
		samplePass = *input.SamplePass
	}
	req := cleaning.ReleaseRequest{
		Line:       input.Line,
		Batch:      input.Batch,
		ApprovedBy: input.ApprovedBy,
		Rinse: model.RinseRecord{
			Flow:         input.RinseFlow,
			Conductivity: input.Conductivity,
			SamplePass:   samplePass,
		},
	}
	clearance, err := s.plant.ReleaseTankLine(req)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, clearance)
}

// tankLines returns the live cleaning certificate of every tank line.
func (s *Server) tankLines(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"clearances": s.plant.TankLineClearances()})
}

// revokeClearance withdraws a tank line's live cleaning certificate.
func (s *Server) revokeClearance(w http.ResponseWriter, r *http.Request) {
	var input revokeClearanceRequest
	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.plant.RevokeTankLine(input.Line); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"line": input.Line, "revoked": true})
}
