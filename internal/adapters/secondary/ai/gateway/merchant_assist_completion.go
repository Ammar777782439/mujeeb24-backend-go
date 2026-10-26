package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantassist"
)

// MerchantAssistCompletion reuses the existing native provider router,
// including its error/fallback policy. No tools or database mutations.
type MerchantAssistCompletion struct {
	Client *native.CompletionClient
	Model  string
}

func (g *MerchantAssistCompletion) Generate(ctx context.Context, system, user string) (string, error) {
	if g == nil || g.Client == nil || g.Model == "" || len(user) > 30000 || len(system) > 3000 {
		return "", errors.New("merchant assist model is unavailable or input exceeds limits")
	}
	request := map[string]any{
		"model":       g.Model,
		"messages":    []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
		"max_tokens":  700,
		"stream":      false,
	}
	raw, _, err := g.Client.Complete(ctx, request)
	if err != nil {
		return "", err
	}
	var out struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string  `json:"content"`
				Refusal *string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", errors.New("merchant assist invalid model response")
	}
	if len(out.Choices) != 1 || out.Choices[0].FinishReason != "stop" || out.Choices[0].Message.Refusal != nil {
		return "", errors.New("merchant assist model response incomplete or refused")
	}
	text := strings.TrimSpace(out.Choices[0].Message.Content)
	if text == "" || len(text) > 4000 {
		return "", errors.New("merchant assist model returned invalid answer length")
	}
	return text, nil
}

var _ merchantassist.TextGenerator = (*MerchantAssistCompletion)(nil)
