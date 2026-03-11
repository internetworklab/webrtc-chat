package handler

import (
	"encoding/json"
	"net/http"

	pkguser "example.com/webrtcserver/pkg/models/user"
	pkgmyjwt "example.com/webrtcserver/pkg/my_jwt"
)

type BotsManagementHandler struct {
	UserManager pkguser.UserManager
	JWTManager  pkgmyjwt.JWTManager
}

func (h *BotsManagementHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(&ErrResponse{Err: pkguser.ErrUsernameDuplicated.Error()})
}
