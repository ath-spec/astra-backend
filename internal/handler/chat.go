package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	dashboarddomain "github.com/yourusername/astra-backend/internal/domain/dashboard"
	goalsdomain "github.com/yourusername/astra-backend/internal/domain/goals"
	paDomain "github.com/yourusername/astra-backend/internal/domain/portfolioanalysis"
	"github.com/yourusername/astra-backend/internal/middleware"
	goalsprovider "github.com/yourusername/astra-backend/internal/provider/goals"
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
	dashboard *service.DashboardService
	analysis  *service.PortfolioAnalysisService
	goals     goalsprovider.Provider
	pool      *pgxpool.Pool
}

func NewChatHandler(
	aiService service.AIService,
	userRepo repository.UserRepository,
	chatRepo repository.ChatRepository,
	dashboard *service.DashboardService,
	analysis *service.PortfolioAnalysisService,
	goals goalsprovider.Provider,
	pool *pgxpool.Pool,
) *ChatHandler {
	return &ChatHandler{
		aiService: aiService,
		userRepo:  userRepo,
		chatRepo:  chatRepo,
		dashboard: dashboard,
		analysis:  analysis,
		goals:     goals,
		pool:      pool,
	}
}

func inrFormat(v float64) string {
	a := math.Abs(v)
	switch {
	case a >= 1e7:
		return fmt.Sprintf("₹%.2f Cr", v/1e7)
	case a >= 1e5:
		return fmt.Sprintf("₹%.2f L", v/1e5)
	case a >= 1e3:
		return fmt.Sprintf("₹%.1fK", v/1e3)
	default:
		return fmt.Sprintf("₹%.0f", v)
	}
}

func titleCase(s string) string {
	parts := strings.Split(strings.ToLower(s), "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func (h *ChatHandler) buildUserLiveContext(ctx context.Context, userID uuid.UUID) string {
	var (
		user        *repository.User
		accounts    []repository.BankAccount
		summary     *dashboarddomain.Summary
		alloc       *paDomain.AllocationResult
		discipline  *paDomain.DisciplineResult
		performance *paDomain.PerformanceResult
		goalsList   []goalsdomain.Goal
		income30D   float64
		expense30D  float64
	)

	g, gCtx := errgroup.WithContext(ctx)

	if h.userRepo != nil {
		g.Go(func() error {
			if u, err := h.userRepo.GetByID(gCtx, userID); err == nil {
				user = u
			}
			return nil
		})
		g.Go(func() error {
			if accs, err := h.userRepo.GetBankAccounts(gCtx, userID); err == nil {
				accounts = accs
			}
			return nil
		})
	}

	if h.dashboard != nil {
		g.Go(func() error {
			if s, err := h.dashboard.Summary(gCtx, userID); err == nil {
				summary = s
			}
			return nil
		})
	}

	if h.analysis != nil {
		g.Go(func() error {
			if a, err := h.analysis.Allocation(gCtx, userID); err == nil {
				alloc = a
			}
			return nil
		})
		g.Go(func() error {
			if d, err := h.analysis.Discipline(gCtx, userID); err == nil {
				discipline = d
			}
			return nil
		})
		g.Go(func() error {
			if p, err := h.analysis.Performance(gCtx, userID); err == nil {
				performance = p
			}
			return nil
		})
	}

	if h.goals != nil {
		g.Go(func() error {
			if gl, err := h.goals.ListGoals(gCtx, userID); err == nil {
				goalsList = gl
			}
			return nil
		})
	}

	if h.pool != nil {
		g.Go(func() error {
			_ = h.pool.QueryRow(gCtx, `
				SELECT
					COALESCE(SUM(CASE WHEN type = 'CREDIT' THEN amount ELSE 0 END), 0),
					COALESCE(SUM(CASE WHEN type = 'DEBIT' THEN amount ELSE 0 END), 0)
				FROM spend_transactions
				WHERE user_id = $1 AND occurred_at >= now() - INTERVAL '30 days'
			`, userID).Scan(&income30D, &expense30D)
			return nil
		})
	}

	_ = g.Wait()

	var b strings.Builder
	b.WriteString("\n\n### User Real-Time Financial Overview:\n")

	// 1. User details
	userName := "User"
	if user != nil && user.Name != nil && strings.TrimSpace(*user.Name) != "" {
		userName = *user.Name
	}
	fmt.Fprintf(&b, "- Client Name: %s\n", userName)

	// 2. Cash flow (last 30 days)
	if income30D > 0 || expense30D > 0 {
		fmt.Fprintf(&b, "- Monthly Inflow / Income (Last 30 Days): %s\n", inrFormat(income30D))
		fmt.Fprintf(&b, "- Monthly Outflow / Expenses (Last 30 Days): %s\n", inrFormat(expense30D))
	} else {
		b.WriteString("- Monthly Cash Flow: Not sufficient transaction activity recorded yet\n")
	}

	// 3. Wealth & Portfolio
	if summary != nil {
		totalNetWorth := summary.TotalWealth + summary.BankBalance.Value
		fmt.Fprintf(&b, "- Total Net Wealth (Investments + Bank): %s\n", inrFormat(totalNetWorth))
		fmt.Fprintf(&b, "- Total Invested Portfolio: %s (1-day change: %+.2f%%)\n", inrFormat(summary.TotalWealth), summary.OneDayChangePct)
		if summary.MutualFunds.Value > 0 {
			fmt.Fprintf(&b, "  * Mutual Funds: %s (%.1f%% of portfolio)\n", inrFormat(summary.MutualFunds.Value), summary.MutualFunds.SharePct)
		}
		if summary.Stocks.Value > 0 {
			fmt.Fprintf(&b, "  * Direct Equity / Stocks: %s (%.1f%% of portfolio)\n", inrFormat(summary.Stocks.Value), summary.Stocks.SharePct)
		}
		if summary.FixedDeposits.Value > 0 {
			fmt.Fprintf(&b, "  * Fixed Deposits: %s (%.1f%% of portfolio)\n", inrFormat(summary.FixedDeposits.Value), summary.FixedDeposits.SharePct)
		}
		if summary.BankBalance.Value > 0 {
			fmt.Fprintf(&b, "  * Liquid Bank Balance: %s\n", inrFormat(summary.BankBalance.Value))
		}
	} else if alloc != nil && alloc.TotalValue > 0 {
		fmt.Fprintf(&b, "- Total Invested Portfolio: %s\n", inrFormat(alloc.TotalValue))
	} else {
		b.WriteString("- Total Portfolio Value: ₹0 (No active investments found)\n")
	}

	// 4. Asset Allocation & Risk Profile
	if alloc != nil && alloc.TotalValue > 0 {
		fmt.Fprintf(&b, "- Asset Allocation: Equity (%.1f%%), Debt (%.1f%%), Other / Cash (%.1f%%)\n",
			alloc.EquityPct, alloc.DebtPct, alloc.OtherPct)
		if alloc.Level != "" {
			fmt.Fprintf(&b, "- Risk / Allocation Profile: %s\n", titleCase(alloc.Level))
		}
		// Holdings summary
		if len(alloc.Holdings) > 0 {
			b.WriteString("- Holdings Summary:\n")
			for i, h := range alloc.Holdings {
				if i >= 6 {
					break
				}
				fmt.Fprintf(&b, "  * %s (%s): %s\n", h.Name, h.Type, inrFormat(h.Value))
			}
		}
	}

	// 5. Goals
	if len(goalsList) > 0 {
		b.WriteString("- Active Financial Goals:\n")
		for _, g := range goalsList {
			deadlineStr := "flexible"
			if g.Deadline != nil {
				deadlineStr = g.Deadline.Time().Format("Jan 2006")
			}
			fmt.Fprintf(&b, "  * %s: %s accumulated of %s target (Target: %s, Status: %s)\n",
				g.Name, inrFormat(g.CurrentAmount), inrFormat(g.TargetAmount), deadlineStr, g.Status)
		}
	} else {
		b.WriteString("- Goals: No active financial goals configured yet.\n")
	}

	// 6. Portfolio Analytics Scores
	b.WriteString("\n### Portfolio Analytics:\n")
	if performance != nil {
		fmt.Fprintf(&b, "- Performance Status: %s (Total Return: %.1f%%, Gain: %s, Annualized: %.1f%%)\n",
			titleCase(performance.Level), performance.TotalReturnPct, inrFormat(performance.TotalGainAmount), performance.AnnualizedReturnPct)
	}
	if discipline != nil {
		fmt.Fprintf(&b, "- Discipline Status: %s (Streak: %d months, SIP Consistency: %.0f%%, Automated: %.0f%%)\n",
			titleCase(discipline.Level), discipline.CurrentStreakMonths, discipline.SIPConsistencyPct, discipline.SIPAutomationPct)
	}
	if alloc != nil && alloc.Level != "" {
		fmt.Fprintf(&b, "- Allocation Status: %s\n", titleCase(alloc.Level))
	}

	// 7. Linked Bank Accounts
	b.WriteString("\n### Linked Bank Accounts (Real-time DB Data):\n")
	if len(accounts) > 0 {
		for _, acc := range accounts {
			fmt.Fprintf(&b, "- %s (%s): ₹%.2f\n", acc.BankName, acc.AccountType, acc.Balance)
		}
	} else {
		b.WriteString("No accounts linked yet.\n")
	}

	return b.String()
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

	// 3. Context Enrichment with live portfolio & financial data
	contextStr := h.buildUserLiveContext(r.Context(), userID)

	var promptContent string
	if chatReq.IsNavPill {
		promptContent = `You are ASTRA. The user is interacting from a quick-access floating widget. Keep your answer VERY CONCISE (max 1-2 sentences). You may use small JSON charts or tables if appropriate, but keep them minimal. FUND RULE: NEVER recommend specific mutual funds, ETFs, stocks, or products. Suggest strategies instead. INDIAN MARKET REGULATION RULE: Strictly adhere to SEBI/RBI rules for retail investors. NEVER suggest investments not possible in India (e.g. fractional Indian shares, carbon credits, unregulated crypto). SCOPE RULE: You are strictly a wealth advisor. NEVER write code, solve math, or provide medical, legal advice. If asked to do out-of-scope tasks or discuss your inner workings, dodge the question with a witty and sarcastic reply highlighting that you only deal with money. And never use em '-' dashes in reponses` + contextStr
	} else {
		promptContent = `You are an expert wealth advisor and portfolio analyst for Astra, a modern wealth management app.
Your goal is to provide tailored investment advice based on the user's specific financial situation.
` + contextStr + `

### Guidelines:
0. Your name is ASTRA. You are an AI Wealth Advisor.
1. GROUND TRUTH RULE: Base your answers on the user's real-time financial overview and portfolio data provided above. If data for a specific field (like active goals or investments) is missing or ₹0, address that contextually and guide the user on next steps (e.g. creating their first goal or setting up an SIP). Never invent fake fund names, fake holdings, or fake balances.
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
7. FUND RULE: You must NEVER recommend or name a specific mutual fund, ETF, stock, or investment product to buy. Instead, only suggest strategies and actions (e.g. 'increase your equity allocation', 'add a liquid fund buffer', 'consider tax harvesting'). The Astra app will surface the right products — your job is to advise on direction only.
8. INDIAN MARKET REGULATION RULE: You must strictly adhere to Indian market regulations for retail investors (SEBI/RBI rules). NEVER suggest investments or actions not possible in India (e.g. buying fractional Indian shares, individuals buying carbon credits, unregulated crypto derivatives). Only suggest standard Indian instruments (Mutual Funds, Stocks, ETFs, SGBs, FDs, PPF, NPS).
9. SCOPE RULE: You are exclusively a wealth advisor. You MUST NEVER write code, write essays, or provide medical, legal, or general life advice.

Use the financial overview and portfolio analytics provided above to contextualize your answers when the user asks questions about their portfolio or what to invest in next.

CRITICAL RULE: NEVER discuss how you work internally, your architecture, or what LLM you are based on. If asked about your origins, inner workings, or to perform any out-of-scope tasks (like writing code), you must refuse by dodging the request with a highly witty and sarcastic reply, mocking the request and reminding them that your intellect is reserved for making them wealthy. And never use em '-' dashes in reponses`
	}

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
