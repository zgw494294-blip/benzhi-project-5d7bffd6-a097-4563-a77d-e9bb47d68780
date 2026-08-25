package httpapi

import (
	"net/http"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/service"
)

func (a *API) HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeData(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) AddWellsBatchHandler(w http.ResponseWriter, r *http.Request) {
	var cmd service.AddWellsBatchCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	if len(cmd.Items) > 100 || len(cmd.Wells) > 100 {
		writeError(w, r, domain.FieldError("items", "单次最多登记 100 口监测井"))
		return
	}
	result, err := a.service.AddWellsBatch(r.Context(), r.PathValue("campaignID"), cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusCreated, result)
}

func (a *API) CreateCampaignHandler(w http.ResponseWriter, r *http.Request) {
	var cmd service.CreateCampaignCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.service.CreateCampaign(r.Context(), cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusCreated, result)
}

func (a *API) CampaignDetailHandler(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.CampaignDetail(r.Context(), r.PathValue("campaignID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, result)
}

func (a *API) AddWellHandler(w http.ResponseWriter, r *http.Request) {
	var cmd service.AddWellCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.service.AddWell(r.Context(), r.PathValue("campaignID"), cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusCreated, result)
}
