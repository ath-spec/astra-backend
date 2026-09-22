package database

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/commons/connectors"
)

type Database struct {
	Pool *pgxpool.Pool
}

// NewDatabase opens the application connection pool via the shared Postgres
// connector (bounded retry, production pool tuning, DSN/env-driven TLS and
// transaction-pooler compatibility).
func NewDatabase(ctx context.Context, connectionString string) (*Database, error) {
	pool, err := connectors.CreatePostgresPool(ctx, connectionString)
	if err != nil {
		return nil, err
	}
	slog.Info("database pool ready")
	db := &Database{Pool: pool}
	go db.monitorPoolHealth(ctx)
	return db, nil
}

// monitorPoolHealth periodically samples pgxpool's own stats so pool
// exhaustion under real load shows up in logs before it turns into a
// user-visible timeout. MaxConns=25 was picked without any visibility into
// whether that ceiling is ever actually hit — this makes that observable
// instead of guessing at a new number. Only logs when EmptyAcquireCount (a
// request that had to wait because every connection was checked out) has
// grown since the last sample, so a healthy pool stays silent.
func (db *Database) monitorPoolHealth(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	var lastEmptyAcquires int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stat := db.Pool.Stat()
			if empty := stat.EmptyAcquireCount(); empty > lastEmptyAcquires {
				slog.Warn("db pool exhaustion: requests waited for a free connection",
					"new_empty_acquires", empty-lastEmptyAcquires,
					"total_empty_acquires", empty,
					"acquired_conns", stat.AcquiredConns(),
					"idle_conns", stat.IdleConns(),
					"max_conns", stat.MaxConns(),
				)
				lastEmptyAcquires = empty
			}
		}
	}
}

func (db *Database) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}
