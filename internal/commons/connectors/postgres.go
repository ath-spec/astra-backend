package connectors

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/yourusername/astra-backend/internal/commons/logger"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CreatePostgresPool opens a production pool with bounded retry: up to 5
// attempts with capped exponential backoff, honoring ctx cancellation and
// returning an error on exhaustion instead of blocking forever. TLS is
// driven by the DSN's own sslmode (pgx handles this natively via
// ParseConfig) rather than a hardcoded override, so the same code path
// works against RDS or a local dev Postgres.
//
// Set DB_TRANSACTION_POOLER=true when the DSN points at a transaction-mode
// connection pooler (e.g. pgBouncer, Supabase's pooler) that doesn't
// support server-side prepared statements.
func CreatePostgresPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}

	if os.Getenv("DB_TRANSACTION_POOLER") == "true" {
		config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	config.MaxConns = 100
	config.MinConns = 5
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute
	config.ConnConfig.ConnectTimeout = 10 * time.Second

	const maxAttempts = 5
	backoff := time.Second
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pool, poolErr := pgxpool.NewWithConfig(ctx, config)
		if poolErr == nil {
			if pingErr := pool.Ping(ctx); pingErr == nil {
				logger.Info("connected to postgres")
				return pool, nil
			} else {
				pool.Close()
				lastErr = pingErr
			}
		} else {
			lastErr = poolErr
		}

		logger.Error("postgres connection attempt %d/%d failed: %s", attempt, maxAttempts, lastErr)
		if attempt == maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 5*time.Second {
			backoff *= 2
		}
	}

	return nil, fmt.Errorf("unable to connect to postgres after %d attempts: %w", maxAttempts, lastErr)
}

// Infinite iterator that returns the Postgres session
func CreatePostgresSession(dsn string) *pgxpool.Pool {
	count := 0

	// Parse the DSN first
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		logger.Error("unable to parse postgres dsn: %s", err)
	} else {
		// Disable prepared statements for pgBouncer compatibility (Supabase)
		config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}

	for {
		var db *pgxpool.Pool
		if config != nil {
			db, err = pgxpool.NewWithConfig(context.Background(), config)
		} else {
			db, err = pgxpool.New(context.Background(), dsn)
		}

		if err != nil {
			count++
		} else {
			logger.Info("connected to postgres!")
			return db
		}
		if count == 5 {
			logger.Error("unable to connect to postgres: %s", err)
			logger.Info("retying in 5 seconds...")
			time.Sleep(time.Second * 5)
			count = 0
		}
	}
}
