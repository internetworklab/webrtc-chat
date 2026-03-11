package handler

import (
	"context"
	"net/http"
	"strings"

	pkgjwt "example.com/webrtcserver/pkg/my_jwt"
)

type JWTMiddleware struct {
	origin     http.Handler
	jwtManager pkgjwt.JWTManager
}

func (m *JWTMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		m.origin.ServeHTTP(w, r)
		return
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		m.origin.ServeHTTP(w, r)
		return
	}

	tokenString := parts[1]
	if tokenString == "" {
		m.origin.ServeHTTP(w, r)
		return
	}

	valid, claims, err := m.jwtManager.Validate(r.Context(), tokenString)
	if err != nil || !valid {
		m.origin.ServeHTTP(w, r)
		return
	}

	userID, err := claims.GetSubject()
	if err != nil || userID == "" {
		m.origin.ServeHTTP(w, r)
		return
	}

	ctx := context.WithValue(r.Context(), CtxSessionKeyUserIdFromJWT, userID)
	m.origin.ServeHTTP(w, r.WithContext(ctx))
}

func WithJWTHandler(origin http.Handler, jwtManager pkgjwt.JWTManager) http.Handler {
	return &JWTMiddleware{
		origin:     origin,
		jwtManager: jwtManager,
	}
}
