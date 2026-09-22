package idbiaa

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/idbimap"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
	"github.com/yourusername/astra-backend/internal/repository"
)

// ---- fakes --------------------------------------------------------------

type fakeProvider struct {
	consentHandle string
	consentStatus string
	list          *idbi.GetConsentListResponse
	redirect      *idbi.GetWebRedirectionURLResponse
	stmt          *idbi.GetAAStatementResponse
	stmtFinPro    *idbi.GetAAStatementResponse
	decrypted     *idbi.GenerateDecryptedResponse

	reqConsentCalls int
	listCalls       int
	redirectCalls   int
	stmtCalls       int
	stmtFinProCalls int
	decryptedCalls  int
}

func (f *fakeProvider) RequestConsent(_ context.Context, _ idbi.RequestConsentRequest) (*idbi.RequestConsentResponse, error) {
	f.reqConsentCalls++
	r := &idbi.RequestConsentResponse{}
	r.Data.Status = f.consentStatus
	r.Data.ConsentHandle = f.consentHandle
	return r, nil
}
func (f *fakeProvider) GetConsentList(_ context.Context, _ idbi.GetConsentListRequest) (*idbi.GetConsentListResponse, error) {
	f.listCalls++
	return f.list, nil
}
func (f *fakeProvider) GetWebRedirectionURL(_ context.Context, _ idbi.GetWebRedirectionURLRequest) (*idbi.GetWebRedirectionURLResponse, error) {
	f.redirectCalls++
	return f.redirect, nil
}
func (f *fakeProvider) GetAAStatement(_ context.Context, _ idbi.GetAAStatementRequest) (*idbi.GetAAStatementResponse, error) {
	f.stmtCalls++
	return f.stmt, nil
}
func (f *fakeProvider) GetAAStatementFromFinPro(_ context.Context, _ idbi.GetAAStatementRequest) (*idbi.GetAAStatementResponse, error) {
	f.stmtFinProCalls++
	if f.stmtFinPro != nil {
		return f.stmtFinPro, nil
	}
	return f.stmt, nil
}
func (f *fakeProvider) GenerateDecryptedResponse(_ context.Context, _ idbi.GenerateDecryptedResponseRequest) (*idbi.GenerateDecryptedResponse, error) {
	f.decryptedCalls++
	return f.decrypted, nil
}

type fakeRepo struct {
	consents map[string]repository.AAConsentRow
	linked   map[string][]repository.AALinkedAccountRow
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		consents: map[string]repository.AAConsentRow{},
		linked:   map[string][]repository.AALinkedAccountRow{},
	}
}

func (r *fakeRepo) CreateConsent(_ context.Context, row repository.AAConsentRow) error {
	if existing, ok := r.consents[row.ConsentHandle]; ok {
		if row.ConsentID == "" {
			row.ConsentID = existing.ConsentID
		}
		row.CreatedAt = existing.CreatedAt
	} else {
		row.CreatedAt = time.Now().UTC()
	}
	row.UpdatedAt = time.Now().UTC()
	r.consents[row.ConsentHandle] = row
	return nil
}
func (r *fakeRepo) UpdateConsentStatus(_ context.Context, handle, status, consentID string, approvedAt *time.Time) error {
	row, ok := r.consents[handle]
	if !ok {
		return repository.ErrNoConsent
	}
	row.Status = status
	if consentID != "" {
		row.ConsentID = consentID
	}
	if approvedAt != nil {
		row.ApprovedAt = approvedAt
	}
	r.consents[handle] = row
	return nil
}
func (r *fakeRepo) MarkFetched(_ context.Context, handle string) error {
	row, ok := r.consents[handle]
	if !ok {
		return repository.ErrNoConsent
	}
	now := time.Now().UTC()
	row.LastFetchedAt = &now
	r.consents[handle] = row
	return nil
}
func (r *fakeRepo) GetConsent(_ context.Context, handle string) (repository.AAConsentRow, bool, error) {
	row, ok := r.consents[handle]
	return row, ok, nil
}
func (r *fakeRepo) ListConsents(_ context.Context, userID uuid.UUID) ([]repository.AAConsentRow, error) {
	var out []repository.AAConsentRow
	for _, row := range r.consents {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}
func (r *fakeRepo) ReplaceLinkedAccounts(_ context.Context, handle string, accs []repository.AALinkedAccountRow) error {
	r.linked[handle] = accs
	return nil
}
func (r *fakeRepo) ListLinkedAccounts(_ context.Context, handle string) ([]repository.AALinkedAccountRow, error) {
	return r.linked[handle], nil
}

type fakeSpend struct {
	rows   []idbimap.SpendRow
	source string
}

func (s *fakeSpend) UpsertSpendTransactions(_ context.Context, _ uuid.UUID, source string, rows []idbimap.SpendRow) (int, error) {
	s.source = source
	s.rows = append(s.rows, rows...)
	return len(rows), nil
}

// ---- fixtures ---------------------------------------------------------------

const handle = "6afdd734-be3b-475f-a8fc-3fcd18c04714"

func activeList() *idbi.GetConsentListResponse {
	return &idbi.GetConsentListResponse{
		Status: "SUCCESS",
		Data: []idbi.Consent{{
			ConsentID:     "CONSENT-0001",
			Status:        "ACTIVE",
			ConsentHandle: handle,
			Accounts: []idbi.ConsentLinkedAccount{{
				FipName:             "IDBI Bank",
				FipID:               "IDBI001",
				AccountType:         "SAVINGS",
				LinkReferenceNumber: "LRN0001",
				MaskedAccountNumber: "XXXXXXXX0003",
				FiType:              "DEPOSIT",
			}},
		}},
	}
}

func stmtResp() *idbi.GetAAStatementResponse {
	r := &idbi.GetAAStatementResponse{Status: "Success"}
	acc := idbi.AAStatementAccount{LinkReferenceNumber: "LRN0001"}
	acc.Transactions.Transaction = []idbi.AAStatementTxn{
		{TxnID: "TXN0001", Type: "DEBIT", Mode: "UPI", Amount: "1200.50", Narration: "SWIGGY", TransactionTimestamp: "2025-05-10T10:00:00.000Z"},
		{TxnID: "TXN0002", Type: "CREDIT", Mode: "NEFT", Amount: "50000.00", Narration: "SALARY", TransactionTimestamp: "2025-05-01T09:00:00.000Z"},
	}
	r.Data = []idbi.AAStatementAccount{acc}
	return r
}

// ---- tests ----------------------------------------------------------------

func TestRequestConsent_StubMode_ConfirmsFrom591(t *testing.T) {
	prov := &fakeProvider{consentHandle: handle, consentStatus: "PENDING", list: activeList()}
	repo := newFakeRepo()
	s := New(prov, repo, &fakeSpend{}, Config{RedirectMode: "stub"}, nil)

	uid := uuid.New()
	view, err := s.RequestConsent(context.Background(), uid, "9988776655", "660100100003")
	if err != nil {
		t.Fatalf("RequestConsent: %v", err)
	}
	if prov.redirectCalls != 0 {
		t.Errorf("stub mode must not call 592, got %d calls", prov.redirectCalls)
	}
	if view.RedirectURL != "" {
		t.Errorf("stub mode must not surface a redirect URL, got %q", view.RedirectURL)
	}
	if view.Status != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE (confirmed from 591)", view.Status)
	}
	if view.ConsentID != "CONSENT-0001" {
		t.Errorf("consent id = %q", view.ConsentID)
	}
	if len(view.LinkedAccounts) != 1 || view.LinkedAccounts[0].LinkRefNumber != "LRN0001" {
		t.Errorf("linked accounts = %+v", view.LinkedAccounts)
	}
	if got := repo.consents[handle]; got.Status != "ACTIVE" || got.UserID != uid {
		t.Errorf("stored consent = %+v", got)
	}
}

func TestRequestConsent_LiveMode_ReturnsRedirectAndStaysPending(t *testing.T) {
	prov := &fakeProvider{
		consentHandle: handle, consentStatus: "PENDING",
		redirect: &idbi.GetWebRedirectionURLResponse{
			Status: "Success",
			Data: []struct {
				WebRedirectionURL string `json:"webRedirectionUrl"`
			}{{WebRedirectionURL: "https://webrd.onemoney.in/uat/v2/?ecreq=xxx"}},
		},
	}
	repo := newFakeRepo()
	s := New(prov, repo, &fakeSpend{}, Config{RedirectMode: "live"}, nil)

	view, err := s.RequestConsent(context.Background(), uuid.New(), "9988776655", "660100100003")
	if err != nil {
		t.Fatalf("RequestConsent: %v", err)
	}
	if prov.redirectCalls != 1 {
		t.Errorf("live mode must call 592 once, got %d", prov.redirectCalls)
	}
	if view.RedirectURL == "" {
		t.Error("live mode must surface the 592 redirect URL")
	}
	if view.Status != "PENDING" {
		t.Errorf("status = %q, want PENDING until the webhook approves", view.Status)
	}
	if prov.listCalls != 0 {
		t.Errorf("live mode must not confirm from 591 yet, got %d calls", prov.listCalls)
	}
}

func TestHandleConsentNotification_Approves(t *testing.T) {
	prov := &fakeProvider{consentHandle: handle, consentStatus: "PENDING", list: activeList()}
	repo := newFakeRepo()
	s := New(prov, repo, &fakeSpend{}, Config{RedirectMode: "live"}, nil)

	uid := uuid.New()
	_ = repo.CreateConsent(context.Background(), repository.AAConsentRow{
		ConsentHandle: handle, UserID: uid, Status: "PENDING", PartyIDValue: "9988776655", VUA: "9988776655@onemoney",
	})

	err := s.HandleConsentNotification(context.Background(), idbi.PushConsentNotification{
		ConsentHandle: handle, EventType: "CONSENT", EventStatus: "CONSENT_APPROVED", ConsentID: "CONSENT-0001",
	})
	if err != nil {
		t.Fatalf("HandleConsentNotification: %v", err)
	}
	if got := repo.consents[handle]; got.Status != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE", got.Status)
	}
	if len(repo.linked[handle]) != 1 {
		t.Errorf("expected linked accounts pulled on approval, got %+v", repo.linked[handle])
	}
}

func TestHandleConsentNotification_UnknownHandleIsIgnored(t *testing.T) {
	s := New(&fakeProvider{}, newFakeRepo(), &fakeSpend{}, Config{}, nil)
	if err := s.HandleConsentNotification(context.Background(), idbi.PushConsentNotification{
		ConsentHandle: "nope", EventStatus: "CONSENT_APPROVED",
	}); err != nil {
		t.Fatalf("unknown handle must be a no-op, got %v", err)
	}
}

func TestHandleDataNotification_FetchesStatements(t *testing.T) {
	prov := &fakeProvider{consentHandle: handle, consentStatus: "PENDING", list: activeList(), stmt: stmtResp()}
	repo := newFakeRepo()
	spend := &fakeSpend{}
	s := New(prov, repo, spend, Config{RedirectMode: "stub"}, nil)

	uid := uuid.New()
	if _, err := s.RequestConsent(context.Background(), uid, "9988776655", "660100100003"); err != nil {
		t.Fatalf("setup RequestConsent: %v", err)
	}

	err := s.HandleDataNotification(context.Background(), idbi.PushDataNotification{
		ConsentHandle: handle, EventType: "DATA", EventStatus: "DATA_READY", ConsentID: "CONSENT-0001",
	})
	if err != nil {
		t.Fatalf("HandleDataNotification: %v", err)
	}
	if spend.source != SourceAA {
		t.Errorf("spend source = %q, want %q", spend.source, SourceAA)
	}
	if len(spend.rows) != 2 {
		t.Fatalf("expected 2 spend rows, got %d", len(spend.rows))
	}
	if repo.consents[handle].LastFetchedAt == nil {
		t.Error("last_fetched_at not stamped")
	}
}

func TestHandleDataNotification_NonReadyIgnored(t *testing.T) {
	prov := &fakeProvider{consentHandle: handle, consentStatus: "PENDING", list: activeList(), stmt: stmtResp()}
	repo := newFakeRepo()
	spend := &fakeSpend{}
	s := New(prov, repo, spend, Config{RedirectMode: "stub"}, nil)
	_ = repo.CreateConsent(context.Background(), repository.AAConsentRow{ConsentHandle: handle, UserID: uuid.New()})

	if err := s.HandleDataNotification(context.Background(), idbi.PushDataNotification{
		ConsentHandle: handle, EventStatus: "DATA_DENIED",
	}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(spend.rows) != 0 {
		t.Error("non-ready data event must not fetch")
	}
}

func TestCompleteRedirect_Approved(t *testing.T) {
	prov := &fakeProvider{consentHandle: handle, consentStatus: "PENDING", list: activeList()}
	dec := &idbi.GenerateDecryptedResponse{}
	dec.Data.Srcref = handle
	dec.Data.Errorcode = "0"
	dec.Data.Status = "S"
	prov.decrypted = dec
	repo := newFakeRepo()
	s := New(prov, repo, &fakeSpend{}, Config{RedirectMode: "live"}, nil)

	uid := uuid.New()
	_ = repo.CreateConsent(context.Background(), repository.AAConsentRow{
		ConsentHandle: handle, UserID: uid, Status: "PENDING", PartyIDValue: "9988776655", VUA: "9988776655@onemoney",
	})

	view, err := s.CompleteRedirect(context.Background(), uid, "ecres-blob", "250220261248050", "fi-blob")
	if err != nil {
		t.Fatalf("CompleteRedirect: %v", err)
	}
	if prov.decryptedCalls != 1 {
		t.Errorf("593 called %d times, want 1", prov.decryptedCalls)
	}
	if view.Status != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE", view.Status)
	}
	if len(view.LinkedAccounts) != 1 {
		t.Errorf("linked accounts pulled after approval: %+v", view.LinkedAccounts)
	}
}

func TestCompleteRedirect_Rejected(t *testing.T) {
	prov := &fakeProvider{consentHandle: handle}
	dec := &idbi.GenerateDecryptedResponse{}
	dec.Data.Srcref = handle
	dec.Data.Errorcode = "1"
	dec.Data.Status = "F"
	prov.decrypted = dec
	repo := newFakeRepo()
	s := New(prov, repo, &fakeSpend{}, Config{RedirectMode: "live"}, nil)

	uid := uuid.New()
	_ = repo.CreateConsent(context.Background(), repository.AAConsentRow{ConsentHandle: handle, UserID: uid, Status: "PENDING"})

	view, err := s.CompleteRedirect(context.Background(), uid, "ecres", "d", "fi")
	if err != nil {
		t.Fatalf("CompleteRedirect: %v", err)
	}
	if view.Status != "REJECTED" {
		t.Errorf("status = %q, want REJECTED", view.Status)
	}
	if prov.listCalls != 0 {
		t.Error("rejected consent must not pull the account list")
	}
}

func TestFetchStatements_FinProSource(t *testing.T) {
	prov := &fakeProvider{consentHandle: handle, consentStatus: "PENDING", list: activeList(), stmt: stmtResp()}
	repo := newFakeRepo()
	spend := &fakeSpend{}
	s := New(prov, repo, spend, Config{RedirectMode: "stub", StatementSource: "finpro"}, nil)

	uid := uuid.New()
	if _, err := s.RequestConsent(context.Background(), uid, "9988776655", "660100100003"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := s.FetchStatements(context.Background(), uid, handle); err != nil {
		t.Fatalf("FetchStatements: %v", err)
	}
	if prov.stmtFinProCalls != 1 {
		t.Errorf("739 called %d times, want 1", prov.stmtFinProCalls)
	}
	if prov.stmtCalls != 0 {
		t.Errorf("595 called %d times, want 0 (finpro selected)", prov.stmtCalls)
	}
	if len(spend.rows) != 2 {
		t.Errorf("spend rows = %d, want 2", len(spend.rows))
	}
}
