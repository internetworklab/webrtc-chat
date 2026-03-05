package login

import "context"

type UserSessionManager interface {
	// Login is to associate a session with a registered user
	LogIn(ctx context.Context, userId string, sessionId string) error

	// Log out is to de-ssociate a session with the user bound
	LogOut(ctx context.Context, sessionId string) error

	// Returns an empty string if the user hasn't log in, otherwise returns an non-empty string
	// Callers should check error first
	GetUserIdBySessionId(ctx context.Context, sessionId string) (string, error)
}
