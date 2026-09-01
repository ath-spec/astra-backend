package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	analyticsdomain "github.com/yourusername/astra-backend/internal/domain/analytics"
	dashboarddomain "github.com/yourusername/astra-backend/internal/domain/dashboard"
	fddomain "github.com/yourusername/astra-backend/internal/domain/fd"
	goalsdomain "github.com/yourusername/astra-backend/internal/domain/goals"
	mfdomain "github.com/yourusername/astra-backend/internal/domain/mf"
	paDomain "github.com/yourusername/astra-backend/internal/domain/portfolioanalysis"
	stocksdomain "github.com/yourusername/astra-backend/internal/domain/stocks"
	watchlistdomain "github.com/yourusername/astra-backend/internal/domain/watchlist"
	"github.com/yourusername/astra-backend/internal/middleware"
	fdprovider "github.com/yourusername/astra-backend/internal/provider/fd"
	goalsprovider "github.com/yourusername/astra-backend/internal/provider/goals"
	mfprovider "github.com/yourusername/astra-backend/internal/provider/mf"
	stocksprovider "github.com/yourusername/astra-backend/internal/provider/stocks"
	"github.com/yourusername/astra-backend/internal/repository"
	"github.com/yourusername/astra-backend/internal/service"
	analyticsservice "github.com/yourusername/astra-backend/internal/service/analytics"
)

type ChatRequest struct {
	Messages  []map[string]interface{} `json:"messages"`
	IsNavPill bool                     `json:"is_nav_pill"`
}

type TTSRequest struct {
	Text string `json:"text"`
}

type ChatHandler struct {
	aiService      service.AIService
	userRepo       repository.UserRepository
	chatRepo       repository.ChatRepository
	memory         *service.MemoryService
	pii            *service.PIIRedactor
	dashboard      *service.DashboardService
	analysis       *service.PortfolioAnalysisService
	goals          goalsprovider.Provider
	stocks         stocksprovider.Provider
	mf             mfprovider.Provider
	fd             fdprovider.Provider
	watchlist      *service.WatchlistService
	spendAnalytics *analyticsservice.Service
	pool           *pgxpool.Pool
}

func NewChatHandler(
	aiService service.AIService,
	userRepo repository.UserRepository,
	chatRepo repository.ChatRepository,
	memory *service.MemoryService,
	dashboard *service.DashboardService,
	analysis *service.PortfolioAnalysisService,
	goals goalsprovider.Provider,
	stocks stocksprovider.Provider,
	mf mfprovider.Provider,
	fd fdprovider.Provider,
	watchlist *service.WatchlistService,
	spendAnalytics *analyticsservice.Service,
	pool *pgxpool.Pool,
) *ChatHandler {
	return &ChatHandler{
		aiService:      aiService,
		userRepo:       userRepo,
		chatRepo:       chatRepo,
		memory:         memory,
		pii:            service.NewPIIRedactor(),
		dashboard:      dashboard,
		analysis:       analysis,
		goals:          goals,
		stocks:         stocks,
		mf:             mf,
		fd:             fd,
		watchlist:      watchlist,
		spendAnalytics: spendAnalytics,
		pool:           pool,
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

// buildUserLiveContext gathers all non-sensitive user data concurrently and encodes it
// in a token-dense, zero-PII structured format.
func (h *ChatHandler) buildUserLiveContext(ctx context.Context, userID uuid.UUID) string {
	var (
		user           *repository.User
		accounts       []repository.BankAccount
		summary        *dashboarddomain.Summary
		alloc          *paDomain.AllocationResult
		discipline     *paDomain.DisciplineResult
		performance    *paDomain.PerformanceResult
		goalsList      []goalsdomain.Goal
		stockHoldings  []stocksdomain.Holding
		mfResult       *mfdomain.HoldingsResult
		fdAccounts     []fddomain.Account
		watchlistItems []watchlistdomain.Item
		categoryTrend  analyticsdomain.CategoryTrendResult
		recurring      analyticsdomain.RecurringResult
		incomeAnalysis analyticsdomain.IncomeResult
		income30D      float64
		expense30D     float64
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

	if h.stocks != nil {
		g.Go(func() error {
			if st, err := h.stocks.GetHoldings(gCtx, userID); err == nil {
				stockHoldings = st
			}
			return nil
		})
	}

	if h.mf != nil {
		g.Go(func() error {
			if mf, err := h.mf.GetHoldings(gCtx, userID); err == nil {
				mfResult = mf
			}
			return nil
		})
	}

	if h.fd != nil {
		g.Go(func() error {
			if fds, err := h.fd.ListFDs(gCtx, userID); err == nil {
				fdAccounts = fds
			}
			return nil
		})
	}

	if h.watchlist != nil {
		g.Go(func() error {
			if wl, err := h.watchlist.List(gCtx, userID); err == nil {
				watchlistItems = wl
			}
			return nil
		})
	}

	if h.spendAnalytics != nil {
		g.Go(func() error {
			if ct, err := h.spendAnalytics.CategoryTrend(gCtx, userID); err == nil {
				categoryTrend = ct
			}
			return nil
		})
		g.Go(func() error {
			if rec, err := h.spendAnalytics.RecurringDetection(gCtx, userID); err == nil {
				recurring = rec
			}
			return nil
		})
		g.Go(func() error {
			if inc, err := h.spendAnalytics.IncomeAnalysis(gCtx, userID); err == nil {
				incomeAnalysis = inc
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
	b.WriteString("\n\n### LIVE GROUND-TRUTH FINANCIAL STATE (TOKEN-DENSE):\n")

	// 1. User First Name (strictly ZERO PII: never expose PAN, phone, email, full credentials)
	firstName := "User"
	if user != nil && user.Name != nil && strings.TrimSpace(*user.Name) != "" {
		parts := strings.Fields(*user.Name)
		if len(parts) > 0 {
			firstName = parts[0]
		}
	}
	fmt.Fprintf(&b, "[USER: %s]\n", firstName)

	// 2. Net Worth & Portfolios
	if summary != nil {
		totalNetWorth := summary.TotalWealth + summary.BankBalance.Value
		fmt.Fprintf(&b, "[NET_WORTH: Total=%s | Invested=%s (1d:%+.1f%%) | LiquidBank=%s | MF=%s (%.0f%%) | Stocks=%s (%.0f%%) | FD=%s (%.0f%%)]\n",
			inrFormat(totalNetWorth), inrFormat(summary.TotalWealth), summary.OneDayChangePct, inrFormat(summary.BankBalance.Value),
			inrFormat(summary.MutualFunds.Value), summary.MutualFunds.SharePct,
			inrFormat(summary.Stocks.Value), summary.Stocks.SharePct,
			inrFormat(summary.FixedDeposits.Value), summary.FixedDeposits.SharePct)
	} else if alloc != nil && alloc.TotalValue > 0 {
		fmt.Fprintf(&b, "[NET_WORTH: Invested=%s]\n", inrFormat(alloc.TotalValue))
	}

	// 3. Asset Allocation & Risk Profile
	if alloc != nil && alloc.TotalValue > 0 {
		fmt.Fprintf(&b, "[ALLOCATION: Eq=%.0f%% | Debt=%.0f%% | Other=%.0f%% | RiskProfile=%s]\n",
			alloc.EquityPct, alloc.DebtPct, alloc.OtherPct, titleCase(alloc.Level))
	}

	// 4. Stocks Holdings (symbol, quantity, value, PnL)
	if len(stockHoldings) > 0 {
		b.WriteString("[STOCKS: ")
		for i, st := range stockHoldings {
			if i > 0 {
				b.WriteString(", ")
			}
			val := float64(st.Quantity) * st.LastPrice
			pnlPct := 0.0
			if st.AveragePrice > 0 {
				pnlPct = (st.LastPrice - st.AveragePrice) / st.AveragePrice * 100
			}
			fmt.Fprintf(&b, "%s (%d qty, %s, %+.1f%%)", st.TradingSymbol, st.Quantity, inrFormat(val), pnlPct)
		}
		b.WriteString("]\n")
	}

	// 5. Mutual Funds Holdings (name, value, returns)
	if mfResult != nil && len(mfResult.Folios) > 0 {
		b.WriteString("[MUTUAL_FUNDS: ")
		for i, f := range mfResult.Folios {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s (%s, %+.1f%%)", f.SchemeName, inrFormat(f.CurrentValue), f.ReturnsPct)
		}
		b.WriteString("]\n")
	}

	// 6. Fixed Deposits (principal, rate, maturity)
	if len(fdAccounts) > 0 {
		b.WriteString("[FIXED_DEPOSITS: ")
		for i, fd := range fdAccounts {
			if i > 0 {
				b.WriteString(", ")
			}
			matStr := fd.MaturityDate.Time().Format("Jan 2006")
			fmt.Fprintf(&b, "FD #%s (%s @ %.1f%%, Mat: %s)", fd.FDAccountNumber, inrFormat(fd.PrincipalAmount), fd.InterestRate, matStr)
		}
		b.WriteString("]\n")
	}

	// 7. Watchlist
	if len(watchlistItems) > 0 {
		b.WriteString("[WATCHLIST: ")
		for i, wl := range watchlistItems {
			if i >= 6 {
				break
			}
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s", wl.SchemeName)
		}
		b.WriteString("]\n")
	}

	// 8. Cash Flow (30 Days)
	if income30D > 0 || expense30D > 0 || incomeAnalysis.AvgCreditAmount > 0 {
		inc := income30D
		if inc == 0 {
			inc = incomeAnalysis.AvgCreditAmount
		}
		savRate := 0.0
		if inc > 0 && inc >= expense30D {
			savRate = (inc - expense30D) / inc * 100
		}
		fmt.Fprintf(&b, "[CASH_FLOW_30D: Income=%s | Spend=%s | SavingsRate=%.0f%%]\n",
			inrFormat(inc), inrFormat(expense30D), savRate)
	}

	// 9. Spend Categories
	if len(categoryTrend.Categories) > 0 {
		b.WriteString("[SPEND_CATEGORIES: ")
		for i, cat := range categoryTrend.Categories {
			if i >= 5 {
				break
			}
			if i > 0 {
				b.WriteString(" | ")
			}
			fmt.Fprintf(&b, "%s=%s", cat.Category, inrFormat(cat.CurrentTotal))
		}
		b.WriteString("]\n")
	}

	// 10. Recurring Subscriptions / Bills
	if len(recurring.Recurring) > 0 {
		b.WriteString("[RECURRING_BILLS: ")
		for i, rec := range recurring.Recurring {
			if i >= 5 {
				break
			}
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s (%s/%s)", rec.Merchant, inrFormat(rec.AvgAmount), strings.ToLower(rec.Frequency))
		}
		b.WriteString("]\n")
	}

	// 11. Goals
	if len(goalsList) > 0 {
		b.WriteString("[GOALS: ")
		for i, g := range goalsList {
			if i > 0 {
				b.WriteString(" | ")
			}
			deadlineStr := "flexible"
			if g.Deadline != nil {
				deadlineStr = g.Deadline.Time().Format("Jan 2006")
			}
			fmt.Fprintf(&b, "%q (Saved:%s / Target:%s, Deadline:%s, %s)",
				g.Name, inrFormat(g.CurrentAmount), inrFormat(g.TargetAmount), deadlineStr, g.Status)
		}
		b.WriteString("]\n")
	}

	// 12. Portfolio Analytics Scores
	if performance != nil || discipline != nil {
		perfStr := "N/A"
		if performance != nil {
			perfStr = fmt.Sprintf("%s (TotalReturn: %+.1f%%, Gain: %s)", titleCase(performance.Level), performance.TotalReturnPct, inrFormat(performance.TotalGainAmount))
		}
		discStr := "N/A"
		if discipline != nil {
			discStr = fmt.Sprintf("%s (%d-mo streak, %.0f%% consistency, %.0f%% automated)",
				titleCase(discipline.Level), discipline.CurrentStreakMonths, discipline.SIPConsistencyPct, discipline.SIPAutomationPct)
		}
		fmt.Fprintf(&b, "[ANALYTICS: Perf=%s | Disc=%s]\n", perfStr, discStr)
	}

	// 13. Bank Accounts (Masked zero-PII: only Bank Name + Account Type + Balance)
	if len(accounts) > 0 {
		b.WriteString("[BANK_ACCOUNTS: ")
		for i, acc := range accounts {
			if i > 0 {
				b.WriteString(" | ")
			}
			fmt.Fprintf(&b, "%s (%s, %s)", acc.BankName, acc.AccountType, inrFormat(acc.Balance))
		}
		b.WriteString("]\n")
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

	// 3. Conversation Memory Management:
	// Filter any system messages from input to maintain clean dialogue history
	cleanIncoming := make([]map[string]interface{}, 0, len(chatReq.Messages))
	for _, m := range chatReq.Messages {
		if role, ok := m["role"].(string); ok && role != "system" {
			cleanIncoming = append(cleanIncoming, m)
		}
	}

	// If client only sent the latest message, load prior dialogue history from database session
	if len(cleanIncoming) <= 1 {
		if sess, err := h.chatRepo.GetSessionForUser(r.Context(), userID); err == nil && len(sess.Messages) > 0 {
			// Combine historical dialogue turns with the new message if not already present
			if len(cleanIncoming) == 1 {
				lastMsg := cleanIncoming[0]
				lastContent, _ := lastMsg["content"].(string)
				// Check if the last message in session matches
				sessLen := len(sess.Messages)
				if sessLen > 0 {
					prevContent, _ := sess.Messages[sessLen-1]["content"].(string)
					if prevContent != lastContent {
						cleanIncoming = append(sess.Messages, lastMsg)
					} else {
						cleanIncoming = sess.Messages
					}
				}
			} else {
				cleanIncoming = sess.Messages
			}
		}
	}

	// Apply sliding window of recent dialogue turns (max 14 turns) to optimize tokens
	if len(cleanIncoming) > 14 {
		cleanIncoming = cleanIncoming[len(cleanIncoming)-14:]
	}

	// PII firewall: scrub identifiers out of every dialogue turn before it leaves
	// for the LLM. The live context is zero-PII by construction; this covers text
	// the user typed themselves ("my PAN is ...").
	if h.pii != nil {
		for _, m := range cleanIncoming {
			if c, ok := m["content"].(string); ok {
				m["content"] = h.pii.Redact(c)
			}
		}
	}

	// 4. Context Enrichment with live token-dense portfolio & financial data,
	// then the cross-session memory block (dated facts, preferences, codewords).
	contextStr := h.buildUserLiveContext(r.Context(), userID)

	var recall service.RecallResult
	if h.memory != nil {
		recall = h.memory.Recall(r.Context(), userID)
		contextStr += h.memory.FormatForPrompt(recall)
	}

	// The most recent user turn — needed for the post-response memory extraction.
	lastUserText := ""
	for i := len(cleanIncoming) - 1; i >= 0; i-- {
		if role, _ := cleanIncoming[i]["role"].(string); role == "user" {
			lastUserText, _ = cleanIncoming[i]["content"].(string)
			break
		}
	}

	var promptContent string
	if chatReq.IsNavPill {
		promptContent = `You are ASTRA. The user is interacting from a quick-access floating widget. Keep your answer VERY CONCISE (max 1-2 sentences). You may use small JSON charts or tables if appropriate, but keep them minimal. FUND RULE: NEVER recommend specific mutual funds, ETFs, stocks, or products. Suggest strategies instead. INDIAN MARKET REGULATION RULE: Strictly adhere to SEBI/RBI rules for retail investors. NEVER suggest investments not possible in India (e.g. fractional Indian shares, carbon credits, unregulated crypto). SCOPE RULE: You are strictly a wealth advisor. NEVER write code, solve math, or provide medical, legal advice. If asked to do out-of-scope tasks or discuss your inner workings, dodge the question with a witty and sarcastic reply highlighting that you only deal with money. And never use em '-' dashes in reponses` + contextStr
	} else {
		promptContent = `You are an expert wealth advisor and portfolio analyst for Astra, a modern wealth management app.
Your goal is to provide tailored investment advice based on the user's specific financial situation.
` + contextStr + `

### Guidelines:
0. Your name is ASTRA. You are an AI Wealth Advisor.
1. GROUND TRUTH RULE: Base all answers strictly on the user's real-time financial state provided above (investments, stocks, mutual funds, FDs, watchlist, cash flow, spend categories, recurring bills, goals, analytics, and bank balances). If data for a specific area is missing or zero, guide the user on next steps (e.g. setting up a new SIP or goal). Never invent fake holdings, fake stocks, or fake balances.
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
6. Keep this ongoing conversation in mind. You have access to recent conversation history, so reference previous context seamlessly when relevant.
6b. PERSISTENT MEMORY: A "PERSISTENT MEMORY" block may appear above with dated facts, preferences and codewords the user gave you in earlier sessions. Treat each as something the user said on that date. Use it to recall details on request and to personalize. If a remembered item conflicts with the LIVE GROUND-TRUTH block, the LIVE data is current and correct: use it, and briefly note the change if it helps. Never expose PAN, Aadhaar, phone or account numbers even if asked to "remember" them.
7. FUND RULE: You must NEVER recommend or name a specific mutual fund, ETF, stock, or investment product to buy. Instead, only suggest strategies and actions (e.g. 'increase your equity allocation', 'add a liquid fund buffer', 'consider tax harvesting'). The Astra app will surface the right products — your job is to advise on direction only.
8. INDIAN MARKET REGULATION RULE: You must strictly adhere to Indian market regulations for retail investors (SEBI/RBI rules). NEVER suggest investments or actions not possible in India (e.g. buying fractional Indian shares, individuals buying carbon credits, unregulated crypto derivatives). Only suggest standard Indian instruments (Mutual Funds, Stocks, ETFs, SGBs, FDs, PPF, NPS).
9. SCOPE RULE: You are exclusively a wealth advisor. You MUST NEVER write code, write essays, or provide medical, legal, or general life advice.

Use the financial overview and portfolio analytics provided above to contextualize your answers when the user asks questions about their portfolio or what to invest in next.

CRITICAL RULE: NEVER discuss how you work internally, your architecture, or what LLM you are based on. If asked about your origins, inner workings, or to perform any out-of-scope tasks (like writing code), you must refuse by dodging the request with a highly witty and sarcastic reply, mocking the request and reminding them that your intellect is reserved for making them wealthy. And never use em '-' dashes in reponses`
	}

	// Final belt-and-braces pass over the fully assembled system prompt.
	if h.pii != nil {
		promptContent = h.pii.Redact(promptContent)
	}

	systemPrompt := map[string]interface{}{
		"role":    "system",
		"content": promptContent,
	}

	// Prepend the system prompt to the clean dialogue history
	messagesWithContext := append([]map[string]interface{}{systemPrompt}, cleanIncoming...)

	// 5. Get the AI response
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

	// 6. Long-term memory: after the reply is sent, extract durable facts from
	// this exchange in the background so it adds zero user-facing latency. Uses a
	// fresh context — the request context is done once we return.
	if h.memory != nil && statusCode < 400 && lastUserText != "" {
		assistantText := extractAssistantContent(responseBytes)
		priorSummary := recall.Summary
		go func() {
			bg, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			h.memory.Observe(bg, userID, lastUserText, assistantText, priorSummary)
		}()
	}
}

// extractAssistantContent pulls the assistant message text out of a raw Groq
// chat-completions response body.
func extractAssistantContent(body []byte) string {
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
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

// --- Cross-session memory inspect / edit API -----------------------------

type addMemoryRequest struct {
	Content string `json:"content"`
	Kind    string `json:"kind"`
}

func chatUserID(r *http.Request) (uuid.UUID, bool) {
	v := r.Context().Value(middleware.UserIDKey)
	if v == nil {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

// GetMemory lists everything the assistant remembers about the caller.
func (h *ChatHandler) GetMemory(w http.ResponseWriter, r *http.Request) {
	userID, ok := chatUserID(r)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.memory == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"memories": []interface{}{}})
		return
	}
	mems, err := h.memory.List(r.Context(), userID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to load memory")
		return
	}
	out := make([]map[string]interface{}, 0, len(mems))
	for _, m := range mems {
		out = append(out, map[string]interface{}{
			"id":          m.ID,
			"kind":        m.Kind,
			"content":     m.Content,
			"salience":    m.Salience,
			"source":      m.Source,
			"captured_at": m.CapturedAt.Unix(),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"memories": out})
}

// AddMemory pins a fact the user explicitly wants remembered.
func (h *ChatHandler) AddMemory(w http.ResponseWriter, r *http.Request) {
	userID, ok := chatUserID(r)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.memory == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Memory is not enabled")
		return
	}
	var req addMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Content) == "" {
		respondWithError(w, http.StatusBadRequest, "content is required")
		return
	}
	if err := h.memory.AddUserMemory(r.Context(), userID, req.Content, req.Kind); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// DeleteMemory forgets one memory by id.
func (h *ChatHandler) DeleteMemory(w http.ResponseWriter, r *http.Request) {
	userID, ok := chatUserID(r)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if h.memory == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Memory is not enabled")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid id")
		return
	}
	deleted, err := h.memory.Forget(r.Context(), userID, id)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to forget memory")
		return
	}
	if !deleted {
		respondWithError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
		"code":  code,
	})
}
