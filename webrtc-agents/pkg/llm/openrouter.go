package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	openRouterBaseURL = "https://openrouter.ai/api/v1"
	openRouterTimeout = 60 * time.Second
)

type AuthorizationHeaderBuilder func(apikey string) string

func defaultAuthHeaderGetter(apiKey string) string {
	return fmt.Sprintf("Bearer %s", apiKey)
}

type OpenRouterCompletionProxy struct {
	APIKey                 string
	HttpClient             *http.Client
	BaseURL                string
	GetAuthorizationHeader AuthorizationHeaderBuilder
}

func (p *OpenRouterCompletionProxy) getAPIKey() string {
	return p.APIKey
}

func (p *OpenRouterCompletionProxy) getHttpClient() *http.Client {
	if p.HttpClient == nil {
		return &http.Client{
			Timeout: openRouterTimeout,
		}
	}
	return p.HttpClient
}

func (p *OpenRouterCompletionProxy) getBaseURL() string {
	if p.BaseURL == "" {
		return openRouterBaseURL
	}
	return p.BaseURL
}

func (p *OpenRouterCompletionProxy) Generate(ctx context.Context, request OpenRouterCompletionRequest) OpenRouterResponse {
	url := p.getBaseURL() + "/chat/completions"

	bodyBytes, err := json.Marshal(request)
	if err != nil {
		return OpenRouterResponse{
			ID:       "",
			Provider: "openrouter",
			Model:    request.Model,
			Object:   "error",
			Created:  time.Now().Unix(),
			Choices: []OpenRouterChoice{
				{
					Index:        0,
					FinishReason: "error",
					Message: &OpenRouterMessage{
						Role:    LLMRoleAssistant,
						Content: fmt.Sprintf("Failed to marshal request: %v", err),
					},
				},
			},
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return OpenRouterResponse{
			ID:       "",
			Provider: "openrouter",
			Model:    request.Model,
			Object:   "error",
			Created:  time.Now().Unix(),
			Choices: []OpenRouterChoice{
				{
					Index:        0,
					FinishReason: "error",
					Message: &OpenRouterMessage{
						Role:    LLMRoleAssistant,
						Content: fmt.Sprintf("Failed to create request: %v", err),
					},
				},
			},
		}
	}

	req.Header.Set("Content-Type", "application/json")
	if p.GetAuthorizationHeader == nil {
		p.GetAuthorizationHeader = defaultAuthHeaderGetter
	}
	authHeader := p.GetAuthorizationHeader(p.getAPIKey())
	req.Header.Set("Authorization", authHeader)

	resp, err := p.getHttpClient().Do(req)
	if err != nil {
		return OpenRouterResponse{
			ID:       "",
			Provider: "openrouter",
			Model:    request.Model,
			Object:   "error",
			Created:  time.Now().Unix(),
			Choices: []OpenRouterChoice{
				{
					Index:        0,
					FinishReason: "error",
					Message: &OpenRouterMessage{
						Role:    LLMRoleAssistant,
						Content: fmt.Sprintf("Failed to send request: %v", err),
					},
				},
			},
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return OpenRouterResponse{
			ID:       "",
			Provider: "openrouter",
			Model:    request.Model,
			Object:   "error",
			Created:  time.Now().Unix(),
			Choices: []OpenRouterChoice{
				{
					Index:        0,
					FinishReason: "error",
					Message: &OpenRouterMessage{
						Role:    LLMRoleAssistant,
						Content: fmt.Sprintf("Failed to read response: %v", err),
					},
				},
			},
		}
	}

	if resp.StatusCode != http.StatusOK {
		return OpenRouterResponse{
			ID:       "",
			Provider: "openrouter",
			Model:    request.Model,
			Object:   "error",
			Created:  time.Now().Unix(),
			Choices: []OpenRouterChoice{
				{
					Index:        0,
					FinishReason: "error",
					Message: &OpenRouterMessage{
						Role:    LLMRoleAssistant,
						Content: fmt.Sprintf("API error (status %d): %s", resp.StatusCode, string(respBody)),
					},
				},
			},
		}
	}

	var openRouterResp OpenRouterResponse
	if err := json.Unmarshal(respBody, &openRouterResp); err != nil {
		return OpenRouterResponse{
			ID:       "",
			Provider: "openrouter",
			Model:    request.Model,
			Object:   "error",
			Created:  time.Now().Unix(),
			Choices: []OpenRouterChoice{
				{
					Index:        0,
					FinishReason: "error",
					Message: &OpenRouterMessage{
						Role:    LLMRoleAssistant,
						Content: fmt.Sprintf("Failed to parse response: %v", err),
					},
				},
			},
		}
	}

	return openRouterResp
}
