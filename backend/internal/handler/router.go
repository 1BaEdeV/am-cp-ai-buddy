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

func NewRouter(authHandler AuthEndpoints, accessHandler AccessEndpoints) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("GET /api/v1/me", authHandler.Me)
	if accessHandler != nil {
		mux.HandleFunc("GET /api/v1/onboarding/tasks/working-accesses", accessHandler.GetWorkingAccesses)
	}
	return mux
}
