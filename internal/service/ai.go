package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const groqAPIURL = "https://api.groq.com/openai/v1/chat/completions"

type AIService interface {
	GetChatCompletion(ctx context.Context, messages []map[string]interface{}) ([]byte, int, error)
}

type GroqAIService struct {
	apiKey string
	client *http.Client
}

func NewGroqAIService(apiKey string) *GroqAIService {
	return &GroqAIService{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

func (s *GroqAIService) GetChatCompletion(ctx context.Context, messages []map[string]interface{}) ([]byte, int, error) {
	groqPayload := map[string]interface{}{
		"model":    "llama-3.1-70b-versatile",
		"messages": messages,
	}

	jsonData, err := json.Marshal(groqPayload)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("error marshalling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", groqAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("error contacting Groq API: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("error reading response body: %w", err)
	}

	return bodyBytes, resp.StatusCode, nil
}
