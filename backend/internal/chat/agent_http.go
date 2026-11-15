package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// HTTPAgent sends chat requests to the ML service.
type HTTPAgent struct {
	endpoint string
	client   *http.Client
}

func NewHTTPAgent(endpoint string, client *http.Client) *HTTPAgent {
	if client == nil {
		client = &http.Client{}
	}
	return &HTTPAgent{endpoint: strings.TrimSpace(endpoint), client: client}
}

func (a *HTTPAgent) Reply(ctx context.Context, input AgentRequest) (AgentResponse, error) {
	if a.endpoint == "" {
		return AgentResponse{}, fmt.Errorf("ML_AGENT_URL is empty")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return AgentResponse{}, fmt.Errorf("encode ML request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return AgentResponse{}, fmt.Errorf("create ML request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return AgentResponse{}, fmt.Errorf("call ML service: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.WarnContext(ctx, "закрыть ответ ML-сервиса", "error", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return AgentResponse{}, fmt.Errorf("ML service returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	var result AgentResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return AgentResponse{}, fmt.Errorf("decode ML response: %w", err)
	}
	if strings.TrimSpace(result.Text) == "" {
		return AgentResponse{}, fmt.Errorf("ML service returned an empty answer")
	}
	return result, nil
}
