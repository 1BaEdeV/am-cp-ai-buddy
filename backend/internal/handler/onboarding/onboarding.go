package onboarding

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/onboarding"
)

const demoEmployeeHeader = "X-Demo-Employee-Id"

type OnboardingHandler struct {
	service *onboarding.Service
}

func NewOnboardingHandler(service *onboarding.Service) *OnboardingHandler {
	return &OnboardingHandler{service: service}
}

func (h *OnboardingHandler) GetTeamIntro(w http.ResponseWriter, r *http.Request) {
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

	task, err := h.service.GetTeamIntro(r.Context(), employeeID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	handler.WriteJSON(w, http.StatusOK, task)
}

func (h *OnboardingHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, onboarding.ErrInternNotFound):
		handler.WriteAPIError(w, http.StatusNotFound, "intern_not_found", "Стажёр не найден.")
	case errors.Is(err, onboarding.ErrForbidden):
		handler.WriteAPIError(w, http.StatusForbidden, "forbidden", "Карточка доступна только стажёру.")
	case errors.Is(err, onboarding.ErrProfileNotFound):
		handler.WriteAPIError(w, http.StatusForbidden, "profile_required", "Сначала заполни профиль.")
	default:
		slog.ErrorContext(r.Context(), "получить карточку знакомства", "error", err)
		handler.WriteAPIError(w, http.StatusInternalServerError, "internal_error", "Не удалось загрузить карточку.")
	}
}