package handler

import (
	"encoding/json"
	"net/http"

	pkglogin "example.com/webrtcserver/pkg/models/login"
)

type LogoutHandler struct {
	UserSessionManager pkglogin.UserSessionManager
}

func (h *LogoutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessId := ctx.Value(CtxSessionKeySessionId)
	if sessId == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No valid session id is found"})
		return
	}

	if err := h.UserSessionManager.LogOut(ctx, sessId.(string)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Unable to log user out, internal problem"})
		return
	}

	w.WriteHeader(http.StatusOK)
}
