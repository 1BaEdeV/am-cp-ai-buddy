package mltools

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
	tools "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/mltools"
)

type Service interface {
	GetRoleTools(context.Context) (access.WorkingAccesses, error)
	CheckToolAccess(context.Context, string) (tools.ToolAccess, error)
	PrepareAccessRequest(context.Context, string) (accessrequest.Preparation, error)
	GetAccessRequestStatus(context.Context, string) (accessrequest.Request, error)
}

type Handler struct {
	service Service
	token   string
}

func NewHandler(service Service, token string) *Handler {
	return &Handler{service: service, token: token}
}

func (h *Handler) authorized(w http.ResponseWriter, r *http.Request) bool {
	fields := strings.Fields(r.Header.Get("Authorization"))
	if h.token == "" || len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || subtle.ConstantTimeCompare([]byte(fields[1]), []byte(h.token)) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		handler.WriteAPIError(w, http.StatusUnauthorized, "ml_service_unauthorized", "Нужен сервисный токен ML.")
		return false
	}
	return true
}

func (h *Handler) ListTools(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}
	handler.WriteJSON(w, http.StatusOK, tools.Definitions())
}

func (h *Handler) CallTool(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}
	id := strings.TrimSpace(r.Header.Get("X-Demo-Employee-Id"))
	if id == "" {
		handler.WriteAPIError(w, http.StatusUnauthorized, "demo_identity_required", "Передай контекст текущего сотрудника.")
		return
	}
	ctx := tools.WithEmployeeID(r.Context(), id)
	var result any
	var err error
	switch r.PathValue("toolName") {
	case "get_role_tools":
		var input struct{}
		if !handler.DecodeJSON(w, r, &input) {
			return
		}
		result, err = h.service.GetRoleTools(ctx)
	case "check_tool_access", "prepare_access_request":
		var input struct {
			ResourceID string `json:"resourceId"`
		}
		if !handler.DecodeJSON(w, r, &input) {
			return
		}
		if strings.TrimSpace(input.ResourceID) == "" {
			h.writeError(w, r, accessrequest.ErrInvalidInput)
			return
		}
		if r.PathValue("toolName") == "check_tool_access" {
			result, err = h.service.CheckToolAccess(ctx, input.ResourceID)
		} else {
			result, err = h.service.PrepareAccessRequest(ctx, input.ResourceID)
		}
	case "get_access_request_status":
		var input struct {
			RequestID string `json:"requestId"`
		}
		if !handler.DecodeJSON(w, r, &input) {
			return
		}
		if strings.TrimSpace(input.RequestID) == "" {
			h.writeError(w, r, accessrequest.ErrInvalidInput)
			return
		}
		result, err = h.service.GetAccessRequestStatus(ctx, input.RequestID)
	default:
		handler.WriteAPIError(w, http.StatusNotFound, "tool_not_found", "Инструмент недоступен.")
		return
	}
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	handler.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrEmployeeNotFound):
		handler.WriteAPIError(w, 401, "demo_identity_required", "Сотрудник не найден.")
	case errors.Is(err, auth.ErrForbidden):
		handler.WriteAPIError(w, 403, "forbidden", "Инструмент недоступен для этой роли.")
	case errors.Is(err, auth.ErrProfileRequired):
		handler.WriteAPIError(w, 403, "profile_required", "Заполни профиль сотрудника.")
	case errors.Is(err, accessrequest.ErrInvalidInput):
		handler.WriteAPIError(w, 400, "invalid_input", "Проверь аргументы инструмента.")
	case errors.Is(err, accessrequest.ErrResourceNotRequired):
		handler.WriteAPIError(w, 404, "resource_not_required", "Ресурс не предусмотрен для сотрудника.")
	case errors.Is(err, accessrequest.ErrRequestNotFound):
		handler.WriteAPIError(w, 404, "request_not_found", "Заявка не найдена.")
	case errors.Is(err, tools.ErrResourceChanged), errors.Is(err, demoprovider.ErrResourceNotFound), errors.Is(err, demoprovider.ErrUnsupportedLevel):
		handler.WriteAPIError(w, 409, "resource_changed", "Параметры ресурса изменились. Обнови список ресурсов.")
	case errors.Is(err, access.ErrRequirementsNotConfigured), errors.Is(err, access.ErrInvalidRequirement):
		handler.WriteAPIError(w, 409, "requirements_not_configured", "Обязательные доступы настроены некорректно.")
	case errors.Is(err, demoprovider.ErrCheckFailed), errors.Is(err, context.DeadlineExceeded):
		handler.WriteAPIError(w, 503, "access_check_unavailable", "Не удалось проверить доступ. Повтори позже.")
	default:
		slog.ErrorContext(r.Context(), "выполнить инструмент ML", "tool", r.PathValue("toolName"), "error", err)
		handler.WriteAPIError(w, 500, "internal_error", "Не удалось выполнить инструмент.")
	}
}
