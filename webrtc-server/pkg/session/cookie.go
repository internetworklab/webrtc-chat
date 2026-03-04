package session

import (
	"context"
	"net/http"
	"sync"

	"github.com/google/uuid"
)

// This is a cookie-based session manager
// an implementation of handler.SessionManager

type CookieSessionManager struct {
	sessionStore sync.Map
}

func (sessMngr *CookieSessionManager) getRandomSessionId() string {
	return uuid.NewString()
}

// If no associated is found with such request, returns an empty string
// otherwise returns a session identifier (which is opaque to the user)
func (sessMngr *CookieSessionManager) GetSessionId(ctx context.Context, r *http.Request) string {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		return ""
	}

	sessionId := cookie.Value
	// Check if the session exists in the store
	_, exists := sessMngr.sessionStore.Load(sessionId)
	if !exists {
		return ""
	}

	return sessionId
}

// Associate the request with a session, maybe alter the response when needed.
func (sessMngr *CookieSessionManager) CreateSession(ctx context.Context, w http.ResponseWriter, r *http.Request) string {
	sessionId := sessMngr.getRandomSessionId()

	// Store the session
	sessMngr.sessionStore.Store(sessionId, struct{}{})

	// Set the cookie on the response
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionId,
		Path:     "/",
		HttpOnly: true,
	})

	return sessionId
}
