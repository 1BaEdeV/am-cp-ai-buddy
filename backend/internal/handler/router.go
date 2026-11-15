package handler

import "net/http"

type AuthEndpoints interface {
	Login(http.ResponseWriter, *http.Request)
	Register(http.ResponseWriter, *http.Request)
	Me(http.ResponseWriter, *http.Request)
}

// OnboardingEndpoints описывает HTTP-обработчики плана адаптации.
type OnboardingEndpoints interface {
	GetTeamIntro(http.ResponseWriter, *http.Request)
	GetPlan(http.ResponseWriter, *http.Request)
	CompleteTeamIntro(http.ResponseWriter, *http.Request)
	GetTaskDetails(http.ResponseWriter, *http.Request)
}

// ChatEndpoints описывает HTTP-обработчики чатов AI-buddy.
type ChatEndpoints interface {
	GetTaskChat(http.ResponseWriter, *http.Request)
	SendTaskChatMessage(http.ResponseWriter, *http.Request)
}

// AccessEndpoints описывает HTTP-обработчик карточки рабочих доступов.
type AccessEndpoints interface {
	GetWorkingAccesses(http.ResponseWriter, *http.Request)
}

// AccessRequestEndpoints описывает действия с заявками на доступ.
type AccessRequestEndpoints interface {
	ConfirmDraft(http.ResponseWriter, *http.Request)
	ListRequests(http.ResponseWriter, *http.Request)
	GetRequest(http.ResponseWriter, *http.Request)
	DecideRequest(http.ResponseWriter, *http.Request)
	RetryIssuance(http.ResponseWriter, *http.Request)
}

// NewRouter регистрирует маршруты авторизации, адаптации и получения доступов.
func NewRouter(
	authHandler AuthEndpoints,
	onboardingHandler OnboardingEndpoints,
	chatHandler ChatEndpoints,
	accessHandler AccessEndpoints,
	requestHandler AccessRequestEndpoints,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("GET /api/v1/me", authHandler.Me)

	if onboardingHandler != nil {
		mux.HandleFunc("GET /api/v1/onboarding/tasks/team-intro", onboardingHandler.GetTeamIntro)
		mux.HandleFunc("GET /api/v1/onboarding", onboardingHandler.GetPlan)
		mux.HandleFunc("POST /api/v1/onboarding/tasks/team-intro/complete", onboardingHandler.CompleteTeamIntro)
		mux.HandleFunc("GET /api/v1/onboarding/tasks/{taskId}", onboardingHandler.GetTaskDetails)
	}

	if chatHandler != nil {
		mux.HandleFunc("GET /api/v1/onboarding/tasks/{taskId}/chat", chatHandler.GetTaskChat)
		mux.HandleFunc("POST /api/v1/onboarding/tasks/{taskId}/chat/messages", chatHandler.SendTaskChatMessage)
	}

	if accessHandler != nil {
		mux.HandleFunc("GET /api/v1/onboarding/tasks/working-accesses", accessHandler.GetWorkingAccesses)
	}

	if requestHandler != nil {
		mux.HandleFunc("POST /api/v1/access-requests/drafts/{draftId}/confirm", requestHandler.ConfirmDraft)
		mux.HandleFunc("GET /api/v1/access-requests", requestHandler.ListRequests)
		mux.HandleFunc("GET /api/v1/access-requests/{requestId}", requestHandler.GetRequest)
		mux.HandleFunc("POST /api/v1/access-requests/{requestId}/decision", requestHandler.DecideRequest)
		mux.HandleFunc("POST /api/v1/access-requests/{requestId}/retry", requestHandler.RetryIssuance)
	}

	return mux
}
