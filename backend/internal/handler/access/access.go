package access

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	accessservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
)

// AccessService подбирает ресурсы стажёра и рассчитывает прогресс в сервисном слое.
type AccessService interface {
	ListRequiredAccesses(context.Context, string) (accessservice.WorkingAccesses, error)
}

// AccessHandler обрабатывает HTTP-запрос карточки «Рабочие доступы».
type AccessHandler struct {
	service AccessService
}

// NewAccessHandler принимает сервис рабочих доступов.
func NewAccessHandler(service AccessService) *AccessHandler {
	return &AccessHandler{service: service}
}

// WorkingAccessesTask содержит карточку задания в формате TaskDetails.
type WorkingAccessesTask struct {
	ID              string                        `json:"id"`
	Title           string                        `json:"title"`
	Subtitle        string                        `json:"subtitle"`
	Status          string                        `json:"status"`
	Description     string                        `json:"description"`
	ChatEnabled     bool                          `json:"chatEnabled"`
	WorkingAccesses accessservice.WorkingAccesses `json:"workingAccesses"`
}

// GetWorkingAccesses получает ID из демо-заголовка и возвращает карточку с ресурсами.
func (h *AccessHandler) GetWorkingAccesses(w http.ResponseWriter, r *http.Request) {
	employeeID := strings.TrimSpace(r.Header.Get("X-Demo-Employee-Id"))
	if employeeID == "" {
		handler.WriteAPIError(w, http.StatusUnauthorized, "demo_identity_required", "Введи демо-почту или внутренний ID.")
		return
	}
	accesses, err := h.service.ListRequiredAccesses(r.Context(), employeeID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	status := "not_started"
	if accesses.Progress.Total > 0 && accesses.Progress.Completed == accesses.Progress.Total {
		status = "completed"
	} else if accesses.Progress.Completed > 0 {
		status = "in_progress"
	}
	handler.WriteJSON(w, http.StatusOK, WorkingAccessesTask{
		ID:              "working-accesses",
		Title:           "Рабочие доступы",
		Subtitle:        "Ресурсы команды",
		Status:          status,
		Description:     "Проверь доступы к ресурсам команды.",
		ChatEnabled:     true,
		WorkingAccesses: accesses,
	})
}

func (h *AccessHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrEmployeeNotFound):
		handler.WriteAPIError(w, http.StatusUnauthorized, "demo_identity_required", "Введи демо-почту или внутренний ID.")
	case errors.Is(err, auth.ErrForbidden):
		handler.WriteAPIError(w, http.StatusForbidden, "forbidden", "Действие недоступно для этой роли.")
	case errors.Is(err, auth.ErrProfileRequired):
		handler.WriteAPIError(w, http.StatusForbidden, "profile_required", "Заполни профиль сотрудника.")
	default:
		slog.ErrorContext(r.Context(), "загрузить карточку рабочих доступов", "path", r.URL.Path, "error", err)
		handler.WriteAPIError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить рабочие доступы. Попробуй позже.")
	}
}
