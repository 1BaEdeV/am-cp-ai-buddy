package accessrequest

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	requestservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
)

// RequestService оставляет проверки владельца, ролей и переходов статуса бизнес-слою.
// Подготовка черновика доступна инструменту агента, а не публичному HTTP-маршруту.
type RequestService interface {
	ConfirmAccessRequest(context.Context, string, string) (requestservice.Request, bool, error)
	ListAccessRequests(context.Context, string, requestservice.RequestFilter) (requestservice.RequestList, error)
	GetAccessRequest(context.Context, string, string) (requestservice.Request, error)
	DecideAccessRequest(context.Context, string, string, requestservice.DecisionInput) (requestservice.Request, error)
	RetryAccessIssuance(context.Context, string, string) (requestservice.Request, error)
}

type RequestHandler struct {
	service RequestService
}

func NewRequestHandler(service RequestService) *RequestHandler {
	return &RequestHandler{service: service}
}

// ConfirmDraft принимает явное подтверждение из интерфейса чата.
// Этот обработчик не должен регистрироваться как автономный инструмент LLM.
func (h *RequestHandler) ConfirmDraft(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	var input struct {
		Confirmed bool `json:"confirmed"`
	}
	if !handler.DecodeJSON(w, r, &input) {
		return
	}
	if !input.Confirmed {
		handler.WriteAPIError(w, http.StatusBadRequest, "confirmation_required", "Подтверди отправку выбранного черновика.")
		return
	}
	result, created, err := h.service.ConfirmAccessRequest(r.Context(), employeeID, r.PathValue("draftId"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		w.Header().Set("Location", "/api/v1/access-requests/"+url.PathEscape(result.ID))
	}
	handler.WriteJSON(w, status, result)
}

func (h *RequestHandler) ListRequests(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	filter, err := requestFilter(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	result, err := h.service.ListAccessRequests(r.Context(), employeeID, filter)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	handler.WriteJSON(w, http.StatusOK, result)
}

func (h *RequestHandler) GetRequest(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	result, err := h.service.GetAccessRequest(r.Context(), employeeID, r.PathValue("requestId"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	handler.WriteJSON(w, http.StatusOK, result)
}

func (h *RequestHandler) DecideRequest(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	var input requestservice.DecisionInput
	if !handler.DecodeJSON(w, r, &input) {
		return
	}
	result, err := h.service.DecideAccessRequest(r.Context(), employeeID, r.PathValue("requestId"), input)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	handler.WriteJSON(w, http.StatusOK, result)
}

func (h *RequestHandler) RetryIssuance(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := requestIdentity(w, r)
	if !ok {
		return
	}
	result, err := h.service.RetryAccessIssuance(r.Context(), employeeID, r.PathValue("requestId"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	handler.WriteJSON(w, http.StatusOK, result)
}

func requestIdentity(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.TrimSpace(r.Header.Get("X-Demo-Employee-Id"))
	if id == "" {
		handler.WriteAPIError(w, http.StatusUnauthorized, "demo_identity_required", "Введи демо-почту или внутренний ID.")
		return "", false
	}
	return id, true
}

func requestFilter(r *http.Request) (requestservice.RequestFilter, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return requestservice.RequestFilter{}, requestservice.ErrInvalidInput
	}
	for key, entries := range values {
		if len(entries) != 1 {
			return requestservice.RequestFilter{}, requestservice.ErrInvalidInput
		}
		switch key {
		case "group", "status", "resourceId", "limit", "offset":
		default:
			return requestservice.RequestFilter{}, requestservice.ErrInvalidInput
		}
	}
	filter := requestservice.RequestFilter{
		Group: strings.TrimSpace(values.Get("group")), Status: models.AccessRequestStatus(values.Get("status")),
		ResourceID: strings.TrimSpace(values.Get("resourceId")),
	}
	if values.Has("limit") {
		filter.Limit, err = strconv.Atoi(values.Get("limit"))
		if err != nil || filter.Limit < 1 || filter.Limit > 100 {
			return requestservice.RequestFilter{}, requestservice.ErrInvalidInput
		}
	}
	if values.Has("offset") {
		filter.Offset, err = strconv.Atoi(values.Get("offset"))
		if err != nil || filter.Offset < 0 {
			return requestservice.RequestFilter{}, requestservice.ErrInvalidInput
		}
	}
	return filter, nil
}

func (h *RequestHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrEmployeeNotFound):
		handler.WriteAPIError(w, http.StatusUnauthorized, "demo_identity_required", "Введи демо-почту или внутренний ID.")
	case errors.Is(err, auth.ErrForbidden):
		handler.WriteAPIError(w, http.StatusForbidden, "forbidden", "Действие недоступно для этой роли.")
	case errors.Is(err, auth.ErrProfileRequired):
		handler.WriteAPIError(w, http.StatusForbidden, "profile_required", "Заполни профиль сотрудника.")
	case errors.Is(err, requestservice.ErrInvalidInput):
		handler.WriteAPIError(w, http.StatusBadRequest, "invalid_input", "Проверь параметры запроса.")
	case errors.Is(err, requestservice.ErrDraftNotFound):
		handler.WriteAPIError(w, http.StatusNotFound, "draft_not_found", "Черновик не найден.")
	case errors.Is(err, requestservice.ErrRequestNotFound):
		handler.WriteAPIError(w, http.StatusNotFound, "request_not_found", "Заявка не найдена.")
	case errors.Is(err, requestservice.ErrDraftExpired):
		handler.WriteAPIError(w, http.StatusConflict, "draft_expired", "Срок черновика истёк. Попроси AI-buddy подготовить новый.")
	case errors.Is(err, requestservice.ErrDraftChanged):
		handler.WriteAPIError(w, http.StatusConflict, "draft_changed", "Условия изменились. Попроси AI-buddy подготовить новый черновик.")
	case errors.Is(err, requestservice.ErrAccessAlreadyGranted):
		handler.WriteAPIError(w, http.StatusConflict, "access_already_granted", "Необходимый доступ уже есть.")
	case errors.Is(err, requestservice.ErrActiveRequestExists):
		handler.WriteAPIError(w, http.StatusConflict, "active_request_exists", "Заявка на этот ресурс уже находится в работе.")
	case errors.Is(err, requestservice.ErrRequestAlreadyDecided):
		handler.WriteAPIError(w, http.StatusConflict, "request_already_decided", "По заявке уже принято другое решение. Обнови карточку.")
	case errors.Is(err, requestservice.ErrInvalidRequestState):
		handler.WriteAPIError(w, http.StatusConflict, "invalid_request_state", "Действие недоступно в текущем состоянии заявки.")
	default:
		slog.ErrorContext(r.Context(), "обработать HTTP-запрос заявки на доступ", "path", r.URL.Path, "error", err)
		handler.WriteAPIError(w, http.StatusInternalServerError, "internal_error", "Не удалось выполнить запрос. Попробуй позже.")
	}
}
