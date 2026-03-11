package handler

import (
	"net/http"

	pkgjwt "example.com/webrtcserver/pkg/my_jwt"
)

type JWTMiddleware struct {
	origin     http.Handler
	jwtManager pkgjwt.JWTManager
}

func (m *JWTMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {}

func WithJWTHandler(origin http.Handler, jwtManager pkgjwt.JWTManager) http.Handler {
	return &JWTMiddleware{
		origin:     origin,
		jwtManager: jwtManager,
	}
}
