package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	authservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
)

const demoEmployeeHeader = "X-Demo-Employee-Id"

// AuthService оставляет бизнес-правила и работу с БД за пределами HTTP-слоя.
type AuthService interface {
	Login(context.Context, string) (authservice.LoginResult, error)
	Register(context.Context, string, authservice.RegistrationInput) (authservice.Profile, bool, error)
	Me(context.Context, string) (authservice.Profile, error)
}

type AuthHandler struct {
	service AuthService
}

func NewAuthHandler(service AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Identifier string `json:"identifier"`
	}
	if !handler.DecodeJSON(w, r, &input) {
		return
	}
	result, err := h.service.Login(r.Context(), input.Identifier)
	if err != nil {
		h.writeError(w, r, err, true)
		return
	}
	handler.WriteJSON(w, http.StatusOK, result)
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	employeeID := strings.TrimSpace(r.Header.Get(demoEmployeeHeader))
	if employeeID == "" {
		writeIdentityRequired(w)
		return
	}
	var input authservice.RegistrationInput
	if !handler.DecodeJSON(w, r, &input) {
		return
	}
	profile, created, err := h.service.Register(r.Context(), employeeID, input)
	if err != nil {
		h.writeError(w, r, err, false)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	handler.WriteJSON(w, status, profile)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	employeeID := strings.TrimSpace(r.Header.Get(demoEmployeeHeader))
	if employeeID == "" {
		writeIdentityRequired(w)
		return
	}
	profile, err := h.service.Me(r.Context(), employeeID)
	if err != nil {
		h.writeError(w, r, err, false)
		return
	}
	handler.WriteJSON(w, http.StatusOK, profile)
}

func (h *AuthHandler) writeError(w http.ResponseWriter, r *http.Request, err error, login bool) {
	switch {
	case errors.Is(err, authservice.ErrInvalidInput):
		handler.WriteAPIError(w, http.StatusBadRequest, "invalid_input", err.Error())
	case errors.Is(err, authservice.ErrEmployeeNotFound):
		if login {
			handler.WriteAPIError(w, http.StatusNotFound, "demo_employee_not_found", "Проверь демо-почту или внутренний ID.")
		} else {
			writeIdentityRequired(w)
		}
	case errors.Is(err, authservice.ErrForbidden):
		handler.WriteAPIError(w, http.StatusForbidden, "forbidden", "Действие недоступно для этой роли.")
	case errors.Is(err, authservice.ErrProfileRequired):
		handler.WriteAPIError(w, http.StatusForbidden, "profile_required", "Заполни профиль сотрудника.")
	case errors.Is(err, authservice.ErrRegistrationUnavailable):
		handler.WriteAPIError(w, http.StatusConflict, "registration_unavailable", "Обратись к организатору демо.")
	case errors.Is(err, authservice.ErrProfileAlreadyExists):
		handler.WriteAPIError(w, http.StatusConflict, "profile_already_exists", "Открой существующий профиль.")
	default:
		slog.ErrorContext(r.Context(), "ошибка обработки запроса", "path", r.URL.Path, "error", err)
		handler.WriteAPIError(w, http.StatusInternalServerError, "internal_error", "Не удалось выполнить запрос. Попробуй позже.")
	}
}

func writeIdentityRequired(w http.ResponseWriter) {
	handler.WriteAPIError(w, http.StatusUnauthorized, "demo_identity_required", "Введи демо-почту или внутренний ID.")
}
