package handler

import "net/http"

// AuthEndpoints описывает HTTP-хэндлеры, необходимые роутеру.
type AuthEndpoints interface {
	Login(http.ResponseWriter, *http.Request)
	Register(http.ResponseWriter, *http.Request)
	Me(http.ResponseWriter, *http.Request)
}

// AccessEndpoints описывает HTTP-хэндлер карточки рабочих доступов.
type AccessEndpoints interface {
	GetWorkingAccesses(http.ResponseWriter, *http.Request)
}

// AccessRequestEndpoints описывает действия пользователя с подготовленными заявками.
type AccessRequestEndpoints interface {
	ConfirmDraft(http.ResponseWriter, *http.Request)
	ListRequests(http.ResponseWriter, *http.Request)
	GetRequest(http.ResponseWriter, *http.Request)
	DecideRequest(http.ResponseWriter, *http.Request)
	RetryIssuance(http.ResponseWriter, *http.Request)
}

func NewRouter(authHandler AuthEndpoints, accessHandler AccessEndpoints, requestHandler AccessRequestEndpoints) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("GET /api/v1/me", authHandler.Me)
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
