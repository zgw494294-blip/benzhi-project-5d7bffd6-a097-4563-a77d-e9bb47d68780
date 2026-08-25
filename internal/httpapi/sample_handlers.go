package httpapi

import (
	"net/http"

	"groundwater-release/internal/service"
)

func (a *API) AddSampleHandler(w http.ResponseWriter, r *http.Request) {
	var cmd service.AddSampleCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.service.AddSample(r.Context(), r.PathValue("campaignID"), cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusCreated, result)
}

func (a *API) ReviseSampleHandler(w http.ResponseWriter, r *http.Request) {
	var cmd service.ReviseSampleCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.service.ReviseSample(r.Context(), r.PathValue("campaignID"), r.PathValue("sampleID"), cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, result)
}
