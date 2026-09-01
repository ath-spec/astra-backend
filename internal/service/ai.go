package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/yourusername/astra-backend/internal/repository"
)

const groqAPIURL = "https://api.groq.com/openai/v1/chat/completions"

type AIService interface {
	GetChatCompletion(ctx context.Context, userID uuid.UUID, messages []map[string]interface{}) ([]byte, int, error)
	GetTextToSpeech(ctx context.Context, text string) ([]byte, int, error)
}

type GroqAIService struct {
	groqAPIKey   string
	sarvamAPIKey string
	client       *http.Client
	chatRepo     repository.ChatRepository
}

func NewGroqAIService(groqAPIKey, sarvamAPIKey string, chatRepo repository.ChatRepository) *GroqAIService {
	return &GroqAIService{
		groqAPIKey:   groqAPIKey,
		sarvamAPIKey: sarvamAPIKey,
		client:       &http.Client{},
		chatRepo:     chatRepo,
	}
}

func (s *GroqAIService) GetChatCompletion(ctx context.Context, userID uuid.UUID, messages []map[string]interface{}) ([]byte, int, error) {
	modelsToTry := []string{"openai/gpt-oss-120b", "openai/gpt-oss-20b"}

	var lastBodyBytes []byte
	var lastStatusCode int
	var lastErr error

	for _, model := range modelsToTry {
		groqPayload := map[string]interface{}{
			"model":    model,
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

		req.Header.Set("Authorization", "Bearer "+s.groqAPIKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("error contacting Groq API with model %s: %w", model, err)
			continue // Try fallback
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("error reading response body for model %s: %w", model, err)
			continue // Try fallback
		}

		lastStatusCode = resp.StatusCode
		lastBodyBytes = bodyBytes

		// If successful, save clean dialogue history and return immediately
		if resp.StatusCode == http.StatusOK {
			var groqResp struct {
				Choices []struct {
					Message map[string]interface{} `json:"message"`
				} `json:"choices"`
			}

			if err := json.Unmarshal(bodyBytes, &groqResp); err == nil && len(groqResp.Choices) > 0 {
				assistantMsg := groqResp.Choices[0].Message

				// Filter out system messages so only dialogue history is persisted
				dialogueMessages := make([]map[string]interface{}, 0, len(messages)+1)
				for _, msg := range messages {
					if role, ok := msg["role"].(string); ok && role != "system" {
						dialogueMessages = append(dialogueMessages, msg)
					}
				}
				dialogueMessages = append(dialogueMessages, assistantMsg)

				// Keep sliding window of last 20 messages to prevent token explosion
				if len(dialogueMessages) > 20 {
					dialogueMessages = dialogueMessages[len(dialogueMessages)-20:]
				}

				session, err := s.chatRepo.GetSessionForUser(ctx, userID)
				if err == nil {
					session.Messages = dialogueMessages
					_ = s.chatRepo.SaveSession(ctx, session)
				}
			}
			return bodyBytes, resp.StatusCode, nil
		}

		// If status is not 200, it's an API error, continue to fallback
	}

	// If all models fail, return the last error encountered (or the last non-200 response)
	if lastStatusCode != 0 {
		return lastBodyBytes, lastStatusCode, nil
	}
	return nil, http.StatusBadGateway, lastErr
}

func (s *GroqAIService) GetTextToSpeech(ctx context.Context, text string) ([]byte, int, error) {
	url := "https://api.sarvam.ai/text-to-speech"

	payload := map[string]interface{}{
		"inputs":               []string{text},
		"target_language_code": "en-IN",
		"speaker":              "shubh",
		"model":                "bulbul:v3",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	req.Header.Set("api-subscription-key", s.sarvamAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	return bodyBytes, resp.StatusCode, err
}
