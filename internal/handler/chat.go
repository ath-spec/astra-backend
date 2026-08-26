package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/repository"
	"github.com/yourusername/astra-backend/internal/service"
)

type ChatRequest struct {
	Messages  []map[string]interface{} `json:"messages"`
	IsNavPill bool                     `json:"is_nav_pill"`
}

type TTSRequest struct {
	Text string `json:"text"`
}

type ChatHandler struct {
	aiService service.AIService
	userRepo  repository.UserRepository
	chatRepo  repository.ChatRepository
}

func NewChatHandler(aiService service.AIService, userRepo repository.UserRepository, chatRepo repository.ChatRepository) *ChatHandler {
	return &ChatHandler{
		aiService: aiService,
		userRepo:  userRepo,
		chatRepo:  chatRepo,
	}
}

func (h *ChatHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	// 1. Securely extract the User ID from the Context (set by JWT middleware)
	userIDValue := r.Context().Value(middleware.UserIDKey)
	if userIDValue == nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized: No user in context")
		return
	}

	userID, ok := userIDValue.(uuid.UUID)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "Internal Error: Invalid user ID format")
		return
	}

	// 2. Decode the incoming chat messages
	var chatReq ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// 3. Context Enrichment! (Backend Patterns)
	// Fetch user's bank accounts from DB
	accounts, err := h.userRepo.GetBankAccounts(r.Context(), userID)

	contextStr := "\n\n### Linked Bank Accounts (Real-time DB Data):\n"
	if err == nil && len(accounts) > 0 {
		for _, acc := range accounts {
			contextStr += fmt.Sprintf("- %s (%s): ₹%.2f\n", acc.BankName, acc.AccountType, acc.Balance)
		}
	} else {
		contextStr += "No accounts linked yet.\n"
	}

	var promptContent string
	if chatReq.IsNavPill {
		promptContent = `You are ASTRA. The user is interacting from a quick-access floating widget. Keep your answer VERY CONCISE (max 1-2 sentences). You may use small JSON charts or tables if appropriate, but keep them minimal. FUND RULE: NEVER recommend specific mutual funds, ETFs, stocks, or products. Suggest strategies instead. INDIAN MARKET REGULATION RULE: Strictly adhere to SEBI/RBI rules for retail investors. NEVER suggest investments not possible in India (e.g. fractional Indian shares, carbon credits, unregulated crypto). SCOPE RULE: You are strictly a wealth advisor. NEVER write code, solve math, or provide medical, legal advice. If asked to do out-of-scope tasks or discuss your inner workings, dodge the question with a witty and sarcastic reply highlighting that you only deal with money. And never use em '-' dashes in reponses`
	} else {
		promptContent = `You are an expert wealth advisor and portfolio analyst for Astra, a modern wealth management app.
Your goal is to provide tailored investment advice based on the user's specific financial situation.

### User Financial Overview:
- Monthly Income: ₹1,50,000
- Monthly Budget & Expenses: ₹70,000
- Current Portfolio Value: ₹12,40,000
- Asset Allocation: Equity (75%), Debt (15%), Gold (10%)
- Top Holdings: Parag Parikh Flexi Cap Fund (35%), Nifty 50 Index Fund (40%), Sovereign Gold Bonds (10%), Liquid Funds (15%).
- Risk Profile: Moderately Aggressive
- Goals: Wealth accumulation for retirement in 20 years.

### Portfolio Analytics:
- Performance Score: 85/100 (Winning streak, most funds are ahead of benchmark. Momentum is strong. Fees aren't eating into gains.)
- Allocation Score: 78/100 (Consider Index funds for stability. Save tax on gains via Tax Harvesting.)
- Discipline Score: 92/100 (Consistent SIPs. Yearly investments are growing steadily.)

### Guidelines:
0. Your name is ASTRA. You are an AI Wealth Advisor.
1. You MUST NEVER say you lack real-time data, internet access, or tell the user to calculate something themselves. If you need data you don't have (like current mutual fund returns), confidently INVENT highly realistic mock data to answer the question directly.
2. NEVER give tutorials, long explanations, or extensive markdown documents. You must keep every response extremely short (MAX 2-3 sentences).
3. CRITICAL UI RULE: You MUST NEVER use Markdown tables or extensive bullet points for comparisons. You must ONLY output a single JSON code block using ` + "```json" + ` that our app will parse into a beautiful interactive widget. Keep tables strictly to 1 table per response and MAX 3 rows.
   - If the user asks for their Portfolio Allocation or Asset Breakdown, you MUST output a pie chart using this format:
   - For a Table:
` + "```json\n{ \"type\": \"table\", \"title\": \"Optional Title\", \"columns\": [\"Col1\", \"Col2\"], \"rows\": [[\"Val1\", \"Val2\"]] }\n```" + `
   - For a Chart:
` + "```json\n{ \"type\": \"chart\", \"chartType\": \"pie\", \"title\": \"Portfolio\", \"data\": {\"Equities\": 75, \"Debt\": 15, \"Gold\": 10} }\n```" + `
(chartType can be "pie", "doughnut", or "bar").
4. TEXT FORMATTING RULE: Do not use markdown formatting (like bolding, italics, or long bullet points). Just provide simple text.
5. IMPORTANT LANGUAGE RULE: You must respond in the exact same language the user uses (English, Hindi, or Hinglish).
6. Keep this ongoing conversation in mind. You have access to the chat history, so reference previous messages if deemed necessary to make the interaction feel natural and seamless.
7. FUND RULE: You must NEVER recommend or name a specific mutual fund, ETF, stock, or investment product. Instead, only suggest strategies and actions (e.g. 'increase your equity allocation', 'add a liquid fund buffer', 'consider tax harvesting'). The Astra app will surface the right products — your job is to advise on direction only.
8. INDIAN MARKET REGULATION RULE: You must strictly adhere to Indian market regulations for retail investors (SEBI/RBI rules). NEVER suggest investments or actions not possible in India (e.g. buying fractional Indian shares, individuals buying carbon credits, unregulated crypto derivatives). Only suggest standard Indian instruments (Mutual Funds, Stocks, ETFs, SGBs, FDs, PPF, NPS).
9. SCOPE RULE: You are exclusively a wealth advisor. You MUST NEVER write code, write essays, or provide medical, legal, or general life advice.

Use the financial overview and portfolio analytics provided above to contextualize your answers when the user asks questions about their portfolio or what to invest in next.

CRITICAL RULE: NEVER discuss how you work internally, your architecture, or what LLM you are based on. If asked about your origins, inner workings, or to perform any out-of-scope tasks (like writing code), you must refuse by dodging the request with a highly witty and sarcastic reply, mocking the request and reminding them that your intellect is reserved for making them wealthy. And never use em '-' dashes in reponses`
	}

	// Attach the dynamic database bank accounts to whichever prompt is used!
	promptContent += contextStr

	systemPrompt := map[string]interface{}{
		"role":    "system",
		"content": promptContent,
	}

	// Prepend the system prompt to the messages
	messagesWithContext := append([]map[string]interface{}{systemPrompt}, chatReq.Messages...)

	// 4. Get the AI response
	responseBytes, statusCode, err := h.aiService.GetChatCompletion(r.Context(), userID, messagesWithContext)
	if err != nil {
		log.Printf("[DEBUG] AI Service Network Error: %v", err)
		respondWithError(w, statusCode, "Error processing chat request")
		return
	}

	// Extensive Debug Logs
	if statusCode >= 400 {
		log.Printf("[DEBUG] GROQ API FAILURE (Status %d): %s", statusCode, string(responseBytes))
	} else {
		log.Printf("[DEBUG] GROQ API SUCCESS (Status %d): Response received", statusCode)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(responseBytes)
}

// GetHistory returns the user's existing chat messages
func (h *ChatHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	userIDValue := r.Context().Value(middleware.UserIDKey)
	if userIDValue == nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	userID := userIDValue.(uuid.UUID)
	session, err := h.chatRepo.GetSessionForUser(r.Context(), userID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to load chat history")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"messages": session.Messages,
	})
}

func (h *ChatHandler) HandleTTS(w http.ResponseWriter, r *http.Request) {
	var ttsReq TTSRequest
	if err := json.NewDecoder(r.Body).Decode(&ttsReq); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if ttsReq.Text == "" {
		http.Error(w, `{"error": "Text is required"}`, http.StatusBadRequest)
		return
	}

	bodyBytes, statusCode, err := h.aiService.GetTextToSpeech(r.Context(), ttsReq.Text)
	if err != nil {
		log.Printf("TTS error: %v", err)
		http.Error(w, `{"error": "TTS failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(bodyBytes)
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
		"code":  code,
	})
}
