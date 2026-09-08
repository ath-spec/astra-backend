// Package idbiaccounts is IDBI integration feature 1: mirror a customer's IDBI
// deposit accounts and their balances into idbi_accounts, and serve them to the
// dashboard / accounts screens.
//
// Flow:  idbi_customer_link (cifId) --394--> account list
//
//	                            --365--> per-account detail (7 balance
//	                                     types, lien, holder, branch)
//	--idbimap--> []idbimap.Account --repo--> idbi_accounts --> screens
//
// The whole feature is gated by the caller: if it isn't wired (flag off) these
// methods are simply never invoked and the tables stay empty. Read-path results
// are cached in-process with a short TTL.
package idbiaccounts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/idbimap"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
	"github.com/yourusername/astra-backend/internal/repository"
)

// Provider is the subset of *idbi.Client this service needs. Narrowed to an
// interface so tests can supply a fake and so a mock IDBI can be dropped in.
type Provider interface {
	GetCustomerAccountsByCustID(ctx context.Context, req idbi.GetCustomerAccountsByCustIDRequest) (*idbi.GetCustomerAccountsByCustIDResponse, error)
	PerformAccountEnquiry(ctx context.Context, req idbi.PerformAccountEnquiryRequest) (*idbi.PerformAccountEnquiryResponse, error)
}

// Repo is the subset of *repository.IDBIRepository this service needs.
type Repo interface {
	GetCustomerLink(ctx context.Context, userID uuid.UUID) (repository.CustomerLink, error)
	UpsertCustomerLink(ctx context.Context, userID uuid.UUID, cifID, custID, source string) error
	ReplaceAccounts(ctx context.Context, userID uuid.UUID, accs []idbimap.Account) error
	ListAccounts(ctx context.Context, userID uuid.UUID) ([]repository.MirroredAccount, error)
}

// Config tunes the service. Zero values are fine.
type Config struct {
	// CacheTTL for the read path. Default 3m.
	CacheTTL time.Duration
	// StaleAfter: List() triggers a background refresh when the newest
	// synced_at is older than this. Default 30m. Zero disables auto-refresh.
	StaleAfter time.Duration
	// SkipPerAccountEnquiry: by default Refresh also calls 365 for each account
	// (richer detail: available/effective/lien balances, holder, branch). Set
	// this to true to stop at the lighter 394 data only.
	SkipPerAccountEnquiry bool
}

type Service struct {
	prov Provider
	repo Repo
	cfg  Config
	log  *slog.Logger

	cache *ttlCache
	// single-flight per user so concurrent List() calls don't stampede the
	// gateway on a cold cache.
	inflight sync.Map // userID -> *sync.Once wrapper
}

func New(prov Provider, repo Repo, cfg Config, log *slog.Logger) *Service {
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 3 * time.Minute
	}
	if cfg.StaleAfter == 0 {
		cfg.StaleAfter = 30 * time.Minute
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{prov: prov, repo: repo, cfg: cfg, log: log, cache: newTTLCache()}
}

// Account is the service's outward shape (no IDBI field names leak past here).
type Account struct {
	AccountNumber    string    `json:"account_number"`
	AccountType      string    `json:"account_type"`
	Currency         string    `json:"currency"`
	HolderName       string    `json:"holder_name,omitempty"`
	Status           string    `json:"status,omitempty"`
	BranchName       string    `json:"branch_name,omitempty"`
	LedgerBalance    float64   `json:"ledger_balance"`
	AvailableBalance float64   `json:"available_balance"`
	EffectiveBalance float64   `json:"effective_balance"`
	LienAmount       float64   `json:"lien_amount"`
	SyncedAt         time.Time `json:"synced_at"`
}

// SeedLink sets the user's IDBI identity directly. Used for demo users and
// wherever the customer-id roster gives us cifId/custId outright.
func (s *Service) SeedLink(ctx context.Context, userID uuid.UUID, cifID, custID string) error {
	if cifID == "" {
		return errors.New("idbiaccounts: SeedLink requires a cifId")
	}
	return s.repo.UpsertCustomerLink(ctx, userID, cifID, custID, "seed")
}

// ResolveLinkByAccount looks a bare account number up via 365 and stores the
// resulting custId against the user. NOTE: 365 does not return cifId, which
// 394 needs — so a link created this way still needs its cifId filled in
// (from the roster, 442, or SeedLink) before Refresh can list accounts.
func (s *Service) ResolveLinkByAccount(ctx context.Context, userID uuid.UUID, accountNumber string) (custID string, err error) {
	resp, err := s.prov.PerformAccountEnquiry(ctx, idbi.PerformAccountEnquiryRequest{AcctID: accountNumber})
	if err != nil {
		return "", fmt.Errorf("idbiaccounts: resolve by account %s: %w", accountNumber, err)
	}
	if resp.CustID == "" {
		return "", fmt.Errorf("idbiaccounts: 365 returned no custId for %s", accountNumber)
	}
	if err := s.repo.UpsertCustomerLink(ctx, userID, "", resp.CustID, "account_lookup"); err != nil {
		return "", err
	}
	return resp.CustID, nil
}

// Refresh pulls the customer's accounts from IDBI and replaces the local
// mirror. Safe to call repeatedly; it is the write path for both the sync
// worker and an explicit "refresh" action.
func (s *Service) Refresh(ctx context.Context, userID uuid.UUID) error {
	link, err := s.repo.GetCustomerLink(ctx, userID)
	if err != nil {
		return err // includes repository.ErrNoCustomerLink
	}
	if link.CifID == "" {
		return fmt.Errorf("idbiaccounts: user %s has a custId but no cifId — cannot call 394", userID)
	}

	var req idbi.GetCustomerAccountsByCustIDRequest
	req.Input.CifID = link.CifID
	req.Input.AcctType = "SBA"
	req.Txn = "E"

	listResp, err := s.prov.GetCustomerAccountsByCustID(ctx, req)
	if err != nil {
		return fmt.Errorf("idbiaccounts: 394 for cif %s: %w", link.CifID, err)
	}

	accs := idbimap.AccountsFromCustomerList(listResp)

	if !s.cfg.SkipPerAccountEnquiry {
		for i := range accs {
			enq, eerr := s.prov.PerformAccountEnquiry(ctx, idbi.PerformAccountEnquiryRequest{AcctID: accs[i].AccountNumber})
			if eerr != nil {
				// non-fatal: keep the lighter 394 data for this account
				s.log.Warn("idbiaccounts: 365 enrich failed",
					"account", accs[i].AccountNumber, "error", eerr)
				continue
			}
			accs[i] = idbimap.MergeAccount(accs[i], idbimap.AccountFromEnquiry(enq))
		}
	}

	if err := s.repo.ReplaceAccounts(ctx, userID, accs); err != nil {
		return err
	}
	// mirror changed — drop the cached read for this user.
	s.cache.delete(userID.String())
	s.log.Info("idbiaccounts: refreshed", "user", userID, "accounts", len(accs))
	return nil
}

// List returns the user's mirrored accounts. Served from cache when warm; from
// the DB otherwise. When the newest sync is older than cfg.StaleAfter it kicks
// a background Refresh (best-effort) but still returns what's on hand.
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]Account, error) {
	key := userID.String()
	if v, ok := s.cache.get(key); ok {
		return v.([]Account), nil
	}

	rows, err := s.repo.ListAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(rows))
	var newest time.Time
	for _, r := range rows {
		if r.SyncedAt.After(newest) {
			newest = r.SyncedAt
		}
		out = append(out, Account{
			AccountNumber:    r.AccountNumber,
			AccountType:      r.AccountType,
			Currency:         r.Currency,
			HolderName:       r.HolderName,
			Status:           r.Status,
			BranchName:       r.BranchName,
			LedgerBalance:    r.LedgerBalance,
			AvailableBalance: r.AvailableBalance,
			EffectiveBalance: r.EffectiveBalance,
			LienAmount:       r.LienAmount,
			SyncedAt:         r.SyncedAt,
		})
	}

	s.cache.set(key, out, s.cfg.CacheTTL)

	if s.cfg.StaleAfter > 0 && (newest.IsZero() || time.Since(newest) > s.cfg.StaleAfter) {
		s.refreshInBackground(userID)
	}
	return out, nil
}

func (s *Service) refreshInBackground(userID uuid.UUID) {
	if _, busy := s.inflight.LoadOrStore(userID, struct{}{}); busy {
		return
	}
	go func() {
		defer s.inflight.Delete(userID)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := s.Refresh(ctx, userID); err != nil && !errors.Is(err, repository.ErrNoCustomerLink) {
			s.log.Warn("idbiaccounts: background refresh failed", "user", userID, "error", err)
		}
	}()
}
