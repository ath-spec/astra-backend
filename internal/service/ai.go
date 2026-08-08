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
}

type GroqAIService struct {
	apiKey   string
	client   *http.Client
	chatRepo repository.ChatRepository
}

func NewGroqAIService(apiKey string, chatRepo repository.ChatRepository) *GroqAIService {
	return &GroqAIService{
		apiKey:   apiKey,
		client:   &http.Client{},
		chatRepo: chatRepo,
	}
}

func (s *GroqAIService) GetChatCompletion(ctx context.Context, userID uuid.UUID, messages []map[string]interface{}) ([]byte, int, error) {
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

	// If successful, save the chat history to the database
	if resp.StatusCode == http.StatusOK {
		// Parse the AI's response so we can append it to the history
		var groqResp struct {
			Choices []struct {
				Message map[string]interface{} `json:"message"`
			} `json:"choices"`
		}
		
		if err := json.Unmarshal(bodyBytes, &groqResp); err == nil && len(groqResp.Choices) > 0 {
			updatedMessages := append(messages, groqResp.Choices[0].Message)
			
			// Fetch existing session or create a new one
			session, err := s.chatRepo.GetSessionForUser(ctx, userID)
			if err == nil {
				session.Messages = updatedMessages
				_ = s.chatRepo.SaveSession(ctx, session) // We ignore the error here so we don't fail the user request if DB save fails
			}
		}
	}

	return bodyBytes, resp.StatusCode, nil
}
