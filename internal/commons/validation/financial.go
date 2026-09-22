package validation

import (
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
)

// PAN validation regex (Indian PAN format)
var panRegex = regexp.MustCompile(`^[A-Z]{5}[0-9]{4}[A-Z]{1}$`)

// Phone number validation (E.164 format)
var phoneRegex = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)

// Income bracket validation
var incomeBracketRegex = regexp.MustCompile(`^(0-25000|25000-50000|50000-100000|100000-500000|500000\+)$`)

// PAN validation
func panValidation(fl validator.FieldLevel) bool {
	input := strings.ToUpper(fl.Field().String())
	return panRegex.MatchString(input)
}

// Phone validation
func phoneValidation(fl validator.FieldLevel) bool {
	input := fl.Field().String()
	return phoneRegex.MatchString(input)
}

// Income bracket validation
func incomeBracketValidation(fl validator.FieldLevel) bool {
	input := fl.Field().String()
	return incomeBracketRegex.MatchString(input)
}

// Financial goals validation (no special characters that could be dangerous)
func financialGoalsValidation(fl validator.FieldLevel) bool {
	input := fl.Field().String()
	// Allow letters, numbers, spaces, commas, periods, and basic punctuation
	return regexp.MustCompile(`^[a-zA-Z0-9\s,.\-()]+$`).MatchString(input)
}
