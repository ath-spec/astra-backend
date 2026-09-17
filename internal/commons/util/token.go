package util

import "strings"

const (
	AccessTokenType  = "access"
	RefreshTokenType = "refresh"
	OtpTokenType     = "otp"
	OtpResendType    = "otp-resend"
	EmailTokenType   = "email"
)

const TokenVersionType = "v"

func Highphenate(inp ...string) string {
	return strings.Join(inp, "-")
}
