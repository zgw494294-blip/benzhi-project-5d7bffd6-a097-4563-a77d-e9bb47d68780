package httpapi

import (
	"net/http"

	"groundwater-release/internal/service"
)

type API struct {
	service *service.Service
	mux     *http.ServeMux
}

func New(s *service.Service) *API {
	a := &API{service: s, mux: http.NewServeMux()}
	a.routes()
	return a
}

func (a *API) Handler() http.Handler { return RequestContext(a.mux) }

func (a *API) routes() {
	a.mux.HandleFunc("GET /healthz", a.HealthHandler)
	a.mux.HandleFunc("POST /api/v1/campaigns", a.CreateCampaignHandler)
	a.mux.HandleFunc("GET /api/v1/campaigns/{campaignID}", a.CampaignDetailHandler)
	a.mux.HandleFunc("POST /api/v1/campaigns/{campaignID}/wells", a.AddWellHandler)
	a.mux.HandleFunc("POST /api/v1/campaigns/{campaignID}/wells:batch", a.AddWellsBatchHandler)
	a.mux.HandleFunc("POST /api/v1/campaigns/{campaignID}/samples", a.AddSampleHandler)
	a.mux.HandleFunc("PATCH /api/v1/campaigns/{campaignID}/samples/{sampleID}", a.ReviseSampleHandler)
	a.mux.HandleFunc("POST /api/v1/campaigns/{campaignID}/checks", a.RunQualityCheckHandler)
	a.mux.HandleFunc("GET /api/v1/campaigns/{campaignID}/checks", a.CheckHistoryHandler)
	a.mux.HandleFunc("POST /api/v1/campaigns/{campaignID}/checks:reopen", a.ReopenCheckHandler)
	a.mux.HandleFunc("POST /api/v1/campaigns/{campaignID}/exceptions/{exceptionID}/evidence", a.AddEvidenceHandler)
	a.mux.HandleFunc("POST /api/v1/campaigns/{campaignID}/exceptions/{exceptionID}/review", a.ReviewExceptionHandler)
	a.mux.HandleFunc("POST /api/v1/campaigns/{campaignID}/exceptions/{exceptionID}/evidence/{revisionAction}", a.WithdrawEvidenceHandler)
	a.mux.HandleFunc("GET /api/v1/campaigns/{campaignID}/approval-readiness", a.ApprovalReadinessHandler)
	a.mux.HandleFunc("POST /api/v1/campaigns/{campaignID}/approve", a.ApproveHandler)
	a.mux.HandleFunc("POST /api/v1/campaigns/{campaignID}/freeze", a.FreezeHandler)
	a.mux.HandleFunc("POST /api/v1/campaigns/{campaignID}/credentials", a.IssueCredentialHandler)
	a.mux.HandleFunc("GET /api/v1/campaigns/{campaignID}/credentials/verification", a.CredentialVerificationHandler)
}
