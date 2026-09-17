package middlewares

import (
	"context"
	"net/http"
	"os"

	"github.com/yourusername/astra-backend/internal/commons/redis"
	"github.com/yourusername/astra-backend/internal/commons/token"
	"github.com/yourusername/astra-backend/internal/commons/util"

	"github.com/google/uuid"
)

// frontendBypassAllowed gates the "Bearer frontend" auth bypass below. It
// requires an explicit opt-in (ALLOW_FRONTEND_BYPASS=true) for local/staging
// use, and is hard-disabled in production regardless of that flag — a
// second, environment-driven check so a misconfigured flag can't reopen it.
func frontendBypassAllowed() bool {
	return !util.IsProduction() && os.Getenv("ALLOW_FRONTEND_BYPASS") == "true"
}

type TokenPayloadType string

var TokenPayloadKey TokenPayloadType = "auth-payload"

// authRedis holds the Redis client for session validation.
var authRedis redis.Client

// InitAuthMiddleware sets the Redis client used by TokenMiddleware.
func InitAuthMiddleware(rc redis.Client) {
	authRedis = rc
}

// Middleware to check for an existing token and adding to current context.
// Adds the context of invalid token, if a token does not exist in the request,
// clients can handle this as required
func TokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")

		if len(auth) <= len("Bearer ") {
			util.ErrorJson(w, util.ErrInvalidToken)
			return
		}

		bearer := "Bearer "
		auth = auth[len(bearer):]

		// FRONTEND BYPASS: "Bearer frontend" skips token validation.
		// Dev/staging only — see frontendBypassAllowed.
		if auth == "frontend" && frontendBypassAllowed() {
			id, _ := uuid.Parse("550e8400-e29b-41d4-a716-446655440000")
			ctx := context.WithValue(r.Context(), TokenPayloadKey, token.Payload{
				Zyid:      id,
				Roles:     util.CONSUL,
				TokenType: util.AccessTokenType,
				Phone:     "+919876543210",
				Email:     "caligula@hotmail.com",
			})
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
			return
		}

		payload, err := token.VerifyToken(auth)
		if err != nil {
			util.ErrorJson(w, err)
			return
		}

		// Session validation: check this token's SessionId matches active session in Redis
		if authRedis != nil && payload.TokenType == util.AccessTokenType {
			sessionKey := "session:" + payload.Zyid.String()
			activeSessionId, err := authRedis.Get(r.Context(), sessionKey)
			if err != nil {
				// Redis key missing = session was never registered OR Redis is down
				util.ErrorJson(w, util.ErrMultipleLogin)
				return
			}
			if activeSessionId != payload.SessionId.String() {
				// Token's session != active session: another device logged in
				util.ErrorJson(w, util.ErrMultipleLogin)
				return
			}
		}

		ctx := context.WithValue(r.Context(), TokenPayloadKey, *payload)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

// Middleware to get the token payload from a context given.
// Any handler that requires a token must call this method to get the payload
// and check for token existence.
func GetTokenPayloadFromContext(ctx context.Context, tokenType string) (token.Payload, error) {
	tokenPayload := ctx.Value(TokenPayloadKey).(token.Payload)
	if tokenPayload.TokenType != tokenType {
		return token.Payload{}, util.ErrInvalidToken
	}
	return tokenPayload, nil
}

// RefreshTokenMiddleware specifically handles refresh tokens
func RefreshTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")

		if len(auth) <= len("Bearer ") {
			util.ErrorJson(w, util.ErrInvalidToken)
			return
		}

		bearer := "Bearer "
		auth = auth[len(bearer):]

		// FRONTEND BYPASS: "Bearer frontend" skips token validation.
		// Dev/staging only — see frontendBypassAllowed.
		if auth == "frontend" && frontendBypassAllowed() {
			id, _ := uuid.Parse("550e8400-e29b-41d4-a716-446655440000")
			tokenId, _ := uuid.Parse("80a05804-84c7-4e39-a3a3-fa7c7e802300")
			ctx := context.WithValue(r.Context(), TokenPayloadKey, token.Payload{
				Id:        tokenId,
				Zyid:      id,
				Roles:     util.CONSUL,
				TokenType: util.RefreshTokenType,
				Phone:     "+919876543210",
				Email:     "caligula@hotmail.com",
			})
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
			return
		}

		payload, err := token.VerifyToken(auth)
		if err != nil {
			util.ErrorJson(w, err)
			return
		}

		// Verify that this is a refresh token
		if payload.TokenType != util.RefreshTokenType {
			util.ErrorJson(w, util.ErrInvalidToken)
			return
		}

		ctx := context.WithValue(r.Context(), TokenPayloadKey, *payload)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

func AddDefaultProfileToContext(r *http.Request, tokenType string) *http.Request {
	id, _ := uuid.Parse("0b927d97-782a-4c82-b9d2-e4e06774ed37")
	tokenId, _ := uuid.Parse("80a05804-84c7-4e39-a3a3-fa7c7e802300")
	ctx := context.WithValue(r.Context(), TokenPayloadKey, token.Payload{
		Id:        tokenId,
		Zyid:      id,
		Phone:     "+919876543210",
		Roles:     util.CONSUL,
		TokenType: tokenType,
	})
	return r.WithContext(ctx)
}
