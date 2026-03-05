package github

import (
	"context"
	"log"
	"sync"
)

type GithubTokenResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope,omitempty"`
	TokenType   string `json:"token_type,omitempty"`
}

// More on https://docs.github.com/en/rest/users/users?apiVersion=2022-11-28
type GithubProfileResponse struct {
	Login     string `json:"login"`
	Id        *int   `json:"id,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	HTMLURL   string `json:"html_url,omitempty"`
	Type      string `json:"type,omitempty"`
	Name      string `json:"name,omitempty"`
}

// The purposes of an GithubLoginManager is to associate or de-associate a session with a Github token
type GithubLoginManager interface {
	Login(ctx context.Context, sessionId string, ghToken GithubTokenResponse) error
	GetToken(ctx context.Context, sessionId string) (*GithubTokenResponse, error)
	DeleteToken(ctx context.Context, sessionId string) error
}

type MemoryGithubLoginManager struct {
	store sync.Map
	Debug bool
}

// Login stores the GitHub token for the given session ID
func (m *MemoryGithubLoginManager) Login(ctx context.Context, sessionId string, ghToken GithubTokenResponse) error {
	if m.Debug {
		log.Println("Github Login, token:", ghToken.AccessToken)
	}
	m.store.Store(sessionId, ghToken)
	return nil
}

// GetToken retrieves the GitHub token for the given session ID
// Returns the token and true if found, or an empty token and false if not found
func (m *MemoryGithubLoginManager) GetToken(ctx context.Context, sessionId string) (*GithubTokenResponse, error) {
	if v, ok := m.store.Load(sessionId); ok {
		token := v.(GithubTokenResponse)
		return &token, nil
	}
	return nil, nil
}

// Better do this only after the token has been revoked
func (m *MemoryGithubLoginManager) DeleteToken(ctx context.Context, sessionId string) error {
	m.store.Delete(sessionId)
	return nil
}
