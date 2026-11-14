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
func (h *ChatHandler) SendTaskChatMessage(w http.ResponseWriter, r *http.Request) {
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

	idempotencyKey := r.Header.Get("Idempotency-Key")

	var input chatservice.MessageInput
	if !handler.DecodeJSON(w, r, &input) {
		return
	}

	run, err := h.service.SendMessage(
		r.Context(),
		employeeID,
		r.PathValue("taskId"),
		idempotencyKey,
		input,
	)
	if err != nil {
		h.writeSendError(w, r, err)
		return
	}

	status := http.StatusAccepted
	if run.Status == "completed" || run.Status == "failed" {
		status = http.StatusOK
	}

	handler.WriteJSON(w, status, run)
}  


func (h *ChatHandler) writeSendError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, chatservice.ErrInvalidMessage):
		handler.WriteAPIError(w, http.StatusBadRequest, "invalid_input", "Проверь текст вопроса и Idempotency-Key.")
	case errors.Is(err, chatservice.ErrInvalidTask),
		errors.Is(err, chatservice.ErrEmployeeNotFound):
		handler.WriteAPIError(w, http.StatusNotFound, "not_found", "Чат не найден.")
	case errors.Is(err, chatservice.ErrForbidden):
		handler.WriteAPIError(w, http.StatusForbidden, "forbidden", "Чат доступен только стажёру.")
	case errors.Is(err, chatservice.ErrProfileRequired):
		handler.WriteAPIError(w, http.StatusForbidden, "profile_required", "Сначала заполни профиль.")
	case errors.Is(err, chatservice.ErrChatBusy):
		handler.WriteAPIError(w, http.StatusConflict, "chat_busy", "Дождись ответа в этом чате.")
	case errors.Is(err, chatservice.ErrIdempotencyKeyReused):
		handler.WriteAPIError(w, http.StatusConflict, "idempotency_key_reused", "Для нового сообщения нужен новый ключ.")
	default:
		slog.ErrorContext(r.Context(), "отправить вопрос в чат", "error", err)
		handler.WriteAPIError(w, http.StatusInternalServerError, "internal_error", "Не удалось отправить вопрос.")
	}
}