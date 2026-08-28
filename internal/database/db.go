package database

import (
	"context"
	"log/slog"

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
	return &Database{Pool: pool}, nil
}

func (db *Database) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}
