package token

import (
	"github.com/yourusername/astra-backend/internal/commons/util"
	"time"

	"github.com/google/uuid"
)

// TokenPayload represents the structure of the token payload for the budgeting app.
type Payload struct {
	Id          uuid.UUID   `json:"id"`
	SessionId   uuid.UUID   `json:"session_id"`  // Unique session ID tied to the refresh token
	Zyid        uuid.UUID   `json:"zyid"`        // Unique user ID
	Name        string      `json:"name"`        // User's full name
	TokenType   string      `json:"token_type"`  // Token Type
	Phone       string      `json:"phone"`       // User's email address
	Email       string      `json:"email"`       // User's email address
	IssuedAt    time.Time   `json:"iat"`         // Issued at timestamp
	ExpiresAt   time.Time   `json:"exp"`         // Expiration timestamp
	Roles       util.Role   `json:"roles"`       // Roles assigned to the user
	Features    Features    `json:"features"`    // Enabled features for the user
	Preferences Preferences `json:"preferences"` // User preferences
	Limits      Limits      `json:"limits"`      // User's limits and settings
}

// Features defines the enabled features for the user.
type Features struct {
	Budgeting bool `json:"budgeting"` // Access to budgeting feature
	Analytics bool `json:"analytics"` // Access to analytics tools
}

// Preferences defines user preferences for personalization.
type Preferences struct {
	Currency string `json:"currency"` // Preferred currency (e.g., "USD")
	Language string `json:"language"` // Preferred language (e.g., "en")
	Theme    string `json:"theme"`    // Preferred app theme (e.g., "dark")
}

// Limits defines user-specific settings related to budgeting.
type Limits struct {
	MonthlyBudget   float64 `json:"monthly_budget"`   // Monthly budget limit
	ExpenseTracking bool    `json:"expense_tracking"` // Expense tracking enabled or not
}

func newPayload(p *Payload, duration time.Duration) *Payload {
	return &Payload{
		Id:          p.Id,
		SessionId:   p.SessionId,
		Zyid:        p.Zyid,
		Name:        p.Name,
		Phone:       p.Phone,
		Email:       p.Email,
		TokenType:   p.TokenType,
		Roles:       p.Roles,
		Features:    p.Features,
		Preferences: p.Preferences,
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(duration),
	}
}
