// Package idbiaa is IDBI integration feature 4: Account Aggregator (AA)
// account linking. It replaces the aa_handler.go stub (which returned a
// fake "CONSENT-xxxx" and no data) with the real 590/591/592 consent flow
// plus 497/498 inbound webhooks and 595 statement pulls.
//
// The consent redirect (592) returns a dummy URL in the sandbox, so the
// redirect step is gated by Config.RedirectMode:
//
//	"stub" (default) — skip the browser redirect; confirm the consent
//	                    straight from 591 getConsentList (the sandbox
//	                    returns it ACTIVE with linked accounts).
//	"live"           — return the 592 URL to the client and wait for the
//	                    497 CONSENT_APPROVED webhook to flip status.
//
// Statement transactions (595) are written into spend_transactions with
// source='aa', so the existing analytics engine consumes them unchanged.
// Gated by IDBI_AA_ENABLED.
package idbiaa

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/idbimap"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
	"github.com/yourusername/astra-backend/internal/repository"
)

// Provider is the subset of *idbi.Client this service needs.
type Provider interface {
	RequestConsent(ctx context.Context, req idbi.RequestConsentRequest) (*idbi.RequestConsentResponse, error)
	GetConsentList(ctx context.Context, req idbi.GetConsentListRequest) (*idbi.GetConsentListResponse, error)
	GetWebRedirectionURL(ctx context.Context, req idbi.GetWebRedirectionURLRequest) (*idbi.GetWebRedirectionURLResponse, error)
	GetAAStatement(ctx context.Context, req idbi.GetAAStatementRequest) (*idbi.GetAAStatementResponse, error)
}

// Repo is the AA mirror-table repository.
type Repo interface {
	CreateConsent(ctx context.Context, row repository.AAConsentRow) error
	UpdateConsentStatus(ctx context.Context, handle, status, consentID string, approvedAt *time.Time) error
	MarkFetched(ctx context.Context, handle string) error
	GetConsent(ctx context.Context, handle string) (repository.AAConsentRow, bool, error)
	ListConsents(ctx context.Context, userID uuid.UUID) ([]repository.AAConsentRow, error)
	ReplaceLinkedAccounts(ctx context.Context, handle string, accs []repository.AALinkedAccountRow) error
	ListLinkedAccounts(ctx context.Context, handle string) ([]repository.AALinkedAccountRow, error)
}

// SpendRepo lands AA statement rows into spend_transactions.
type SpendRepo interface {
	UpsertSpendTransactions(ctx context.Context, userID uuid.UUID, source string, rows []idbimap.SpendRow) (int, error)
}

// SourceAA is the spend_transactions.source tag for AA-sourced rows.
const SourceAA = "aa"

// Config carries the AA caller identity and the redirect switch. All values
// are supplied by the caller (from config.Load / IDBI_AA_* env vars) — this
// package holds no fallback literals of its own.
type Config struct {
	RedirectMode string // "stub" | "live"
	VUASuffix    string // appended to the mobile to form the VUA, e.g. "@onemoney"
	ProductID    string // AA productID, e.g. "TEST"
	CallbackURL  string // redirectUrl handed to 592 (only used in "live" mode)
}

type Service struct {
	prov  Provider
	repo  Repo
	spend SpendRepo
	cfg   Config
	log   *slog.Logger
}

func New(prov Provider, repo Repo, spend SpendRepo, cfg Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{prov: prov, repo: repo, spend: spend, cfg: cfg, log: log}
}

// stubMode reports whether the consent redirect step is skipped. Anything
// other than an explicit "live" is treated as stub — the safe default.
func (s *Service) stubMode() bool { return !strings.EqualFold(s.cfg.RedirectMode, "live") }

// LinkedAccount is the outward shape of one AA-linked account.
type LinkedAccount struct {
	LinkRefNumber       string `json:"link_ref_number"`
	FipID               string `json:"fip_id,omitempty"`
	FipName             string `json:"fip_name,omitempty"`
	AccountType         string `json:"account_type,omitempty"`
	FiType              string `json:"fi_type,omitempty"`
	MaskedAccountNumber string `json:"masked_account_number,omitempty"`
}

// ConsentView is the outward shape of a consent.
type ConsentView struct {
	ConsentHandle  string          `json:"consent_handle"`
	ConsentID      string          `json:"consent_id,omitempty"`
	Status         string          `json:"status"`
	VUA            string          `json:"vua,omitempty"`
	AccountID      string          `json:"account_id,omitempty"`
	RedirectURL    string          `json:"redirect_url,omitempty"` // set in "live" mode — open this in a browser
	LinkedAccounts []LinkedAccount `json:"linked_accounts"`
	ApprovedAt     *time.Time      `json:"approved_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// RequestConsent starts an AA link: 590 requestConsent, then either return
// the 592 redirect URL ("live") or confirm straight from 591 ("stub").
func (s *Service) RequestConsent(ctx context.Context, userID uuid.UUID, mobile, accountID string) (ConsentView, error) {
	mobile = strings.TrimSpace(mobile)
	if mobile == "" {
		return ConsentView{}, fmt.Errorf("idbiaa: mobile is required to request consent")
	}
	vua := mobile + s.cfg.VUASuffix

	resp, err := s.prov.RequestConsent(ctx, idbi.RequestConsentRequest{
		PartyIdentifierType:  "MOBILE",
		PartyIdentifierValue: mobile,
		ProductID:            s.cfg.ProductID,
		AccountID:            accountID,
		Vua:                  vua,
		TransactionID:        numericID(),
	})
	if err != nil {
		return ConsentView{}, fmt.Errorf("idbiaa: 590 requestConsent: %w", err)
	}
	handle := resp.Data.ConsentHandle
	if handle == "" {
		return ConsentView{}, fmt.Errorf("idbiaa: 590 returned no consent handle")
	}
	status := firstNonEmpty(resp.Data.Status, "PENDING")

	row := repository.AAConsentRow{
		ConsentHandle: handle,
		UserID:        userID,
		Status:        status,
		PartyIDType:   "MOBILE",
		PartyIDValue:  mobile,
		VUA:           vua,
		ProductID:     s.cfg.ProductID,
		AccountID:     accountID,
	}

	if !s.stubMode() {
		red, rerr := s.prov.GetWebRedirectionURL(ctx, idbi.GetWebRedirectionURLRequest{
			ConsentHandle: handle,
			RedirectURL:   s.cfg.CallbackURL,
		})
		if rerr != nil {
			return ConsentView{}, fmt.Errorf("idbiaa: 592 getWebRedirectionURL: %w", rerr)
		}
		if len(red.Data) > 0 {
			row.WebRedirectURL = red.Data[0].WebRedirectionURL
		}
		row.RedirectURL = s.cfg.CallbackURL
		if err := s.repo.CreateConsent(ctx, row); err != nil {
			return ConsentView{}, err
		}
		s.log.Info("idbiaa: consent requested (live redirect)", "handle", handle, "user", userID)
		return ConsentView{
			ConsentHandle: handle,
			Status:        status,
			VUA:           vua,
			AccountID:     accountID,
			RedirectURL:   row.WebRedirectURL,
			CreatedAt:     time.Now().UTC(),
		}, nil
	}

	// stub mode — persist, then confirm from 591. A 591 hiccup must not fail
	// the whole request: the consent (590) is already created and stored, so
	// fall back to the stored PENDING view and let the caller retry via
	// POST /consents/{handle}/refresh.
	if err := s.repo.CreateConsent(ctx, row); err != nil {
		return ConsentView{}, err
	}
	s.log.Info("idbiaa: consent requested (stub mode — confirming from 591)", "handle", handle, "user", userID)
	view, err := s.Refresh(ctx, userID, handle)
	if err != nil {
		s.log.Warn("idbiaa: stub-mode 591 confirm failed, returning pending view", "handle", handle, "error", err)
		stored, ok, _ := s.repo.GetConsent(ctx, handle)
		if !ok {
			return ConsentView{}, err
		}
		return viewOf(stored, nil), nil
	}
	return view, nil
}

// List returns every consent for a user with its linked accounts.
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]ConsentView, error) {
	rows, err := s.repo.ListConsents(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ConsentView, 0, len(rows))
	for _, r := range rows {
		linked, err := s.repo.ListLinkedAccounts(ctx, r.ConsentHandle)
		if err != nil {
			return nil, err
		}
		out = append(out, viewOf(r, linked))
	}
	return out, nil
}

// Refresh re-reads a consent from 591 getConsentList and updates status +
// linked accounts. Safe to call at any time.
func (s *Service) Refresh(ctx context.Context, userID uuid.UUID, handle string) (ConsentView, error) {
	row, ok, err := s.repo.GetConsent(ctx, handle)
	if err != nil {
		return ConsentView{}, err
	}
	if !ok {
		return ConsentView{}, repository.ErrNoConsent
	}
	if row.UserID != userID {
		return ConsentView{}, repository.ErrNoConsent
	}

	list, err := s.prov.GetConsentList(ctx, idbi.GetConsentListRequest{
		PartyIdentifierType:  row.PartyIDType,
		PartyIdentifierValue: row.PartyIDValue,
		ProductID:            firstNonEmpty(row.ProductID, s.cfg.ProductID),
		AccountID:            row.AccountID,
		Vua:                  row.VUA,
	})
	if err != nil {
		return ConsentView{}, fmt.Errorf("idbiaa: 591 getConsentList: %w", err)
	}

	match := pickConsent(list, handle)
	if match == nil {
		// Nothing came back for this handle — return what we have stored.
		linked, _ := s.repo.ListLinkedAccounts(ctx, handle)
		return viewOf(row, linked), nil
	}

	status := strings.ToUpper(firstNonEmpty(match.Status, row.Status))
	var approvedAt *time.Time
	if status == "ACTIVE" {
		if t := idbimap.MustParseTime(match.ConsentCreationData); !t.IsZero() {
			approvedAt = &t
		} else {
			now := time.Now().UTC()
			approvedAt = &now
		}
	}
	if err := s.repo.UpdateConsentStatus(ctx, handle, status, match.ConsentID, approvedAt); err != nil {
		return ConsentView{}, err
	}

	linkedRows := make([]repository.AALinkedAccountRow, 0, len(match.Accounts))
	for _, a := range match.Accounts {
		linkedRows = append(linkedRows, repository.AALinkedAccountRow{
			ConsentHandle:       handle,
			LinkRefNumber:       a.LinkReferenceNumber,
			FipID:               a.FipID,
			FipName:             a.FipName,
			AccountType:         a.AccountType,
			FiType:              a.FiType,
			MaskedAccountNumber: a.MaskedAccountNumber,
		})
	}
	if err := s.repo.ReplaceLinkedAccounts(ctx, handle, linkedRows); err != nil {
		return ConsentView{}, err
	}

	row, _, _ = s.repo.GetConsent(ctx, handle)
	linked, _ := s.repo.ListLinkedAccounts(ctx, handle)
	return viewOf(row, linked), nil
}

// HandleConsentNotification processes an inbound 497 pushConsentNotification.
// It is best-effort: an unknown handle is ignored (logged), never an error,
// so the webhook can always ack 200.
func (s *Service) HandleConsentNotification(ctx context.Context, n idbi.PushConsentNotification) error {
	if n.ConsentHandle == "" {
		return nil
	}
	row, ok, err := s.repo.GetConsent(ctx, n.ConsentHandle)
	if err != nil {
		return err
	}
	if !ok {
		s.log.Warn("idbiaa: 497 for unknown consent handle", "handle", n.ConsentHandle, "status", n.EventStatus)
		return nil
	}

	status := consentStatusFromEvent(n.EventStatus)
	var approvedAt *time.Time
	if status == "ACTIVE" {
		now := time.Now().UTC()
		approvedAt = &now
	}
	if err := s.repo.UpdateConsentStatus(ctx, n.ConsentHandle, status, n.ConsentID, approvedAt); err != nil {
		return err
	}
	s.log.Info("idbiaa: consent notification applied", "handle", n.ConsentHandle, "status", status)

	if status == "ACTIVE" {
		// Pull the linked-account list now that it is approved.
		if _, rerr := s.Refresh(ctx, row.UserID, n.ConsentHandle); rerr != nil {
			s.log.Warn("idbiaa: post-approval refresh failed", "handle", n.ConsentHandle, "error", rerr)
		}
	}
	return nil
}

// HandleDataNotification processes an inbound 498 pushDataNotification. On
// DATA_READY it pulls statements (595) for the consent. Best-effort.
func (s *Service) HandleDataNotification(ctx context.Context, n idbi.PushDataNotification) error {
	if n.ConsentHandle == "" {
		return nil
	}
	if !strings.EqualFold(n.EventStatus, "DATA_READY") {
		s.log.Info("idbiaa: 498 non-ready event ignored", "handle", n.ConsentHandle, "status", n.EventStatus)
		return nil
	}
	row, ok, err := s.repo.GetConsent(ctx, n.ConsentHandle)
	if err != nil {
		return err
	}
	if !ok {
		s.log.Warn("idbiaa: 498 for unknown consent handle", "handle", n.ConsentHandle)
		return nil
	}
	written, ferr := s.FetchStatements(ctx, row.UserID, n.ConsentHandle)
	if ferr != nil {
		s.log.Warn("idbiaa: data fetch failed", "handle", n.ConsentHandle, "error", ferr)
		return nil
	}
	s.log.Info("idbiaa: data fetched on DATA_READY", "handle", n.ConsentHandle, "rows", written)
	return nil
}

// FetchStatements pulls 595 for every linked account under a consent and
// lands the transactions into spend_transactions with source='aa'. Returns
// the number of rows written. Callable on demand (POST /consent/{h}/fetch).
func (s *Service) FetchStatements(ctx context.Context, userID uuid.UUID, handle string) (int, error) {
	row, ok, err := s.repo.GetConsent(ctx, handle)
	if err != nil {
		return 0, err
	}
	if !ok || row.UserID != userID {
		return 0, repository.ErrNoConsent
	}
	linked, err := s.repo.ListLinkedAccounts(ctx, handle)
	if err != nil {
		return 0, err
	}
	refs := make([]string, 0, len(linked))
	for _, l := range linked {
		if l.LinkRefNumber != "" {
			refs = append(refs, l.LinkRefNumber)
		}
	}
	if len(refs) == 0 {
		return 0, fmt.Errorf("idbiaa: consent %s has no linked accounts to fetch", handle)
	}
	if row.ConsentID == "" {
		return 0, fmt.Errorf("idbiaa: consent %s has no consent_id yet — refresh it first", handle)
	}

	stmt, err := s.prov.GetAAStatement(ctx, idbi.GetAAStatementRequest{
		ConsentID:     row.ConsentID,
		LinkRefNumber: refs,
	})
	if err != nil {
		return 0, fmt.Errorf("idbiaa: 595 getAccountStatement: %w", err)
	}

	rows := idbimap.AAStatementAllSpendRows(stmt)
	n := 0
	if s.spend != nil && len(rows) > 0 {
		n, err = s.spend.UpsertSpendTransactions(ctx, userID, SourceAA, rows)
		if err != nil {
			return 0, err
		}
	}
	if err := s.repo.MarkFetched(ctx, handle); err != nil {
		s.log.Warn("idbiaa: mark fetched failed", "handle", handle, "error", err)
	}
	return n, nil
}

// --- helpers ---------------------------------------------------------------

func viewOf(r repository.AAConsentRow, linked []repository.AALinkedAccountRow) ConsentView {
	v := ConsentView{
		ConsentHandle:  r.ConsentHandle,
		ConsentID:      r.ConsentID,
		Status:         r.Status,
		VUA:            r.VUA,
		AccountID:      r.AccountID,
		RedirectURL:    r.WebRedirectURL,
		ApprovedAt:     r.ApprovedAt,
		CreatedAt:      r.CreatedAt,
		LinkedAccounts: make([]LinkedAccount, 0, len(linked)),
	}
	for _, l := range linked {
		v.LinkedAccounts = append(v.LinkedAccounts, LinkedAccount{
			LinkRefNumber:       l.LinkRefNumber,
			FipID:               l.FipID,
			FipName:             l.FipName,
			AccountType:         l.AccountType,
			FiType:              l.FiType,
			MaskedAccountNumber: l.MaskedAccountNumber,
		})
	}
	return v
}

func pickConsent(list *idbi.GetConsentListResponse, handle string) *idbi.Consent {
	if list == nil {
		return nil
	}
	for i := range list.Data {
		if list.Data[i].ConsentHandle == handle {
			return &list.Data[i]
		}
	}
	// Sandbox sometimes returns a single consent without echoing the handle.
	if len(list.Data) == 1 && list.Data[0].ConsentHandle == "" {
		return &list.Data[0]
	}
	return nil
}

func consentStatusFromEvent(eventStatus string) string {
	switch strings.ToUpper(strings.TrimSpace(eventStatus)) {
	case "CONSENT_APPROVED", "ACTIVE", "APPROVED":
		return "ACTIVE"
	case "CONSENT_REJECTED", "REJECTED", "DENIED":
		return "REJECTED"
	case "CONSENT_REVOKED", "REVOKED":
		return "REVOKED"
	case "CONSENT_EXPIRED", "EXPIRED":
		return "EXPIRED"
	case "CONSENT_PAUSED", "PAUSED":
		return "PAUSED"
	default:
		return "PENDING"
	}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func numericID() string {
	n, err := rand.Int(rand.Reader, big.NewInt(9_000_000))
	if err != nil {
		return "1000000"
	}
	return fmt.Sprintf("%d", n.Int64()+1_000_000)
}
