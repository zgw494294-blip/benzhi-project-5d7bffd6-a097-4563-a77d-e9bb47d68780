package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"groundwater-release/internal/domain"
	"groundwater-release/internal/service"
)

func (a *API) ReopenCheckHandler(w http.ResponseWriter, r *http.Request) {
	var cmd service.ReopenCheckCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.service.ReopenCheck(r.Context(), r.PathValue("campaignID"), cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, result)
}

func (a *API) CheckHistoryHandler(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.Atoi(defaultQuery(r, "limit", "20"))
	if err != nil {
		writeError(w, r, domain.FieldError("limit", "limit 必须是整数"))
		return
	}
	offset, err := strconv.Atoi(defaultQuery(r, "offset", "0"))
	if err != nil {
		writeError(w, r, domain.FieldError("offset", "offset 必须是整数"))
		return
	}
	fromID, toID := r.URL.Query().Get("fromCheckId"), r.URL.Query().Get("toCheckId")
	if fromID == "" {
		fromID = r.URL.Query().Get("baseCheckId")
	}
	if toID == "" {
		toID = r.URL.Query().Get("targetCheckId")
	}
	result, err := a.service.CheckHistory(r.Context(), r.PathValue("campaignID"), limit, offset, fromID, toID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, result)
}

func defaultQuery(r *http.Request, key, value string) string {
	if found := r.URL.Query().Get(key); found != "" {
		return found
	}
	return value
}

func (a *API) RunQualityCheckHandler(w http.ResponseWriter, r *http.Request) {
	var cmd service.RunCheckCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.service.RunQualityCheck(r.Context(), r.PathValue("campaignID"), cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, result)
}

func (a *API) AddEvidenceHandler(w http.ResponseWriter, r *http.Request) {
	var cmd service.AddEvidenceCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.service.AddEvidence(r.Context(), r.PathValue("campaignID"), r.PathValue("exceptionID"), cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, result)
}

func (a *API) ReviewExceptionHandler(w http.ResponseWriter, r *http.Request) {
	var cmd service.ReviewExceptionCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.service.ReviewException(r.Context(), r.PathValue("campaignID"), r.PathValue("exceptionID"), cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, result)
}

func (a *API) WithdrawEvidenceHandler(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("revisionAction")
	if !strings.HasSuffix(action, ":withdraw") {
		writeError(w, r, domain.NewError(domain.ErrNotFound, "证据操作不存在"))
		return
	}
	revision, err := strconv.ParseInt(strings.TrimSuffix(action, ":withdraw"), 10, 64)
	if err != nil || revision < 1 {
		writeError(w, r, domain.FieldError("revision", "证据 revision 必须为正整数"))
		return
	}
	var cmd service.WithdrawEvidenceCommand
	if err = decodeJSON(w, r, &cmd); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := a.service.WithdrawEvidence(r.Context(), r.PathValue("campaignID"), r.PathValue("exceptionID"), revision, cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, result)
}
