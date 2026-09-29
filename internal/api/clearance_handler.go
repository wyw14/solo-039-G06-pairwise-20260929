package api

import (
	"net/http"
)

// approveClearance records a tank-line cleaning release (rinse flush flow,
// outlet conductivity, sample result) for the current production batch and
// target grade.
func (s *Server) approveClearance(w http.ResponseWriter, r *http.Request) {
	var input clearanceRequest
	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	clearance, err := s.plant.ApproveLineClearance(input.Line, input.FlushFlow, input.Conductivity, input.SamplePassed)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":       "cleared",
		"line":         clearance.Line,
		"batch":        clearance.Batch,
		"target_grade": clearance.TargetGrade,
		"flush_flow":   clearance.FlushFlow,
		"conductivity": clearance.Conductivity,
		"sample":       clearance.SamplePassed,
		"digest":       clearance.Digest,
	})
}

// clearance reports the cleaning certificate (if any) bound exactly to the
// given tank line and batch.
func (s *Server) clearance(w http.ResponseWriter, r *http.Request) {
	line := r.URL.Query().Get("line")
	batch := r.URL.Query().Get("batch")
	c, ok := s.plant.LineClearance(line, batch)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no cleaning clearance for this tank line and batch"})
		return
	}
	writeJSON(w, http.StatusOK, c)
}
