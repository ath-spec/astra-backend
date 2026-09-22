// Package discoverypool holds the deterministic "AA discovery" bank/account
// generator shared between the discovery endpoint (aa_handler.go, which
// offers these as pick-able candidates) and initial signup seeding
// (user_repo.go, which pre-links the first couple as a starting point) —
// both need to compute the exact same account for a given user+bank+slot so
// a seeded account is correctly recognized as already-linked when discovery
// runs, instead of reappearing as a duplicate candidate.
package discoverypool

import (
	"fmt"
	"hash/fnv"

	"github.com/google/uuid"
)

// BankPool is every bank the discovery/connect flow can offer as a
// candidate, in a single fixed order — every user sees the exact same list,
// matching the app's one consistent "Good Investor" archetype instead of
// varying cosmetically per user via a hash. This used to be 4 differently-
// ordered rotations of the same list, selected by a phone+userID hash
// (archetypeForUser in aa_handler.go); collapsed to one for the same reason
// the investor-archetype seeding itself was collapsed to one — deterministic,
// identical output for every user is easier to reason about and demo than
// four cosmetically-different-but-functionally-identical variants.
//
// Deliberately every bank the frontend has a real, dedicated logo asset for
// (see _getBankLogoAsset in banks_linking_screen.dart and _getBankLogo in
// linked_bank_accounts_screen.dart — both cover the same 17 banks), not a
// small arbitrary subset — anything else falls back to a generic icon or,
// worse, linked_bank_accounts_screen's fallback which shows the ICICI logo
// for an unrecognized name.
//
// "CONNECT MORE ACCOUNTS" is meant to be usable in a loop, picking one bank
// at a time repeatedly until the user is done, so this pool needs enough
// headroom that a real testing/demo session won't exhaust it.
var BankPool = []string{
	"Axis Bank", "ICICI Bank", "HDFC Bank", "Kotak Mahindra Bank", "State Bank of India", "Punjab National Bank",
	"Bank of Baroda", "Canara Bank", "Union Bank of India", "Bank of India", "Indian Bank",
	"Indian Overseas Bank", "UCO Bank", "Bank of Maharashtra", "Punjab & Sind Bank", "IndusInd Bank", "Yes Bank",
}

// AccountsPerBank is how many candidate account slots each bank in the pool
// offers — real customers commonly hold more than one account at the same
// bank (salary + savings + more). Starting at 2; schema/linking logic
// already supports more.
const AccountsPerBank = 2

// SeedAccountCount is how many of the pool's slots are pre-linked at signup
// (the first SeedAccountCount banks' slot 1) rather than left as pick-able
// discovery candidates — every archetype starts with a couple of accounts
// already connected, matching how MF/stocks/spend history are also
// pre-seeded, instead of a completely empty "Bank Accounts" section on
// first login.
const SeedAccountCount = 2

// SlotAccountTypes labels each per-bank slot with a distinct account type
// instead of every slot being an identical "SAVINGS" — customers commonly
// hold more than one type of account at the same bank. Cycles if
// AccountsPerBank ever exceeds this list's length.
var SlotAccountTypes = []string{"SAVINGS", "SALARY", "CURRENT", "NRE", "NRO"}

// AcctNamespace namespaces synthetic discovery-candidate IDs so they can
// never collide with a real IDBI-synced account's ID.
var AcctNamespace = uuid.MustParse("7d3b6e2a-9c41-4b8f-8e2d-5a1f9c6b0d47")

// Account is one deterministic candidate slot for a given user+bank+slot.
type Account struct {
	ID            uuid.UUID
	BankName      string
	AccountType   string
	AccountNumber string
	Balance       float64
}

// Generate computes the single deterministic account for this exact
// user+bank+slot (slot is 1-indexed). The same inputs always produce the
// same account number/balance/id, so re-running discovery — or seeding at
// signup — never drifts.
func Generate(userID uuid.UUID, bankName string, slot int) Account {
	seed := fmt.Sprintf("%s|%s|%d", userID.String(), bankName, slot)
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(seed))
	sum := hasher.Sum64()
	return Account{
		ID:            uuid.NewSHA1(AcctNamespace, []byte(seed)),
		BankName:      bankName,
		AccountType:   SlotAccountTypes[(slot-1)%len(SlotAccountTypes)],
		AccountNumber: fmt.Sprintf("%011d", sum%100000000000),
		Balance:       float64(80000 + int(sum%220000)),
	}
}
