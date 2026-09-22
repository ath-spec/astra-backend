package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/ai/agents"
	"github.com/yourusername/astra-backend/internal/provider/llm"
	"github.com/yourusername/astra-backend/internal/provider/speech"
	"github.com/yourusername/astra-backend/internal/repository"
)

// AIService powers the in-app ASTRA wealth-advisor chat and its text-to-speech.
type AIService interface {
	// GetChatCompletion runs one advisor turn. quick selects the terse
	// nav-pill agent instead of the full advisor. sessionID is the specific
	// thread this turn is saved into (the handler already resolved it) —
	// without this, every save re-resolved "the most recent session for this
	// user" itself, which meant there could only ever be one growing thread
	// per user no matter what the caller intended. The returned bytes are
	// the OpenAI-style chat-completions envelope the frontend already parses.
	GetChatCompletion(ctx context.Context, userID, sessionID uuid.UUID, messages []map[string]interface{}, quick bool) ([]byte, int, error)
	// GetTextToSpeech synthesizes text in the caller-supplied language/script
	// (BCP-47-ish, e.g. "hi-IN", "ta-IN"); empty falls back to "en-IN".
	GetTextToSpeech(ctx context.Context, text string, language string) ([]byte, int, error)
	// GetSpeechToText transcribes audio with Sarvam's auto language detection —
	// the user may speak in any supported language/script, no forcing to English.
	GetSpeechToText(ctx context.Context, audio []byte, filename string) (string, error)
	// GetSpeechToTextStream bridges an already-upgraded client WebSocket to
	// the realtime STT stream for live partial transcripts while the user
	// speaks. Blocks until the stream ends; the caller owns clientConn.
	GetSpeechToTextStream(ctx context.Context, clientConn *websocket.Conn, language string) error
}

// GroqAIService is the AIService implementation. It routes through the
// provider-agnostic llm / speech seams; today those resolve to Groq + Sarvam,
// and this type keeps their exact model lists, envelope shape and history
// behaviour. Switching LLM_PROVIDER / SPEECH_PROVIDER changes the backend with
// no change here.
type GroqAIService struct {
	llm      llm.Provider
	speech   speech.Provider
	agents   *agents.Catalog
	chatRepo repository.ChatRepository
	pool     *pgxpool.Pool
}

func NewGroqAIService(llmProvider llm.Provider, speechProvider speech.Provider, cat *agents.Catalog, chatRepo repository.ChatRepository, pool *pgxpool.Pool) *GroqAIService {
	return &GroqAIService{
		llm:      llmProvider,
		speech:   speechProvider,
		agents:   cat,
		chatRepo: chatRepo,
		pool:     pool,
	}
}

func (s *GroqAIService) GetChatCompletion(ctx context.Context, userID, sessionID uuid.UUID, messages []map[string]interface{}, quick bool) ([]byte, int, error) {
	agentKey := agents.KeyAppChat
	if quick {
		agentKey = agents.KeyAppQuickChat
	}
	system, msgs := splitChatMessages(messages)

	resp, err := s.llm.Complete(ctx, s.agents.Get(agentKey).Request(system, msgs))
	if err != nil {
		if errors.Is(err, llm.ErrNotConfigured) {
			return jsonErrBody("AI is not configured on this environment"), http.StatusServiceUnavailable, nil
		}
		return jsonErrBody("Error processing chat request"), http.StatusBadGateway, err
	}
	if resp == nil || resp.Text == "" {
		return jsonErrBody("The assistant returned an empty response"), http.StatusBadGateway,
			errors.New("ai: empty completion")
	}

	// Intercept Product Lead JSON if present
	leadRegex := regexp.MustCompile("(?s)```json\\s*({[^}]*\"type\"\\s*:\\s*\"rm_lead\"[^}]*})\\s*```")
	matches := leadRegex.FindStringSubmatch(resp.Text)
	if len(matches) > 1 {
		// Strip from text so frontend just sees standard dialogue
		resp.Text = strings.TrimSpace(leadRegex.ReplaceAllString(resp.Text, ""))

		var leadPayload map[string]interface{}
		_ = json.Unmarshal([]byte(matches[1]), &leadPayload)

		// Format the transcript (last 6 messages)
		var transcriptBuilder strings.Builder
		startIdx := 0
		if len(messages) > 6 {
			startIdx = len(messages) - 6
		}
		for i := startIdx; i < len(messages); i++ {
			role, _ := messages[i]["role"].(string)
			content, _ := messages[i]["content"].(string)
			transcriptBuilder.WriteString(fmt.Sprintf("**%s**: %s\n\n", strings.ToUpper(role), content))
		}
		leadPayload["transcript"] = transcriptBuilder.String()
		
		if finalJSON, err := json.Marshal(leadPayload); err == nil {
			go s.logLeadForRM(userID, string(finalJSON))
		}
	}

	// Persist the clean dialogue history (dialogue turns only, sliding window
	// of 20) exactly as before.
	assistant := map[string]interface{}{"role": "assistant", "content": resp.Text}
	dialogue := make([]map[string]interface{}, 0, len(messages)+1)
	for _, m := range messages {
		if role, _ := m["role"].(string); role != "system" {
			dialogue = append(dialogue, m)
		}
	}
	dialogue = append(dialogue, assistant)
	if len(dialogue) > 20 {
		dialogue = dialogue[len(dialogue)-20:]
	}
	_ = s.chatRepo.SaveSession(ctx, &repository.ChatSession{
		ID:       sessionID,
		UserID:   userID,
		Messages: dialogue,
	})

	return marshalChatEnvelope(resp), http.StatusOK, nil
}

func (s *GroqAIService) GetTextToSpeech(ctx context.Context, text string, language string) ([]byte, int, error) {
	if strings.TrimSpace(language) == "" {
		language = detectLanguageCode(text)
	}
	res, err := s.speech.TextToSpeech(ctx, speech.TTSRequest{Text: text, Language: language})
	if err != nil {
		if errors.Is(err, speech.ErrNotConfigured) {
			return jsonErrBody("Voice is not configured on this environment"), http.StatusServiceUnavailable, nil
		}
		return nil, http.StatusBadGateway, err
	}
	// Sarvam's body is JSON carrying base64 audio under "audios" — the client
	// already consumes that shape, so forward it unchanged.
	return res.Audio, http.StatusOK, nil
}

// GetSpeechToText forwards to the speech seam's STT (Sarvam saarika with auto
// language detection today; Transcribe when SPEECH_PROVIDER=aws), returning
// the transcript exactly as recognized — no romanization/transliteration.
func (s *GroqAIService) GetSpeechToText(ctx context.Context, audio []byte, filename string) (string, error) {
	if len(audio) == 0 {
		return "", fmt.Errorf("empty audio")
	}
	res, err := s.speech.SpeechToText(ctx, speech.STTRequest{Audio: audio, Filename: filename})
	if err != nil {
		if errors.Is(err, speech.ErrNotConfigured) {
			return "", fmt.Errorf("voice is not configured on this environment")
		}
		return "", err
	}
	return strings.TrimSpace(res.Text), nil
}

// GetSpeechToTextStream delegates to the speech seam's realtime STT proxy.
func (s *GroqAIService) GetSpeechToTextStream(ctx context.Context, clientConn *websocket.Conn, language string) error {
	if s.speech == nil {
		return fmt.Errorf("voice is not configured on this environment")
	}
	return s.speech.SpeechToTextStream(ctx, clientConn, language)
}

// splitChatMessages turns the handler's []map message list into a system prompt
// plus typed dialogue messages. Multiple system messages are concatenated.
func splitChatMessages(in []map[string]interface{}) (string, []llm.Message) {
	var system string
	msgs := make([]llm.Message, 0, len(in))
	for _, m := range in {
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		if role == "system" {
			if system == "" {
				system = content
			} else {
				system += "\n\n" + content
			}
			continue
		}
		if role == "" {
			role = llm.RoleUser
		}
		msgs = append(msgs, llm.Message{Role: role, Content: content})
	}
	return system, msgs
}

// marshalChatEnvelope re-emits an llm.Response in the OpenAI chat-completions
// shape the frontend and extractAssistantContent expect.
func marshalChatEnvelope(resp *llm.Response) []byte {
	env := map[string]interface{}{
		"model": resp.Model,
		"choices": []map[string]interface{}{
			{"message": map[string]interface{}{"role": "assistant", "content": resp.Text}},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     resp.Usage.PromptTokens,
			"completion_tokens": resp.Usage.CompletionTokens,
		},
	}
	b, _ := json.Marshal(env)
	return b
}

func jsonErrBody(msg string) []byte {
	b, _ := json.Marshal(map[string]interface{}{"error": msg})
	return b
}

func (s *GroqAIService) logLeadForRM(userID uuid.UUID, jsonPayload string) {
	if s.pool == nil {
		return
	}
	ctx := context.Background()
	query := `
		INSERT INTO rm_client_interactions (user_id, rm_id, kind, body, follow_up_at)
		SELECT $1, assigned_rm_id, 'task', $2, now()
		FROM users 
		WHERE id = $1 AND assigned_rm_id IS NOT NULL
	`
	_, err := s.pool.Exec(ctx, query, userID, jsonPayload)
	if err != nil {
		fmt.Printf("Failed to log lead: %v\n", err)
	} else {
		fmt.Printf("Successfully logged lead for user %s: %s\n", userID, jsonPayload)
	}
}
