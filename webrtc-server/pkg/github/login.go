package github

import (
	"context"
	"sync"
)

type GithubTokenResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope,omitempty"`
	TokenType   string `json:"token_type,omitempty"`
}

type GithubLoginManager interface {
	Login(ctx context.Context, sessionId string, ghToken GithubTokenResponse) error
}

type MemoryGithubLoginManager struct {
	store sync.Map
}
