package httpapi

import (
	"net/http"

	"groundwater-release/internal/service"
)

func (a *API) ApproveHandler(w http.ResponseWriter, r *http.Request) {
	var cmd service.ApproveCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.service.Approve(r.Context(), r.PathValue("campaignID"), cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, result)
}

func (a *API) FreezeHandler(w http.ResponseWriter, r *http.Request) {
	var cmd service.FreezeCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.service.Freeze(r.Context(), r.PathValue("campaignID"), cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, result)
}

func (a *API) IssueCredentialHandler(w http.ResponseWriter, r *http.Request) {
	var cmd service.IssueCredentialCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.service.IssueCredential(r.Context(), r.PathValue("campaignID"), cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusCreated, result)
}

func (a *API) ApprovalReadinessHandler(w http.ResponseWriter, r *http.Request) {
	actor := r.URL.Query().Get("actor")
	if actor == "" {
		actor = r.Header.Get("X-Actor")
	}
	result, err := a.service.ApprovalReadiness(r.Context(), r.PathValue("campaignID"), actor)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, result)
}

func (a *API) CredentialVerificationHandler(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.CredentialVerification(r.Context(), r.PathValue("campaignID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, result)
}
