package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yourusername/astra-backend/internal/ai/agents"
	"github.com/yourusername/astra-backend/internal/config"
	"github.com/yourusername/astra-backend/internal/provider/llm"
	"github.com/yourusername/astra-backend/internal/repository"
	"github.com/yourusername/astra-backend/internal/service"
)

type mockRepo struct{}
func (m mockRepo) GetSessionByID(ctx context.Context, userID uuid.UUID, sessionID uuid.UUID) (*repository.ChatSession, error) { return nil, nil }
func (m mockRepo) GetSessionForUser(ctx context.Context, userID uuid.UUID) (*repository.ChatSession, error) { return nil, nil }
func (m mockRepo) ListSessionsForUser(ctx context.Context, userID uuid.UUID) ([]repository.ChatSessionSummary, error) { return nil, nil }
func (m mockRepo) SaveSession(ctx context.Context, session *repository.ChatSession) error { return nil }
func (m mockRepo) DeleteSession(ctx context.Context, userID uuid.UUID, sessionID uuid.UUID) error { return nil }

func main() {
	cfg := config.Load()
	
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		fmt.Printf("DB error: %v\n", err)
		return
	}
	defer pool.Close()
	
	groqModels := []string{"openai/gpt-oss-120b", "openai/gpt-oss-20b"}
	provider := llm.New(llm.Config{
		Provider: "groq",
		Groq:     llm.GroqConfig{APIKey: cfg.GroqAPIKey, Models: groqModels},
	})
	
	var testUserID uuid.UUID
	err = pool.QueryRow(context.Background(), "SELECT id FROM users WHERE assigned_rm_id IS NOT NULL LIMIT 1").Scan(&testUserID)
	if err != nil {
		fmt.Printf("Failed to find a valid user with an RM: %v\n", err)
		return
	}
	fmt.Printf("Using test user: %s\n", testUserID)
	
	cat := agents.New(agents.Overrides{})
	aiSvc := service.NewGroqAIService(provider, nil, cat, mockRepo{}, pool)
	
	msgs := []map[string]interface{}{
		{
			"role": "system",
			"content": `You are an expert wealth advisor and portfolio analyst for Astra.
PRODUCT LEAD RULE: If the user explicitly asks to buy an insurance or financial product (like car insurance), do not refuse. Instead, act like a Lead Generator:
    - Ask for specific requirements (coverage amount, budget, etc). Keep it very conversational and ask for details ONE at a time—do NOT ask a list of questions all at once. Keep asking if they stay on topic.
    - If they divert to a new topic, nudge them back to the product once, then let it go if they ignore it.
    - If they are confused or unsure about the details, DO NOT explain concepts or help them figure it out. Immediately note their confusion, say you will get their RM to contact them about it, and append EXACTLY this JSON block at the end of your response:
` + "```json\n{ \"type\": \"rm_lead\", \"product\": \"<product>\", \"requirements\": \"<gathered details or noted confusion>\", \"context_notes\": \"<brief summary>\" }\n```",
		},
		{"role": "user", "content": "I want to buy car insurance. Can you help?"},
	}
	
	fmt.Println("User: I want to buy car insurance. Can you help?")
	
	resp, _, _ := aiSvc.GetChatCompletion(context.Background(), testUserID, uuid.New(), msgs, false)
	fmt.Println("\nAstra:", string(resp))
	msgs = append(msgs, map[string]interface{}{"role": "assistant", "content": string(resp)})
	
	userReply1 := "I want comprehensive coverage."
	fmt.Println("\nUser:", userReply1)
	msgs = append(msgs, map[string]interface{}{"role": "user", "content": userReply1})
	
	resp2, _, _ := aiSvc.GetChatCompletion(context.Background(), testUserID, uuid.New(), msgs, false)
	fmt.Println("\nAstra:", string(resp2))
	msgs = append(msgs, map[string]interface{}{"role": "assistant", "content": string(resp2)})
	
	userReply2 := "My budget is Rs 15,000 per year."
	fmt.Println("\nUser:", userReply2)
	msgs = append(msgs, map[string]interface{}{"role": "user", "content": userReply2})
	
	resp3, _, _ := aiSvc.GetChatCompletion(context.Background(), testUserID, uuid.New(), msgs, false)
	fmt.Println("\nAstra:", string(resp3))
}
