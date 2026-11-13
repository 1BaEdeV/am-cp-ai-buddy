package handler

import "net/http"

type AuthEndpoints interface {
	Login(http.ResponseWriter, *http.Request)
	Register(http.ResponseWriter, *http.Request)
	Me(http.ResponseWriter, *http.Request)
}

type OnboardingEndpoints interface {
	GetTeamIntro(http.ResponseWriter, *http.Request)
	GetPlan(http.ResponseWriter, *http.Request)
	CompleteTeamIntro(http.ResponseWriter, *http.Request)
}
type ChatEndpoints interface {
	GetTaskChat(http.ResponseWriter, *http.Request)
}

func NewRouter(
	authHandler AuthEndpoints,
	onboardingHandler OnboardingEndpoints,
	chatHandler ChatEndpoints,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("GET /api/v1/me", authHandler.Me)
	mux.HandleFunc(
		"GET /api/v1/onboarding/tasks/team-intro",
		onboardingHandler.GetTeamIntro,
	)
	mux.HandleFunc("GET /api/v1/onboarding", onboardingHandler.GetPlan)

	mux.HandleFunc(
		"POST /api/v1/onboarding/tasks/team-intro/complete",
		onboardingHandler.CompleteTeamIntro,
	)
	mux.HandleFunc(
		"GET /api/v1/onboarding/tasks/{taskId}/chat",
		chatHandler.GetTaskChat,
	)

	return mux
}
