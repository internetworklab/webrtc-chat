package handler

import (
	"context"
	"net/http"
)

type CtxSessionKey string

const (
	CtxSessionKeySessionId CtxSessionKey = "sessionId"
)

type SessionManager interface {
	// If no associated is found with such request, returns an empty string
	// otherwise returns a session identifier (which is opaque to the user)
	GetSessionId(ctx context.Context, r *http.Request) string

	// Associate the request with a session, maybe alter the response when needed.
	CreateSession(ctx context.Context, w http.ResponseWriter, r *http.Request) string
}

type SessionHandler struct {
	origin http.Handler
	sess   SessionManager
}

func (h *SessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// this is a http middleware, it does the following:
	// 1. check if there is some session associated with request, if there is, set the session identifier to context (key is CtxSessionKey)
	// 2. otherwise, create a session with request, and set the session identifier of the newly created session to the ctx
	ctx := r.Context()

	sessionId := h.sess.GetSessionId(ctx, r)
	if sessionId == "" {
		sessionId = h.sess.CreateSession(ctx, w, r)
	}

	ctx = context.WithValue(ctx, CtxSessionKeySessionId, sessionId)
	r = r.WithContext(ctx)

	h.origin.ServeHTTP(w, r)
}

func WithSessionHandler(origin http.Handler, sessionMngr SessionManager) http.Handler {
	return &SessionHandler{
		origin: origin,
		sess:   sessionMngr,
	}
}
