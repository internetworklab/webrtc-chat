package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
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
	APIKeyFromEnv          string
	HttpClient             *http.Client
	BaseURL                string
	GetAuthorizationHeader AuthorizationHeaderBuilder
}

func (p *OpenRouterCompletionProxy) getAPIKey() string {
	if apiKey := p.APIKey; apiKey != "" {
		return apiKey
	}
	if apiKeyEnv := p.APIKeyFromEnv; apiKeyEnv != "" {
		if apiKey := os.Getenv(apiKeyEnv); apiKey != "" {
			return apiKey
		}
	}
	log.New(os.Stderr, "[OpenRouterCompletionProxy] ", log.LUTC).Printf("[WARN] API key is empty, this usually shouldn't happen")
	return ""
}

func (p *OpenRouterCompletionProxy) getHttpClient() *http.Client {
	if p.HttpClient == nil {
		return http.DefaultClient
	}
	return p.HttpClient
}

func (p *OpenRouterCompletionProxy) getBaseURL() string {
	if p.BaseURL == "" {
		return openRouterBaseURL
	}
	return p.BaseURL
}

func (p *OpenRouterCompletionProxy) getErrorResponse(model string, content string) OpenRouterResponse {
	return OpenRouterResponse{
		ID:       "",
		Provider: "openrouter",
		Model:    model,
		Object:   "error",
		Created:  time.Now().Unix(),
		Choices: []OpenRouterChoice{
			{
				Index:        0,
				FinishReason: "error",
				Message: &OpenRouterMessage{
					Role:    LLMRoleAssistant,
					Content: content,
				},
			},
		},
	}
}

func (p *OpenRouterCompletionProxy) getCompletionURL() (url.URL, error) {
	baseURL := p.getBaseURL()
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("failed to parse base URL: %w", err)
	}
	parsedURL.Path = parsedURL.Path + "/chat/completions"
	return *parsedURL, nil
}

func (p *OpenRouterCompletionProxy) Generate(ctx context.Context, request OpenRouterCompletionRequest) OpenRouterResponse {
	completionURL, err := p.getCompletionURL()
	if err != nil {
		return p.getErrorResponse(request.Model, fmt.Sprintf("Failed to build completion URL: %v", err))
	}

	bodyBytes, err := json.Marshal(request)
	if err != nil {
		return p.getErrorResponse(request.Model, fmt.Sprintf("Failed to marshal request: %v", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, completionURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return p.getErrorResponse(request.Model, fmt.Sprintf("Failed to create request: %v", err))
	}

	req.Header.Set("Content-Type", "application/json")
	var authHeaderGetter AuthorizationHeaderBuilder = defaultAuthHeaderGetter
	if p.GetAuthorizationHeader != nil {
		authHeaderGetter = p.GetAuthorizationHeader
	}
	authHeader := authHeaderGetter(p.getAPIKey())
	req.Header.Set("Authorization", authHeader)

	resp, err := p.getHttpClient().Do(req)
	if err != nil {
		return p.getErrorResponse(request.Model, fmt.Sprintf("Failed to send request: %v", err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.getErrorResponse(request.Model, fmt.Sprintf("Failed to read response: %v", err))
	}

	if resp.StatusCode != http.StatusOK {
		return p.getErrorResponse(request.Model, fmt.Sprintf("API error (status %d): %s", resp.StatusCode, string(respBody)))
	}

	var openRouterResp OpenRouterResponse
	if err := json.Unmarshal(respBody, &openRouterResp); err != nil {
		return p.getErrorResponse(request.Model, fmt.Sprintf("Failed to parse response: %v", err))
	}

	return openRouterResp
}
