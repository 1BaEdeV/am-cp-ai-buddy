package handler

import "net/http"

type AuthEndpoints interface {
	Login(http.ResponseWriter, *http.Request)
	Register(http.ResponseWriter, *http.Request)
	Me(http.ResponseWriter, *http.Request)
}

// OnboardingEndpoints describes the HTTP handlers for the onboarding plan.
type OnboardingEndpoints interface {
	GetTeamIntro(http.ResponseWriter, *http.Request)
	GetPlan(http.ResponseWriter, *http.Request)
	CompleteTeamIntro(http.ResponseWriter, *http.Request)
	GetTaskDetails(http.ResponseWriter, *http.Request)
}

// ChatEndpoints describes the HTTP handlers for AI-buddy task chats.
type ChatEndpoints interface {
	GetTaskChat(http.ResponseWriter, *http.Request)
	SendTaskChatMessage(http.ResponseWriter, *http.Request)
}

type AccessEndpoints interface {
	GetWorkingAccesses(http.ResponseWriter, *http.Request)
}

type AccessRequestEndpoints interface {
	ConfirmDraft(http.ResponseWriter, *http.Request)
	ListRequests(http.ResponseWriter, *http.Request)
	GetRequest(http.ResponseWriter, *http.Request)
	DecideRequest(http.ResponseWriter, *http.Request)
	RetryIssuance(http.ResponseWriter, *http.Request)
}

// MLToolEndpoints are registered only when a service token is configured.
type MLToolEndpoints interface {
	ListTools(http.ResponseWriter, *http.Request)
	CallTool(http.ResponseWriter, *http.Request)
}

// NewRouter registers auth, onboarding, chat, access, and optional ML tool routes.
func NewRouter(
	authHandler AuthEndpoints,
	onboardingHandler OnboardingEndpoints,
	chatHandler ChatEndpoints,
	accessHandler AccessEndpoints,
	requestHandler AccessRequestEndpoints,
	toolHandlers ...MLToolEndpoints,
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
	if len(toolHandlers) > 0 && toolHandlers[0] != nil {
		mux.HandleFunc("GET /api/v1/internal/ml/tools", toolHandlers[0].ListTools)
		mux.HandleFunc("POST /api/v1/internal/ml/tools/{toolName}", toolHandlers[0].CallTool)
	}
	return mux
}
