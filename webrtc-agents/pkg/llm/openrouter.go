package llm

type OpenRouterResponse struct {
	ID                string             `json:"id"`
	Provider          string             `json:"provider"`
	Model             string             `json:"model"`
	Object            string             `json:"object"`
	Created           int64              `json:"created"`
	Choices           []OpenRouterChoice `json:"choices"`
	SystemFingerprint *string            `json:"system_fingerprint,omitempty"`
	Usage             *OpenRouterUsage   `json:"usage,omitempty"`
}

type OpenRouterChoice struct {
	Logprobs           interface{}        `json:"logprobs"`
	FinishReason       string             `json:"finish_reason"`
	NativeFinishReason string             `json:"native_finish_reason"`
	Index              int                `json:"index"`
	Message            *OpenRouterMessage `json:"message,omitempty"`
}

type LLMRole string

const (
	LLMRoleUser      LLMRole = "user"
	LLMRoleAssistant LLMRole = "assistant"
	LLMRoleSystem    LLMRole = "system"
)

type OpenRouterMessage struct {
	Role             LLMRole                     `json:"role"`
	Content          string                      `json:"content"`
	Refusal          *string                     `json:"refusal,omitempty"`
	Reasoning        string                      `json:"reasoning"`
	ReasoningDetails []OpenRouterReasoningDetail `json:"reasoning_details"`
}

type OpenRouterReasoningDetail struct {
	Format string `json:"format"`
	Index  int    `json:"index"`
	Type   string `json:"type"`
	Text   string `json:"text"`
}

type OpenRouterUsage struct {
	PromptTokens            int                                `json:"prompt_tokens"`
	CompletionTokens        int                                `json:"completion_tokens"`
	TotalTokens             int                                `json:"total_tokens"`
	Cost                    float64                            `json:"cost"`
	IsByok                  bool                               `json:"is_byok"`
	PromptTokensDetails     *OpenRouterPromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CostDetails             *OpenRouterCostDetails             `json:"cost_details,omitempty"`
	CompletionTokensDetails *OpenRouterCompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

type OpenRouterPromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
	AudioTokens  int `json:"audio_tokens"`
}

type OpenRouterCostDetails struct {
	UpstreamInferenceCost            float64 `json:"upstream_inference_cost"`
	UpstreamInferencePromptCost      float64 `json:"upstream_inference_prompt_cost"`
	UpstreamInferenceCompletionsCost float64 `json:"upstream_inference_completions_cost"`
}

type OpenRouterCompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
	AudioTokens     int `json:"audio_tokens"`
}

type OpenRouterCompletionRequestMessage struct {
	Role    LLMRole `json:"role"`
	Content string  `json:"content"`
}

type OpenRouterCompletionRequestReasoning struct {
	Enabled bool `json:"enabled"`
}

type OpenRouterCompletionRequest struct {
	Model     string                               `json:"model"`
	Messages  []OpenRouterCompletionRequestMessage `json:"messages"`
	Reasoning OpenRouterCompletionRequestReasoning `json:"reasoning"`
}
