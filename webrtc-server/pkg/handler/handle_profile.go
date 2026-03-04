package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	pkggithub "example.com/webrtcserver/pkg/github"
)

type ProfileHandler struct {
	GithubTokenRetriever pkggithub.GithubTokenRetriever
}

type ProfileResponse struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName,omitempty"`
	AvatarURL   string `json:"avatarURL,omitempty"`
}

func (h *ProfileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessId := ctx.Value(CtxSessionKeySessionId)
	if sessId == nil {
		json.NewEncoder(w).Encode(&ErrResponse{Err: "No session Id is found"})
		return
	}

	tokenObj, err := h.GithubTokenRetriever.GetToken(ctx, sessId.(string))
	if err != nil || tokenObj == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to retrieve Github token for: " + sessId.(string)})
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to create HTTP request to obtain profile of current Github user"})
		return
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenObj.AccessToken))
	req.Header.Set("Accept", "application/json")

	cli := http.DefaultClient
	resp, err := cli.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to obtain profile of current Github user"})
		return
	}
	defer resp.Body.Close()

	profileObject := new(pkggithub.GithubProfileResponse)
	if err := json.NewDecoder(resp.Body).Decode(profileObject); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(&ErrResponse{Err: "Failed to obtain profile of current Github user"})
		return
	}

	json.NewEncoder(w).Encode(&ProfileResponse{
		Username:    profileObject.Login,
		DisplayName: profileObject.Name,
		AvatarURL:   profileObject.AvatarURL,
	})
}
