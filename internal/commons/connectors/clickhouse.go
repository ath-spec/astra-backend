package connectors

// ClickHouse connector — PROVISION FOR LATER, not yet wired.
//
// (In z-backend this connector was itself commented out — kept here only so the
// package has a placeholder for a columnar analytics store.)
//
// Postgres serves every read Astra has today. ClickHouse becomes interesting only
// for high-volume append-only analytics that are painful on OLTP Postgres:
//
//   - per-tick / per-event market and price history
//   - clickstream / product analytics
//   - large time-range portfolio aggregations across the whole book
//
// Before adding it, check whether Postgres + partitioning + a read replica, or
// Timescale, covers the need — a second database is real operational weight. On
// AWS there is no managed ClickHouse; it would be ClickHouse Cloud or self-hosted
// on EC2/EKS.
//
// To activate: `go get github.com/ClickHouse/clickhouse-go/v2`, add a connector
// here with the same bounded-retry shape as CreatePostgresPool, and a
// CLICKHOUSE_DSN config knob.
//
// Ported (as a stub) from z-backend server/common/connectors/clickhouse.go.
