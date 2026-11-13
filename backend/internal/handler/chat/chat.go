package chathandler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	chatservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/chat"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
)

const demoEmployeeHeader = "X-Demo-Employee-Id"

type ChatHandler struct {
	service *chatservice.Service
}

func NewChatHandler(service *chatservice.Service) *ChatHandler {
	return &ChatHandler{service: service}
}

func (h *ChatHandler) GetTaskChat(w http.ResponseWriter, r *http.Request) {
	employeeID := strings.TrimSpace(r.Header.Get(demoEmployeeHeader))
	if employeeID == "" {
		handler.WriteAPIError(
			w,
			http.StatusUnauthorized,
			"demo_identity_required",
			"Введи демо-почту или внутренний ID.",
		)
		return
	}

	taskID := r.PathValue("taskId")

	result, err := h.service.GetTaskChat(r.Context(), employeeID, taskID)
	if err != nil {
		switch {
		case errors.Is(err, chatservice.ErrEmployeeNotFound),
			errors.Is(err, chatservice.ErrInvalidTask):
			handler.WriteAPIError(w, http.StatusNotFound, "not_found", "Чат не найден.")
		case errors.Is(err, chatservice.ErrForbidden):
			handler.WriteAPIError(w, http.StatusForbidden, "forbidden", "Чат доступен только стажёру.")
		case errors.Is(err, chatservice.ErrProfileRequired):
			handler.WriteAPIError(w, http.StatusForbidden, "profile_required", "Сначала заполни профиль.")
		default:
			slog.ErrorContext(r.Context(), "получить историю чата", "error", err)
			handler.WriteAPIError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить чат.")
		}
		return
	}

	handler.WriteJSON(w, http.StatusOK, result)
}
