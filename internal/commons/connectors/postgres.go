// Package connectors holds retry-wrapped session factories for the backing
// stores the service talks to. Ported from the z-backend
// server/common/connectors package, which has run this Postgres logic in
// production.
//
// Live connectors: postgres.go (wired into cmd/api) and redis.go (compiled and
// tested, not yet constructed at boot — REDIS_URL is unset). kafka.go and
// clickhouse.go are provisions for later: a commented reference implementation
// plus a note on when that store is worth adding and which config knob / AWS
// service it maps to. Activating those is "go get <client>, uncomment, wire in
// main.go".
package connectors

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool tuning. These are the values the z-backend Postgres connector has run in
// production. They are deliberately conservative: on a multi-replica deploy,
// (MaxConns * replica count) must stay comfortably under the server's
// max_connections, and a bounded lifetime lets a rolling failover / DNS change
// drain cleanly instead of pinning dead sockets.
const (
	poolMaxConns        = 25
	poolMinConns        = 5
	poolMaxConnLifetime = time.Hour
	poolMaxConnIdleTime = 30 * time.Minute
	poolConnectTimeout  = 10 * time.Second

	connectRetries    = 5
	connectRetryDelay = 5 * time.Second
)

// CreatePostgresPool opens a pgx pool against dsn, retrying a bounded number of
// times so a database that is still coming up at boot does not crash-loop the
// process. TLS and pgBouncer/RDS-Proxy compatibility are toggled from the
// connection string and environment (see poolConfig). The returned pool has been
// Ping-verified.
func CreatePostgresPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, fmt.Errorf("database connection string is empty")
	}

	cfg, err := poolConfig(dsn)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 1; attempt <= connectRetries; attempt++ {
		pool, perr := pgxpool.NewWithConfig(ctx, cfg)
		if perr == nil {
			if perr = pool.Ping(ctx); perr == nil {
				slog.Info("connected to postgres",
					"max_conns", cfg.MaxConns,
					"tls", cfg.ConnConfig.TLSConfig != nil,
					"simple_protocol", cfg.ConnConfig.DefaultQueryExecMode == pgx.QueryExecModeSimpleProtocol,
				)
				return pool, nil
			}
			pool.Close()
		}
		lastErr = perr

		if ctx.Err() != nil {
			return nil, fmt.Errorf("unable to connect to database: %w", ctx.Err())
		}
		if attempt < connectRetries {
			slog.Warn("postgres not ready, retrying",
				"attempt", attempt, "of", connectRetries, "error", perr)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("unable to connect to database: %w", ctx.Err())
			case <-time.After(connectRetryDelay):
			}
		}
	}

	return nil, fmt.Errorf("unable to connect to database after %d attempts: %w", connectRetries, lastErr)
}

// poolConfig parses the DSN and applies the production pool settings.
//
//   - TLS is enabled (verifying, TLS 1.2 floor) when the DSN asks for it
//     (sslmode=require|verify-ca|verify-full) or DB_TLS=true. This is what RDS
//     wants; Railway's internal network does not need it.
//   - Prepared-statement caching is disabled when DB_POOL_MODE=transaction, which
//     is required to sit behind a transaction-pooling proxy (PgBouncer, Supabase
//     pooler, RDS Proxy) — otherwise you get "prepared statement already exists".
func poolConfig(dsn string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to parse postgres dsn: %w", err)
	}

	cfg.MaxConns = poolMaxConns
	cfg.MinConns = poolMinConns
	cfg.MaxConnLifetime = poolMaxConnLifetime
	cfg.MaxConnIdleTime = poolMaxConnIdleTime
	cfg.ConnConfig.ConnectTimeout = poolConnectTimeout

	if wantTLS(dsn) {
		cfg.ConnConfig.TLSConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: false, // always verify the server certificate
			ServerName:         cfg.ConnConfig.Host,
		}
	}

	if strings.EqualFold(os.Getenv("DB_POOL_MODE"), "transaction") {
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	return cfg, nil
}

func wantTLS(dsn string) bool {
	if strings.EqualFold(os.Getenv("DB_TLS"), "true") {
		return true
	}
	lower := strings.ToLower(dsn)
	for _, m := range []string{"sslmode=require", "sslmode=verify-ca", "sslmode=verify-full"} {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}
