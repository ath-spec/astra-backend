package repository

// IDBIRepository and IDBIAARepository have been migrated to use
// SQLC-generated queries from internal/db. The generated Queries type
// (db.New / db.NewWithTx) is the sole access path to the IDBI tables —
// no hand-written SQL remains in this package for those tables.
//
// File layout after migration:
//   idbi_repo.go     — IDBIRepository (customer link, accounts, loans,
//                       credit exposure, spend transactions)
//   idbi_aa_repo.go  — IDBIAARepository (AA consents + linked accounts)
//   idbi_seed_repo.go — IDBISeedRepository (seed customers from CSV)
