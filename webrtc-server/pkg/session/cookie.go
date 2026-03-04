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
func (sessMngr *CookieSessionManager) GetSessionId(ctx context.Context, r *http.Request) string {}

// Associate the request with a session, maybe alter the response when needed.
func (sessMngr *CookieSessionManager) CreateSession(ctx context.Context, w http.ResponseWriter, r *http.Request) string {
}
