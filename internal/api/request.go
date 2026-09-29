package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

type transitionRequest struct {
	Grade  string `json:"grade"`
	Source string `json:"source"`
}
type sampleRequest struct {
	Value float64 `json:"value"`
	At    int64   `json:"at"`
}
type routeRequest struct {
	Target string `json:"target"`
	Batch  string `json:"batch"`
}
type leaseRequest struct {
	Owner  string  `json:"owner"`
	Amount float64 `json:"amount"`
}
type clearanceRequest struct {
	Line         string  `json:"line"`
	Batch        string  `json:"batch"`
	RinseFlow    float64 `json:"rinse_flow"`
	Conductivity float64 `json:"outlet_conductivity"`
	SamplePass   *bool   `json:"sample_pass"`
	ApprovedBy   string  `json:"approved_by"`
}
type revokeClearanceRequest struct {
	Line string `json:"line"`
}

func decodeJSON(r *http.Request, value any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	return nil
}

func boolQuery(r *http.Request, key string) (bool, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, errors.New("invalid boolean query")
	}
	return value, nil
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}
