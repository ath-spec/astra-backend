// Package statementsync is IDBI integration feature 2: pull a linked customer's
// real transactions (393 getFullAccountStatement, and optionally AA 595) and
// upsert them into spend_transactions with source='idbi'/'aa'. The analytics
// engine and budget service then run on real data through the same pipe as the
// seeded MockSource.
//
// Gated by IDBI_SPEND_ENABLED: with it on, main wires PgSource (reads
// spend_transactions as-is) instead of MockSource, and this service is what
// fills the table — on a schedule and on demand.
package statementsync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/idbimap"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
	"github.com/yourusername/astra-backend/internal/repository"
)

// Provider is the subset of *idbi.Client this service needs.
type Provider interface {
	GetFullAccountStatement(ctx context.Context, req idbi.FullStatementRequest) (*idbi.FullStatementResponse, error)
}

// Repo is the subset of *repository.IDBIRepository this service needs.
type Repo interface {
	GetCustomerLink(ctx context.Context, userID uuid.UUID) (repository.CustomerLink, error)
	ListAccounts(ctx context.Context, userID uuid.UUID) ([]repository.MirroredAccount, error)
	UpsertSpendTransactions(ctx context.Context, userID uuid.UUID, source string, rows []idbimap.SpendRow) (int, error)
	LatestSpendSync(ctx context.Context, userID uuid.UUID, source string) (time.Time, error)
}

const sourceIDBI = "idbi"

// Config tunes the sync.
type Config struct {
	// Lookback is how far back to pull on a first sync (no prior rows).
	// Default 180 days — matches the analytics engine's seed window.
	Lookback time.Duration
	// Overlap re-pulls this much before the last synced txn each run, to catch
	// late-posted entries. Default 3 days.
	Overlap time.Duration
	// MaxPages caps pagination per account per run. Default 20.
	MaxPages int
}

type Service struct {
	prov Provider
	repo Repo
	cfg  Config
	log  *slog.Logger
}

func New(prov Provider, repo Repo, cfg Config, log *slog.Logger) *Service {
	if cfg.Lookback == 0 {
		cfg.Lookback = 180 * 24 * time.Hour
	}
	if cfg.Overlap == 0 {
		cfg.Overlap = 3 * 24 * time.Hour
	}
	if cfg.MaxPages == 0 {
		cfg.MaxPages = 20
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{prov: prov, repo: repo, cfg: cfg, log: log}
}

// SyncUser pulls statements for every mirrored IDBI account the user has and
// upserts the transactions. Returns the number of rows written. Requires the
// user to be linked (idbi_customer_link) and to have accounts mirrored
// (idbi_accounts) — run idbiaccounts.Refresh first.
func (s *Service) SyncUser(ctx context.Context, userID uuid.UUID) (int, error) {
	if _, err := s.repo.GetCustomerLink(ctx, userID); err != nil {
		return 0, err // repository.ErrNoCustomerLink
	}
	accts, err := s.repo.ListAccounts(ctx, userID)
	if err != nil {
		return 0, err
	}
	if len(accts) == 0 {
		return 0, fmt.Errorf("statementsync: no mirrored accounts for user %s (run account refresh first)", userID)
	}

	last, err := s.repo.LatestSpendSync(ctx, userID, sourceIDBI)
	if err != nil {
		return 0, err
	}
	from := time.Now().Add(-s.cfg.Lookback)
	if !last.IsZero() {
		from = last.Add(-s.cfg.Overlap)
	}
	to := time.Now()

	total := 0
	for _, a := range accts {
		n, aerr := s.syncAccount(ctx, userID, a, from, to)
		if aerr != nil {
			s.log.Warn("statementsync: account failed",
				"user", userID, "account", a.AccountNumber, "error", aerr)
			continue
		}
		total += n
	}
	s.log.Info("statementsync: done", "user", userID, "accounts", len(accts), "rows", total)
	return total, nil
}

func (s *Service) syncAccount(ctx context.Context, userID uuid.UUID, a repository.MirroredAccount, from, to time.Time) (int, error) {
	var req idbi.FullStatementRequest
	req.Input.Acid = a.AccountNumber
	req.Input.BranchID = a.BranchID
	req.Input.FromDate = from.Format("2006-01-02T15:04:05.000")
	req.Input.ToDate = to.Format("2006-01-02T15:04:05.000")
	req.Input.SortIn = "D"

	written := 0
	for page := 0; page < s.cfg.MaxPages; page++ {
		resp, err := s.prov.GetFullAccountStatement(ctx, req)
		if err != nil {
			return written, fmt.Errorf("393 %s page %d: %w", a.AccountNumber, page, err)
		}
		rows := idbimap.FullStatementSpendRows(a.AccountNumber, resp)
		n, uerr := s.repo.UpsertSpendTransactions(ctx, userID, sourceIDBI, rows)
		if uerr != nil {
			return written, uerr
		}
		written += n

		if resp.Result.HasMoreData != "Y" || len(resp.Result.TransactionDetails) == 0 {
			break
		}
		last := resp.Result.TransactionDetails[len(resp.Result.TransactionDetails)-1]
		req.Input.PaginationDetails.LastTxnID = last.TxnID
		req.Input.PaginationDetails.LastTxnSrlNo = last.TxnSrlNo
		req.Input.PaginationDetails.LastPstdDate = last.PstdDate
		req.Input.PaginationDetails.LastTxnDate = last.ValueDate
		req.Input.PaginationDetails.LastBalance = last.TxnBalance
	}
	return written, nil
}
