package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"groundwater-release/internal/domain"
)

const maxBodyBytes = 1 << 20

type responseEnvelope struct {
	RequestID string     `json:"requestId"`
	Data      any        `json:"data,omitempty"`
	Error     *errorBody `json:"error,omitempty"`
}
type errorBody struct {
	Code    string                  `json:"code"`
	Message string                  `json:"message"`
	Field   string                  `json:"field,omitempty"`
	Items   []domain.ItemFieldError `json:"items,omitempty"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return domain.FieldError("body", "JSON 请求体无效: "+err.Error())
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return domain.FieldError("body", "请求体只能包含一个 JSON 对象")
	}
	return nil
}

func writeData(w http.ResponseWriter, r *http.Request, status int, value any) {
	writeJSON(w, status, responseEnvelope{RequestID: requestID(r), Data: value})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	body := &errorBody{Code: "internal_error", Message: "服务内部错误"}
	var de *domain.DomainError
	var batch *domain.BatchValidationError
	if errors.As(err, &batch) {
		writeJSON(w, http.StatusBadRequest, responseEnvelope{RequestID: requestID(r), Error: &errorBody{Code: string(domain.ErrValidation), Message: batch.Error(), Items: batch.Items}})
		return
	}
	if errors.As(err, &de) {
		body = &errorBody{Code: string(de.Code), Message: de.Message, Field: de.Field}
		switch de.Code {
		case domain.ErrValidation:
			status = http.StatusBadRequest
		case domain.ErrNotFound:
			status = http.StatusNotFound
		case domain.ErrForbidden:
			status = http.StatusForbidden
		case domain.ErrConflict, domain.ErrState, domain.ErrStaleVersion:
			status = http.StatusConflict
		case domain.ErrIntegrity:
			status = http.StatusUnprocessableEntity
		}
	}
	writeJSON(w, status, responseEnvelope{RequestID: requestID(r), Error: body})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
