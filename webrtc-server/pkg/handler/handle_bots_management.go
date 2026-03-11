package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"

	pkguser "example.com/webrtcserver/pkg/models/user"
	pkgmyjwt "example.com/webrtcserver/pkg/my_jwt"
)

type BotsManagementHandler struct {
	UserManager pkguser.UserManager
	JWTManager  pkgmyjwt.JWTManager
}

type BotCreationSuccess struct {
	Token string `json:"token"`
}

var ErrUsernameRequired = errors.New("username is required")

func (h *BotsManagementHandler) parseUserCreationPayload(r *http.Request) (*pkguser.UserCreationPayload, error) {
	contentType := r.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)

	switch mediaType {
	case "application/json":
		var payload pkguser.UserCreationPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return nil, fmt.Errorf("invalid JSON body: %w", err)
		}
		return &payload, nil
	case "multipart/form-data":
		return &pkguser.UserCreationPayload{
			Username:    r.FormValue(FormFieldUsername),
			DisplayName: r.FormValue(FormFieldDisplayName),
			AvatarURL:   r.FormValue(FormFieldAvatarURL),
		}, nil
	default:
		// Default to JSON for empty content type
		if contentType == "" {
			var payload pkguser.UserCreationPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				return nil, fmt.Errorf("invalid JSON body: %w", err)
			}
			return &payload, nil
		}
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}
}

func (h *BotsManagementHandler) handleAddBot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	payload, err := h.parseUserCreationPayload(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: err.Error()})
		return
	}

	if payload.Username == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&ErrResponse{Err: ErrUsernameRequired.Error()})
		return
	}

	user, err := h.UserManager.CreateUser(r.Context(), pkguser.UserCreationPayload{
		Username:    payload.Username,
		DisplayName: payload.DisplayName,
		AvatarURL:   payload.AvatarURL,
	}, true)
	if err != nil {
		if err == pkguser.ErrUsernameDuplicated {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(&ErrResponse{Err: err.Error()})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "failed to create user"})
		return
	}

	token, err := h.JWTManager.Issue(r.Context(), user.Id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "failed to issue token"})
		return
	}

	json.NewEncoder(w).Encode(&BotCreationSuccess{Token: token})
}

func (h *BotsManagementHandler) handleDeleteBot(w http.ResponseWriter, r *http.Request) {
	// currently all users are stored in memory, to delete all users: simply re-start the app
	json.NewEncoder(w).Encode(&ErrResponse{Err: "not implemented yet"})
}

func (h *BotsManagementHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/add") && r.Method == http.MethodPost {
		h.handleAddBot(w, r)
		return
	} else if strings.HasSuffix(r.URL.Path, "/delete") && r.Method == http.MethodDelete {
		h.handleDeleteBot(w, r)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(&ErrResponse{Err: "no handlers matched the request."})
}
