package handler

import "net/http"

type CtxSessionKey string

type SessionHandler struct {
	origin http.Handler
}

func (h *SessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.origin.ServeHTTP(w, r)
}

func WithSessionHandler(origin http.Handler) http.Handler {
	return &SessionHandler{
		origin: origin,
	}
}
