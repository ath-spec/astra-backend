package middlewares

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"github.com/yourusername/astra-backend/internal/commons/token"
	"github.com/yourusername/astra-backend/internal/commons/util"
	"strings"
)

var mockPhoneNumbers = map[string]bool{
	"+919876543210": true, "+910123456789": true, "+913333333333": true, "+914444444444": true,
	"+915555555555": true, "+916666666666": true, "+917777777777": true, "+918888888888": true,
	"+911231231231": true, "+911234512345": true, "+912222222222": true, "+912333333333": true,
	"+912444444444": true, "+912555555555": true, "+912666666666": true, "+912777777777": true,
	"+912000000000": true, "+912111111111": true, "+912123456789": true, "+912987654321": true,
	"+913222222222": true, "+913444444444": true, "+913555555555": true,
	"+913666666666": true, "+913777777777": true, "+913888888888": true, "+913999999999": true,
	"+914111111111": true, "+914123456789": true, "+914987654321": true, "+914222222222": true,
	"+914333333333": true, "+914555555555": true, "+914666666666": true,
	"+914777777777": true, "+914888888888": true, "+912888888888": true, "+912999999999": true,
	"+919000000001": true, "+919000000002": true, "+919000000003": true, "+919000000004": true,
	"+919000000005": true, "+919000000006": true, "+919000000007": true, "+919000000008": true,
	"+919000000009": true, "+919000000010": true, "+919000000011": true, "+919000000012": true,
	"+919000000013": true, "+919000000014": true, "+919000000015": true, "+919000000016": true,
	"+919000000017": true, "+919000000018": true, "+919000000019": true, "+919000000020": true,
	"+919000000021": true, "+919000000022": true, "+919000000023": true, "+919000000024": true,
	"+919000000025": true, "+919000000026": true, "+919000000027": true, "+919000000028": true,
	"+919000000029": true, "+919000000030": true, "+919000000031": true, "+919000000032": true,
	"+919000000033": true, "+919000000034": true, "+919000000035": true, "+919000000036": true,
	"+919000000037": true, "+919000000038": true, "+919000000039": true, "+919000000040": true,
	"+919000000041": true, "+919000000042": true, "+919000000043": true, "+919000000044": true,
	"+919000000045": true, "+919000000046": true, "+919000000047": true, "+919000000048": true,
	"+919000000049": true, "+919000000050": true, "+919000000051": true, "+919000000052": true,
	"+919000000053": true, "+919000000054": true, "+919000000055": true, "+919000000056": true,
	"+919000000057": true, "+919000000058": true, "+919000000059": true, "+919000000060": true,
	"+919000000061": true, "+919000000062": true, "+919000000063": true, "+919000000064": true,
	"+919000000065": true, "+919000000066": true, "+919000000067": true, "+919000000068": true,
	"+919000000069": true, "+919000000070": true, "+919000000071": true, "+919000000072": true,
	"+919000000073": true, "+919000000074": true, "+919000000075": true, "+919000000076": true,
	"+919000000077": true, "+919000000078": true, "+919000000079": true, "+919000000080": true,
	"+919000000081": true, "+919000000082": true, "+919000000083": true, "+919000000084": true,
	"+919000000085": true, "+919000000086": true, "+919000000087": true, "+919000000088": true,
	"+919000000089": true, "+919000000090": true, "+919000000091": true, "+919000000092": true,
	"+919000000097": true, "+919000000098": true, "+919000000099": true, "+919000000100": true,
}

func isMockPhone(phone string) bool {
	if phone >= "+911000000000" && phone <= "+911000002500" {
		return true
	}
	return mockPhoneNumbers[phone]
}

// TemporaryMockBlockerMiddleware blocks any request that doesn't belong to a mock phone number.
// It checks the Authorization token or the request body (for OTP endpoints).
// It is a no-op in production: this exists to keep a staging/demo build
// restricted to test numbers, not to ever gate real customer traffic —
// wiring it into a prod router by mistake must not lock out real users.
func TemporaryMockBlockerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if util.IsProduction() {
			next.ServeHTTP(w, r)
			return
		}

		var phone string

		// 1. Try to get phone from Authorization token
		auth := r.Header.Get("Authorization")
		if len(auth) > len("Bearer ") {
			bearer := "Bearer "
			authToken := auth[len(bearer):]
			if authToken != "frontend" {
				payload, err := token.VerifyToken(authToken)
				if err == nil {
					phone = payload.Phone
				}
			}
		}

		// 2. Try to get phone from request body ONLY for OTP paths to avoid memory DoS on large uploads
		if phone == "" && r.Body != nil {
			path := r.URL.Path
			if strings.Contains(path, "/otp/send") || strings.Contains(path, "/otp/verify") {
				// Use a LimitReader to prevent reading massive payloads into memory (8KB max)
				bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 8192))
			if err == nil && len(bodyBytes) > 0 {
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				var payload struct {
					Phone string `json:"phone"`
				}
				if err := json.Unmarshal(bodyBytes, &payload); err == nil && payload.Phone != "" {
					phone = payload.Phone
				}
			}
			}
		}

		// If a phone number was found and it's not a mock number, block it.
		if phone != "" && !isMockPhone(phone) {
			util.NotifyAuthAction(phone, "UNKNOWN", util.GetClientIP(r), "MOCK_BLOCKED")
			util.ErrorJson(w, errors.New("sorry bud, but you're not on our list. live in fomo"))
			return
		}

		next.ServeHTTP(w, r)
	})
}
