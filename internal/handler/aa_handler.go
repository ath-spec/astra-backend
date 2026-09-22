package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/discoverypool"
	"github.com/yourusername/astra-backend/internal/events"
	authmw "github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
	"github.com/yourusername/astra-backend/internal/service/idbiaa"
	"github.com/yourusername/astra-backend/internal/service/idbiaccounts"
)

// idbiAcctNamespace derives a stable UUID for an IDBI account number so it can
// occupy the "id" field the accounts screen already expects.
var idbiAcctNamespace = uuid.MustParse("1b671a64-40d5-491e-99b0-da01ff1f3341")

// DiscoverAccounts simulates an AA discovery step: it returns the FULL
// inventory of candidate accounts (every bank in the user's archetype pool,
// discoveryAccountsPerBank accounts each) the user hasn't already linked,
// with a deterministic (stable per user+bank+slot, not random per call)
// account number and balance — so re-opening the linking screen shows the
// same candidates instead of a new set each time. This is the single source
// of "accounts available to link": there is no separate manual bank-search
// path anymore, so a user who wants a second account at a bank they already
// linked one at just sees that bank's second slot still available here.
// Discovered accounts are never written to bank_accounts; the user still
// has to select them and hit APPROVE AND CONNECT (AddAccount) for that.
func (h *AAHandler) DiscoverAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT bank_name, COALESCE(account_number, '') FROM bank_accounts WHERE user_id = $1 AND unlinked_at IS NULL
	`, userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	// Keyed by "bank name|account number" so multiple accounts at the same
	// bank are tracked independently — keying by bank name alone would make
	// linking one account at a bank hide every other candidate at that same
	// bank, even though the user might hold several real accounts there.
	linked := map[string]bool{}
	for rows.Next() {
		var name, acctNum string
		if err := rows.Scan(&name, &acctNum); err != nil {
			rows.Close()
			apiresponse.Error(w, err)
			return
		}
		linked[name+"|"+acctNum] = true
	}
	rows.Close()

	bankPool := discoverypool.BankPool
	// The "CONNECT MORE ACCOUNTS" picker checks banks first, then calls this
	// endpoint again with ?banks=Bank1,Bank2 to fetch just those banks'
	// candidate accounts rather than returning the entire inventory every
	// time — same discovery data, scoped down to what the user actually
	// asked to see.
	if banksParam := r.URL.Query().Get("banks"); banksParam != "" {
		requested := map[string]bool{}
		for _, name := range strings.Split(banksParam, ",") {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				requested[trimmed] = true
			}
		}
		filtered := bankPool[:0:0]
		for _, bankName := range bankPool {
			if requested[bankName] {
				filtered = append(filtered, bankName)
			}
		}
		bankPool = filtered
	}
	accounts := make([]BankAccountResponse, 0, len(bankPool)*discoverypool.AccountsPerBank)
	for _, bankName := range bankPool {
		for slot := 1; slot <= discoverypool.AccountsPerBank; slot++ {
			cand := discoverypool.Generate(userID, bankName, slot)
			if linked[cand.BankName+"|"+cand.AccountNumber] {
				continue
			}

			accounts = append(accounts, BankAccountResponse{
				ID:            cand.ID,
				BankName:      cand.BankName,
				AccountType:   cand.AccountType,
				Balance:       cand.Balance,
				CreatedAt:     time.Now(),
				IsLinked:      false,
				AccountNumber: cand.AccountNumber,
			})
		}
	}

	apiresponse.OK(w, map[string]any{
		"accounts": accounts,
		"count":    len(accounts),
	})
}

// aaWebhookProcessTimeout bounds the detached processing of an inbound AA
// notification (which itself makes an outbound IDBI call). The webhook is
// acked immediately; this only limits the background work.
const aaWebhookProcessTimeout = 30 * time.Second

// bankDependentSeeder creates demo data (FDs, mandates) tied to a bank
// account the user actually linked themselves, instead of that data being
// hardcoded against a fake bank account inserted at signup before any
// consent. Implemented by *repository.PostgresUserRepository.
type bankDependentSeeder interface {
	SeedBankDependentData(ctx context.Context, userID, bankAccountID uuid.UUID) error
}

type AAHandler struct {
	pool         *pgxpool.Pool
	aa           *idbiaa.Service       // nil unless IDBI_AA_ENABLED — then the consent flow is real
	idbiAccounts *idbiaccounts.Service // nil unless IDBI_ACCOUNTS_ENABLED — then GET /accounts serves real IDBI accounts
	events       *events.Publisher
	seeder       bankDependentSeeder // nil until WithSeeder is called
}

func NewAAHandler(pool *pgxpool.Pool) *AAHandler {
	return &AAHandler{pool: pool}
}

// WithEvents attaches the live-update publisher so adding or unlinking a
// bank account pushes an invalidation to the RM portal.
func (h *AAHandler) WithEvents(pub *events.Publisher) *AAHandler {
	h.events = pub
	return h
}

// WithIDBI attaches the real AA consent service (feature 4). When it is not
// attached, CreateConsent / GetAccountTransactions keep their original stub
// behaviour, so the app is unchanged with the flag off.
func (h *AAHandler) WithIDBI(svc *idbiaa.Service) *AAHandler {
	h.aa = svc
	return h
}

// WithIDBIAccounts folds the customer's real IDBI deposit accounts (feature 1)
// into GET /api/v1/aa/accounts, so the existing accounts screens show them with
// no frontend change. When the user has synced IDBI accounts they replace the
// seeded bank_accounts rows in the response; otherwise the bank_accounts rows
// are returned exactly as before.
func (h *AAHandler) WithIDBIAccounts(svc *idbiaccounts.Service) *AAHandler {
	h.idbiAccounts = svc
	return h
}

// IDBIEnabled reports whether the real AA flow is wired.
func (h *AAHandler) IDBIEnabled() bool { return h.aa != nil }

// WithSeeder attaches the demo-data seeder so a user's first successful
// AddAccount call (from discover -> approve, or the manual "connect more
// accounts" flow) can backfill their FD/mandate demo data against that real
// bank account. Without it, AddAccount behaves exactly as before (no demo
// FD/mandate seeding at all).
func (h *AAHandler) WithSeeder(s bankDependentSeeder) *AAHandler {
	h.seeder = s
	return h
}

type AddBankAccountRequest struct {
	BankName      string  `json:"bank_name"`
	AccountType   string  `json:"account_type"`
	Balance       float64 `json:"balance"`
	AccountNumber string  `json:"account_number"`
}

type BankAccountResponse struct {
	ID          uuid.UUID `json:"id"`
	BankName    string    `json:"bank_name"`
	AccountType string    `json:"account_type"`
	Balance     float64   `json:"balance"`
	CreatedAt   time.Time `json:"created_at"`
	// IsLinked distinguishes a real, already-persisted account (GetAccounts)
	// from a simulated AA-discovery candidate (DiscoverAccounts) that the
	// user still has to approve before it's ever written to bank_accounts.
	IsLinked bool `json:"is_linked"`
	// AccountNumber is only populated for IDBI-synced accounts (the real
	// number IDBI reports back, see idbiAccounts.List below) — manually
	// added accounts have no real account number anywhere in the system
	// (bank_accounts never collected one), so this is omitted for them and
	// the frontend falls back to a synthesized display value.
	AccountNumber string `json:"account_number,omitempty"`
}

func (h *AAHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/accounts", h.GetAccounts)
	r.Get("/accounts/discover", h.DiscoverAccounts)
	r.Get("/accounts/detected", h.DetectedAccounts)
	r.Get("/accounts/available-banks", h.AvailableBanks)
	r.Post("/accounts/connect", h.ConnectAccounts)
	r.Post("/accounts", h.AddAccount)
	r.Post("/accounts/link", h.AddAccount)
	r.Delete("/accounts/{accountID}", h.UnlinkAccount)
	r.Get("/accounts/{accountID}/transactions", h.GetAccountTransactions)
	r.Post("/consent", h.CreateConsent)

	// Real AA consent flow — only routed when IDBI_AA_ENABLED. Existing
	// routes above are untouched.
	if h.aa != nil {
		r.Get("/consents", h.ListConsents)
		r.Post("/consents/callback", h.ConsentCallback) // "live" mode redirect return (593)
		r.Get("/consents/{consentHandle}", h.GetConsent)
		r.Post("/consents/{consentHandle}/refresh", h.RefreshConsent)
		r.Post("/consents/{consentHandle}/fetch", h.FetchConsentData)
	}
	return r
}

// WebhookRoutes returns the inbound AA notification endpoints (497/498).
// These are called by IDBI's gateway, not the app, so they carry no user
// JWT and must be mounted in an unprotected group. Returns nil when the
// AA flow is not wired.
func (h *AAHandler) WebhookRoutes() chi.Router {
	if h.aa == nil {
		return nil
	}
	r := chi.NewRouter()
	r.Post("/consent-notification", h.consentNotification) // 497
	r.Post("/data-notification", h.dataNotification)       // 498
	return r
}

// connectedAccounts assembles the user's full set of already-linked
// accounts — IDBI-synced (feature 1) plus manually/demo-connected
// bank_accounts rows — in the one shape both GetAccounts and ConnectAccounts
// (which returns the post-connect list so the client never has to
// re-request it) need.
func (h *AAHandler) connectedAccounts(ctx context.Context, userID uuid.UUID) ([]BankAccountResponse, error) {
	// Feature 1: when the user has synced IDBI accounts, fold those in
	// alongside bank_accounts (below) rather than replacing them — a bank
	// added manually via the "connect more accounts" search (AddAccount,
	// which only ever writes to bank_accounts) must stay visible even after
	// the user also has real IDBI accounts synced, otherwise it silently
	// vanishes from this list the moment IDBI accounts exist.
	var accounts []BankAccountResponse
	if h.idbiAccounts != nil {
		if idbiAccs, ierr := h.idbiAccounts.List(ctx, userID); ierr == nil && len(idbiAccs) > 0 {
			for _, a := range idbiAccs {
				bal := a.LedgerBalance
				if bal == 0 {
					bal = a.AvailableBalance
				}
				name := "IDBI Bank"
				if a.BranchName != "" {
					name = "IDBI Bank — " + a.BranchName
				}
				accounts = append(accounts, BankAccountResponse{
					ID:            uuid.NewSHA1(idbiAcctNamespace, []byte(a.AccountNumber)),
					BankName:      name,
					AccountType:   a.AccountType,
					Balance:       bal,
					CreatedAt:     a.SyncedAt,
					IsLinked:      true,
					AccountNumber: a.AccountNumber,
				})
			}
		}
	}

	rows, err := h.pool.Query(ctx, `
		SELECT id, bank_name, account_type, balance, created_at, COALESCE(account_number, '')
		FROM bank_accounts
		WHERE user_id = $1 AND unlinked_at IS NULL
		ORDER BY created_at ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var acc BankAccountResponse
		if err := rows.Scan(&acc.ID, &acc.BankName, &acc.AccountType, &acc.Balance, &acc.CreatedAt, &acc.AccountNumber); err != nil {
			return nil, err
		}
		acc.IsLinked = true
		accounts = append(accounts, acc)
	}

	if accounts == nil {
		accounts = []BankAccountResponse{}
	}
	return accounts, nil
}

func (h *AAHandler) GetAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}

	accounts, err := h.connectedAccounts(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}

	apiresponse.OK(w, map[string]any{
		"accounts": accounts,
		"count":    len(accounts),
	})
}

// DetectedAccounts serves the fixed two-account "already detected" pair
// (see detectedBanks) the bank-connection demo flow shows on first entry,
// pre-checked and awaiting approval — distinct from DiscoverAccounts' full
// pool, which backs the separate "pick an additional bank" list. Accounts
// the user has already connected (by bank_name+account_number, same as
// DiscoverAccounts) are excluded so a repeat visit doesn't re-offer them.
func (h *AAHandler) DetectedAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}

	linked, err := h.linkedBankSet(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}

	bankPool := discoverypool.BankPool
	count := discoverypool.SeedAccountCount
	if count > len(bankPool) {
		count = len(bankPool)
	}
	initialBanks := bankPool[:count]

	accounts := make([]BankAccountResponse, 0, len(initialBanks))
	for _, bankName := range initialBanks {
		cand := discoverypool.Generate(userID, bankName, 1)
		if linked[cand.BankName+"|"+cand.AccountNumber] {
			continue
		}
		accounts = append(accounts, BankAccountResponse{
			ID:            cand.ID,
			BankName:      cand.BankName,
			AccountType:   cand.AccountType,
			Balance:       cand.Balance,
			CreatedAt:     time.Now(),
			IsLinked:      false,
			AccountNumber: cand.AccountNumber,
		})
	}

	apiresponse.OK(w, map[string]any{
		"accounts": accounts,
		"count":    len(accounts),
	})
}

// AvailableBanks lists the "add another bank" picker's candidates: every
// bank in the user's archetype pool except the two DetectedAccounts already
// covers and any bank the user has fully exhausted (every slot already
// connected). Grouped as bank names only — ConnectAccounts resolves the
// actual next free slot per bank at connect time, same as DiscoverAccounts
// already does for the legacy picker.
func (h *AAHandler) AvailableBanks(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}

	linkedBankNames, err := h.linkedBankNameSet(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	bankPool := discoverypool.BankPool
	count := discoverypool.SeedAccountCount
	if count > len(bankPool) {
		count = len(bankPool)
	}
	initialBanks := bankPool[:count]
	
	detected := map[string]bool{}
	for _, b := range initialBanks {
		detected[b] = true
	}

	banks := make([]string, 0, len(bankPool))
	for _, bankName := range bankPool {
		if linkedBankNames[bankName] >= discoverypool.AccountsPerBank {
			continue
		}
		if detected[bankName] && linkedBankNames[bankName] == 0 {
			// Skip showing it in "Available Banks" if it's already in DetectedAccounts and not fully linked yet
			// Actually, wait, let's keep the exact same logic as before:
			// "every bank in the user's archetype pool except the two DetectedAccounts already covers"
		}
		// Let's just exclude detected entirely like before:
		if detected[bankName] {
			continue
		}
		banks = append(banks, bankName)
	}

	apiresponse.OK(w, map[string]any{
		"banks": banks,
		"count": len(banks),
	})
}

// connectAccountsRequest is the combined "Approve & Proceed" / "Proceed"
// payload: the demo bank-connection flow lets a user re-approve some subset
// of the fixed DetectedAccounts candidates and/or add whole new banks in one
// round trip, instead of two separate calls.
type connectAccountsRequest struct {
	SelectedExistingAccounts []string `json:"selected_existing_accounts"`
	BanksToAdd                []string `json:"banks_to_add"`
}

// ConnectAccounts is the single endpoint behind both CTA labels ("Approve &
// Proceed" for detected-only, "Proceed" once an additional bank is picked):
// it persists whichever DetectedAccounts candidates the user kept checked,
// mock-generates and persists one account per requested new bank, and
// returns the full resulting connected-account list plus just the ones this
// call added (so the success screen can show "Axis Bank connected" without
// diffing the whole list client-side).
func (h *AAHandler) ConnectAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}

	var req connectAccountsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	if len(req.SelectedExistingAccounts) == 0 && len(req.BanksToAdd) == 0 {
		apiresponse.Error(w, apiresponse.Validation("select at least one bank account or bank to continue"))
		return
	}

	linked, err := h.linkedBankSet(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	linkedBankNames, err := h.linkedBankNameSet(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}

	var toInsert []discoverypool.Account

	// Resolve each requested "existing" account ID against the fixed
	// DetectedAccounts candidates — that's the only source of IDs this field
	// can legitimately reference, so an ID that doesn't match any of them
	// (stale, tampered, already connected) is silently skipped rather than
	// erroring the whole request.
	wanted := map[string]bool{}
	for _, id := range req.SelectedExistingAccounts {
		wanted[id] = true
	}
	
	bankPool := discoverypool.BankPool
	var allBanks []string
	allBanks = append(allBanks, bankPool...)

	for _, bankName := range allBanks {
		for slot := 1; slot <= discoverypool.AccountsPerBank; slot++ {
			cand := discoverypool.Generate(userID, bankName, slot)
			if !wanted[cand.ID.String()] {
				continue
			}
			if linked[cand.BankName+"|"+cand.AccountNumber] {
				continue
			}
			toInsert = append(toInsert, cand)
			linked[cand.BankName+"|"+cand.AccountNumber] = true
		}
	}

	// For each requested new bank, mock-generate its next free slot — same
	// deterministic generator DiscoverAccounts/DetectedAccounts use, so the
	// same user+bank always gets the same account number back rather than a
	// fresh one on every retry.
	for _, bankName := range req.BanksToAdd {
		slotUsed := linkedBankNames[bankName]
		for slot := 1; slot <= discoverypool.AccountsPerBank; slot++ {
			cand := discoverypool.Generate(userID, bankName, slot)
			if linked[cand.BankName+"|"+cand.AccountNumber] {
				continue
			}
			toInsert = append(toInsert, cand)
			linked[cand.BankName+"|"+cand.AccountNumber] = true
			linkedBankNames[bankName] = slotUsed + 1
			break
		}
	}

	if len(toInsert) == 0 {
		// Everything requested was already connected — not an error, just a
		// no-op the client can render the same as a fresh success.
		accounts, err := h.connectedAccounts(r.Context(), userID)
		if err != nil {
			apiresponse.Error(w, err)
			return
		}
		apiresponse.OK(w, map[string]any{
			"newly_added":        []BankAccountResponse{},
			"connected_accounts": accounts,
		})
		return
	}

	var needsSeeding bool
	if h.seeder != nil {
		var fdCount int
		if err := h.pool.QueryRow(r.Context(),
			`SELECT count(*) FROM fd_accounts WHERE user_id = $1`, userID,
		).Scan(&fdCount); err == nil {
			needsSeeding = fdCount == 0
		}
	}

	newlyAdded := make([]BankAccountResponse, 0, len(toInsert))
	var firstInsertedID uuid.UUID
	for _, cand := range toInsert {
		var acc BankAccountResponse
		err := h.pool.QueryRow(r.Context(), `
			INSERT INTO bank_accounts (user_id, bank_name, account_type, balance, account_number)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''))
			RETURNING id, bank_name, account_type, balance, created_at, COALESCE(account_number, '')
		`, userID, cand.BankName, cand.AccountType, cand.Balance, cand.AccountNumber).Scan(
			&acc.ID, &acc.BankName, &acc.AccountType, &acc.Balance, &acc.CreatedAt, &acc.AccountNumber,
		)
		if err != nil {
			apiresponse.Error(w, err)
			return
		}
		acc.IsLinked = true
		newlyAdded = append(newlyAdded, acc)
		if firstInsertedID == uuid.Nil {
			firstInsertedID = acc.ID
		}
	}

	if h.events != nil {
		go h.events.UserChanged(context.Background(), userID, events.TypeBankAccountChanged)
	}
	if h.seeder != nil && needsSeeding && firstInsertedID != uuid.Nil {
		if err := h.seeder.SeedBankDependentData(context.Background(), userID, firstInsertedID); err != nil {
			slog.Error("seed bank dependent data failed", "user_id", userID, "error", err)
		}
	}

	accounts, err := h.connectedAccounts(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}

	apiresponse.OK(w, map[string]any{
		"newly_added":        newlyAdded,
		"connected_accounts": accounts,
	})
}

// linkedBankSet returns the same "bank name|account number" membership set
// DiscoverAccounts uses, shared here so DetectedAccounts/ConnectAccounts
// exclude candidates the user already connected the same way.
func (h *AAHandler) linkedBankSet(ctx context.Context, userID uuid.UUID) (map[string]bool, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT bank_name, COALESCE(account_number, '') FROM bank_accounts WHERE user_id = $1 AND unlinked_at IS NULL
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	linked := map[string]bool{}
	for rows.Next() {
		var name, acctNum string
		if err := rows.Scan(&name, &acctNum); err != nil {
			return nil, err
		}
		linked[name+"|"+acctNum] = true
	}
	return linked, rows.Err()
}

// linkedBankNameSet counts how many slots are already connected per bank
// name, so AvailableBanks can hide a bank once every slot it offers is
// taken, and ConnectAccounts can pick the next free slot for a requested
// bank instead of colliding with one already connected.
func (h *AAHandler) linkedBankNameSet(ctx context.Context, userID uuid.UUID) (map[string]int, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT bank_name, count(*) FROM bank_accounts WHERE user_id = $1 AND unlinked_at IS NULL GROUP BY bank_name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		counts[name] = n
	}
	return counts, rows.Err()
}

func (h *AAHandler) AddAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}

	var req AddBankAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}

	if req.BankName == "" {
		req.BankName = "Bank Account"
	}
	if req.AccountType == "" {
		req.AccountType = "SAVINGS"
	}
	if req.Balance <= 0 {
		req.Balance = 25000.00
	}

	// Checked before the insert below so this always reflects "had zero
	// bank accounts before this call," not "has one now" (which would be
	// true after every single AddAccount, including the second, third, ...).
	var needsSeeding bool
	if h.seeder != nil {
		var fdCount int
		if err := h.pool.QueryRow(r.Context(),
			`SELECT count(*) FROM fd_accounts WHERE user_id = $1`, userID,
		).Scan(&fdCount); err == nil {
			needsSeeding = fdCount == 0
		}
	}

	var acc BankAccountResponse
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO bank_accounts (user_id, bank_name, account_type, balance, account_number)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
		RETURNING id, bank_name, account_type, balance, created_at, COALESCE(account_number, '')
	`, userID, req.BankName, req.AccountType, req.Balance, req.AccountNumber).Scan(
		&acc.ID,
		&acc.BankName,
		&acc.AccountType,
		&acc.Balance,
		&acc.CreatedAt,
		&acc.AccountNumber,
	)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	if h.events != nil {
		go h.events.UserChanged(context.Background(), userID, events.TypeBankAccountChanged)
	}
	// Best-effort, same pattern as the RM auto-assign on signup: this is
	// demo/mock enrichment (FDs, mandates), never something that should
	// fail or slow down the user's actual bank-linking request.
	if h.seeder != nil && needsSeeding {
		if err := h.seeder.SeedBankDependentData(context.Background(), userID, acc.ID); err != nil {
			slog.Error("seed bank dependent data failed", "user_id", userID, "error", err)
		}
	}

	apiresponse.Created(w, acc)
}

func (h *AAHandler) UnlinkAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}

	accountIDStr := chi.URLParam(r, "accountID")
	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid account ID: %v", err))
		return
	}

	// Soft-delete: bank_account_id is a NOT NULL, ON-DELETE-RESTRICT FK from
	// payments, mandates, and fd_accounts, so a hard DELETE here fails the
	// moment the account has any transaction history. Marking it unlinked
	// removes it from every "your accounts" / balance view while preserving
	// that history intact — the same behaviour a real bank's "close account"
	// flow has.
	tag, err := h.pool.Exec(r.Context(), `
		UPDATE bank_accounts
		SET unlinked_at = now()
		WHERE id = $1 AND user_id = $2 AND unlinked_at IS NULL
	`, accountID, userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		// Not a bank_accounts row — check whether it's one of the IDs
		// GetAccounts synthesizes for the user's IDBI-synced accounts
		// (uuid.NewSHA1(idbiAcctNamespace, accountNumber)). Those don't
		// exist in bank_accounts at all, so without this the "remove"
		// action on an IDBI account just 404'd and silently did nothing.
		// IDBI mirrors a whole customer's account list under one link —
		// there's no per-account revoke, so removing any one of them
		// revokes the whole IDBI sync for this user.
		if h.idbiAccounts != nil {
			idbiAccs, ierr := h.idbiAccounts.List(r.Context(), userID)
			if ierr == nil {
				for _, a := range idbiAccs {
					if uuid.NewSHA1(idbiAcctNamespace, []byte(a.AccountNumber)) == accountID {
						if rerr := h.idbiAccounts.Revoke(r.Context(), userID); rerr != nil {
							apiresponse.Error(w, rerr)
							return
						}
						if h.events != nil {
							go h.events.UserChanged(context.Background(), userID, events.TypeBankAccountChanged)
						}
						apiresponse.OK(w, map[string]string{
							"message": "IDBI bank connection revoked successfully",
						})
						return
					}
				}
			}
		}
		apiresponse.Error(w, apiresponse.NotFound("bank account not found"))
		return
	}
	if h.events != nil {
		go h.events.UserChanged(context.Background(), userID, events.TypeBankAccountChanged)
	}

	apiresponse.OK(w, map[string]string{
		"message": "account unlinked successfully",
	})
}

func (h *AAHandler) GetAccountTransactions(w http.ResponseWriter, r *http.Request) {
	apiresponse.OK(w, map[string]any{
		"transactions": []any{},
		"message":      "no transactions for this account",
	})
}

// consentCreateRequest is accepted by CreateConsent when the real AA flow is
// wired. All fields optional — mobile falls back to the user's registered
// phone.
type consentCreateRequest struct {
	Mobile    string `json:"mobile"`
	AccountID string `json:"account_id"`
}

func (h *AAHandler) CreateConsent(w http.ResponseWriter, r *http.Request) {
	// Stub behaviour — unchanged when IDBI_AA_ENABLED is off.
	if h.aa == nil {
		apiresponse.OK(w, map[string]any{
			"consent_id": "CONSENT-" + uuid.New().String()[:8],
			"status":     "ACTIVE",
			"message":    "Consent created successfully",
		})
		return
	}

	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req consentCreateRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // body is optional
	}
	mobile := req.Mobile
	if mobile == "" {
		mobile = h.userPhone(r, userID)
	}
	view, err := h.aa.RequestConsent(r.Context(), userID, mobile, req.AccountID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, view)
}

func (h *AAHandler) ListConsents(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	views, err := h.aa.List(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]any{"consents": views, "count": len(views)})
}

func (h *AAHandler) GetConsent(w http.ResponseWriter, r *http.Request) {
	h.refreshOrGet(w, r, false)
}

func (h *AAHandler) RefreshConsent(w http.ResponseWriter, r *http.Request) {
	h.refreshOrGet(w, r, true)
}

func (h *AAHandler) refreshOrGet(w http.ResponseWriter, r *http.Request, _ bool) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	handle := chi.URLParam(r, "consentHandle")
	view, err := h.aa.Refresh(r.Context(), userID, handle)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, view)
}

// consentCallbackRequest carries the encrypted blob the AA app hands back to
// our redirect URL in "live" mode.
type consentCallbackRequest struct {
	Ecres   string `json:"ecres"`
	Resdate string `json:"resdate"`
	Fi      string `json:"fi"`
}

func (h *AAHandler) ConsentCallback(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req consentCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	if req.Ecres == "" {
		apiresponse.Error(w, apiresponse.Validation("ecres is required"))
		return
	}
	view, err := h.aa.CompleteRedirect(r.Context(), userID, req.Ecres, req.Resdate, req.Fi)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, view)
}

func (h *AAHandler) FetchConsentData(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	handle := chi.URLParam(r, "consentHandle")
	n, err := h.aa.FetchStatements(r.Context(), userID, handle)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]any{"consent_handle": handle, "transactions_written": n})
}

func ackAA(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Success"})
}

// consentNotification handles inbound 497 pushConsentNotification. The AA
// spec wants a fast ack, and processing makes its own outbound IDBI call,
// so the work runs detached with its own timeout and the request is acked
// immediately. The handler is idempotent; failures are logged, and the app
// can always recover state via POST /consents/{handle}/refresh.
func (h *AAHandler) consentNotification(w http.ResponseWriter, r *http.Request) {
	var n idbi.PushConsentNotification
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		ackAA(w)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), aaWebhookProcessTimeout)
		defer cancel()
		if err := h.aa.HandleConsentNotification(ctx, n); err != nil {
			slog.Error("aa: 497 consent notification processing failed",
				"consent_handle", n.ConsentHandle, "event_status", n.EventStatus, "error", err)
		}
	}()
	ackAA(w)
}

// dataNotification handles inbound 498 pushDataNotification. Same fast-ack,
// detached-processing model as consentNotification.
func (h *AAHandler) dataNotification(w http.ResponseWriter, r *http.Request) {
	var n idbi.PushDataNotification
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		ackAA(w)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), aaWebhookProcessTimeout)
		defer cancel()
		if err := h.aa.HandleDataNotification(ctx, n); err != nil {
			slog.Error("aa: 498 data notification processing failed",
				"consent_handle", n.ConsentHandle, "event_status", n.EventStatus, "error", err)
		}
	}()
	ackAA(w)
}

// userPhone looks up the user's registered phone; "" if not found.
func (h *AAHandler) userPhone(r *http.Request, userID uuid.UUID) string {
	var phone string
	_ = h.pool.QueryRow(r.Context(), `SELECT COALESCE(phone_number, '') FROM users WHERE id = $1`, userID).Scan(&phone)
	return phone
}
