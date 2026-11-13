package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler"
	authhandler "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/handler/auth"
)

type authStub struct {
	login    func(context.Context, string) (authservice.LoginResult, error)
	register func(context.Context, string, authservice.RegistrationInput) (authservice.Profile, bool, error)
	me       func(context.Context, string) (authservice.Profile, error)
}

func (s authStub) Login(ctx context.Context, identifier string) (authservice.LoginResult, error) {
	return s.login(ctx, identifier)
}

func (s authStub) Register(ctx context.Context, id string, input authservice.RegistrationInput) (authservice.Profile, bool, error) {
	return s.register(ctx, id, input)
}

func (s authStub) Me(ctx context.Context, id string) (authservice.Profile, error) {
	return s.me(ctx, id)
}

func newRouter(service authhandler.AuthService) http.Handler {
	return handler.NewRouter(
		authhandler.NewAuthHandler(service),
		onboardingStub{},
		
	)
}
func (onboardingStub) CompleteTeamIntro(http.ResponseWriter, *http.Request) {}

type onboardingStub struct{}

func (onboardingStub) GetPlan(http.ResponseWriter, *http.Request)      {}
func (onboardingStub) GetTeamIntro(http.ResponseWriter, *http.Request) {}

func request(router http.Handler, method, path, body, employeeID string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Demo-Employee-Id", employeeID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func checkError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var result struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("не JSON-ответ: %v; тело: %s", err, w.Body.String())
	}
	if w.Code != status || result.Code != code {
		t.Fatalf("получено %d / %q, ожидалось %d / %q", w.Code, result.Code, status, code)
	}
}

func TestLogin(t *testing.T) {
	router := newRouter(authStub{login: func(_ context.Context, identifier string) (authservice.LoginResult, error) {
		if identifier != "EMP-1002" {
			t.Fatalf("неверный identifier: %q", identifier)
		}
		return authservice.LoginResult{Status: authservice.LoginNeedsRegistration, EmployeeID: identifier}, nil
	}})
	w := request(router, http.MethodPost, "/api/v1/auth/login", `{"identifier":"EMP-1002"}`, "")
	var result authservice.LoginResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || result.Status != authservice.LoginNeedsRegistration || result.EmployeeID != "EMP-1002" {
		t.Fatalf("неожиданный ответ: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("ответ авторизации должен запрещать кеширование")
	}
}

func TestRegisterCreatedAndRepeated(t *testing.T) {
	for _, created := range []bool{true, false} {
		t.Run(fmt.Sprint(created), func(t *testing.T) {
			router := newRouter(authStub{register: func(_ context.Context, id string, input authservice.RegistrationInput) (authservice.Profile, bool, error) {
				if id != "EMP-1002" || input.PositionID != "position-go-intern" || input.MentorID != "mentor-katya" || input.InternshipStartDate != "2026-11-09" {
					t.Fatalf("неверные аргументы: %q %+v", id, input)
				}
				return authservice.Profile{ID: "intern-oleg", EmployeeID: id}, created, nil
			}})
			body := `{"positionId":"position-go-intern","mentorId":"mentor-katya","internshipStartDate":"2026-11-09"}`
			w := request(router, http.MethodPost, "/api/v1/auth/register", body, " EMP-1002 ")
			status := http.StatusOK
			if created {
				status = http.StatusCreated
			}
			var profile authservice.Profile
			if err := json.Unmarshal(w.Body.Bytes(), &profile); err != nil {
				t.Fatal(err)
			}
			if w.Code != status || profile.ID != "intern-oleg" || profile.EmployeeID != "EMP-1002" {
				t.Fatalf("неожиданный ответ: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestMe(t *testing.T) {
	router := newRouter(authStub{me: func(_ context.Context, id string) (authservice.Profile, error) {
		if id != "EMP-1001" {
			t.Fatalf("неверный ID: %q", id)
		}
		return authservice.Profile{ID: "intern-alina", EmployeeID: id}, nil
	}})
	w := request(router, http.MethodGet, "/api/v1/me", "", "EMP-1001")
	var result authservice.Profile
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || result.ID != "intern-alina" {
		t.Fatalf("неожиданный ответ: %d %s", w.Code, w.Body.String())
	}
}

func TestMissingIdentity(t *testing.T) {
	// У stub нет методов: если хэндлер вызовет сервис без ID, тест упадёт.
	router := newRouter(authStub{})
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/auth/register"},
		{http.MethodGet, "/api/v1/me"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := request(router, tc.method, tc.path, `{}`, "  ")
			checkError(t, w, http.StatusUnauthorized, "demo_identity_required")
		})
	}
}

func TestInvalidJSON(t *testing.T) {
	router := newRouter(authStub{})
	for _, body := range []string{
		``, `null`, `[]`, `"text"`, `{"identifier":`, `{"identifier":42}`,
		`{"identifier":"EMP-1001","role":"manager"}`,
		`{"identifier":"EMP-1001"} {}`, `{"identifier":"EMP-1001"} trailing`,
	} {
		t.Run(body, func(t *testing.T) {
			w := request(router, http.MethodPost, "/api/v1/auth/login", body, "")
			checkError(t, w, http.StatusBadRequest, "invalid_input")
		})
	}
	oversized := `{"identifier":"` + strings.Repeat("a", 1<<20) + `"}`
	w := request(router, http.MethodPost, "/api/v1/auth/login", oversized, "")
	checkError(t, w, http.StatusRequestEntityTooLarge, "request_too_large")

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{}`))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	checkError(t, w, http.StatusUnsupportedMediaType, "unsupported_media_type")
}

func TestRegisterInvalidJSON(t *testing.T) {
	// Сервис не должен вызываться, если тело регистрации не прошло проверку.
	router := newRouter(authStub{})
	for _, body := range []string{`null`, `{"mentorId":42}`, `{"role":"manager"}`, `{} {}`} {
		t.Run(body, func(t *testing.T) {
			w := request(router, http.MethodPost, "/api/v1/auth/register", body, "EMP-1002")
			checkError(t, w, http.StatusBadRequest, "invalid_input")
		})
	}
}

func TestServiceErrors(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		err                error
		status             int
		code               string
	}{
		{"invalid", http.MethodPost, "/api/v1/auth/login", authservice.ErrInvalidInput, 400, "invalid_input"},
		{"missing login", http.MethodPost, "/api/v1/auth/login", authservice.ErrEmployeeNotFound, 404, "demo_employee_not_found"},
		{"references", http.MethodPost, "/api/v1/auth/login", authservice.ErrRegistrationUnavailable, 409, "registration_unavailable"},
		{"unknown identity", http.MethodPost, "/api/v1/auth/register", authservice.ErrEmployeeNotFound, 401, "demo_identity_required"},
		{"forbidden", http.MethodPost, "/api/v1/auth/register", authservice.ErrForbidden, 403, "forbidden"},
		{"conflict", http.MethodPost, "/api/v1/auth/register", authservice.ErrProfileAlreadyExists, 409, "profile_already_exists"},
		{"profile required", http.MethodGet, "/api/v1/me", authservice.ErrProfileRequired, 403, "profile_required"},
		{"database error", http.MethodGet, "/api/v1/me", errors.New("secret database details"), 500, "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("service: %w", tc.err)
			router := newRouter(authStub{
				login: func(context.Context, string) (authservice.LoginResult, error) {
					return authservice.LoginResult{}, wrapped
				},
				register: func(context.Context, string, authservice.RegistrationInput) (authservice.Profile, bool, error) {
					return authservice.Profile{}, false, wrapped
				},
				me: func(context.Context, string) (authservice.Profile, error) { return authservice.Profile{}, wrapped },
			})
			w := request(router, tc.method, tc.path, `{}`, "EMP-1002")
			checkError(t, w, tc.status, tc.code)
			if strings.Contains(w.Body.String(), "secret database details") {
				t.Fatal("внутренняя ошибка БД попала в HTTP-ответ")
			}
		})
	}
}

func TestMethodNotAllowed(t *testing.T) {
	w := request(newRouter(authStub{}), http.MethodGet, "/api/v1/auth/login", "", "")
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("неожиданный ответ на неверный метод: %d", w.Code)
	}
}
