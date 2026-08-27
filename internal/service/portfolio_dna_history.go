package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/apitime"
	paDomain "github.com/yourusername/astra-backend/internal/domain/portfolioanalysis"
)

// RecordDNASnapshot computes the client's current allocation/DNA and upserts
// today's row in portfolio_dna_snapshots. It returns the freshly computed
// AllocationResult so callers that also need "current" DNA don't recompute.
// Snapshot-write failures are surfaced as errors; callers that treat the
// snapshot as best-effort (e.g. the user's own allocation read) should log
// and continue.
func (s *PortfolioAnalysisService) RecordDNASnapshot(ctx context.Context, userID uuid.UUID) (*paDomain.AllocationResult, error) {
	res, err := s.Allocation(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Coerce nil slices to empty so the JSONB columns (NOT NULL) never
	// receive a literal `null`.
	sectors := res.SectorExposure
	if sectors == nil {
		sectors = []paDomain.SectorExposure{}
	}
	buckets := res.VolatilityBuckets
	if buckets == nil {
		buckets = []paDomain.VolatilityBucket{}
	}
	genomeJSON, err := json.Marshal(res.Genome)
	if err != nil {
		return res, fmt.Errorf("marshal genome: %w", err)
	}
	sectorJSON, err := json.Marshal(sectors)
	if err != nil {
		return res, fmt.Errorf("marshal sector exposure: %w", err)
	}
	volJSON, err := json.Marshal(buckets)
	if err != nil {
		return res, fmt.Errorf("marshal volatility buckets: %w", err)
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO portfolio_dna_snapshots
			(user_id, snapshot_date, level, total_value, equity_pct, debt_pct, other_pct, genome, sector_exposure, volatility_buckets)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, snapshot_date) DO UPDATE SET
			level = EXCLUDED.level,
			total_value = EXCLUDED.total_value,
			equity_pct = EXCLUDED.equity_pct,
			debt_pct = EXCLUDED.debt_pct,
			other_pct = EXCLUDED.other_pct,
			genome = EXCLUDED.genome,
			sector_exposure = EXCLUDED.sector_exposure,
			volatility_buckets = EXCLUDED.volatility_buckets,
			updated_at = now()
	`, userID, today, res.Level, res.TotalValue, res.EquityPct, res.DebtPct, res.OtherPct,
		genomeJSON, sectorJSON, volJSON)
	if err != nil {
		return res, fmt.Errorf("record dna snapshot: %w", err)
	}
	return res, nil
}

// DNAHistory returns up to `days` of recorded DNA snapshots, oldest first.
func (s *PortfolioAnalysisService) DNAHistory(ctx context.Context, userID uuid.UUID, days int) ([]paDomain.DNAHistoryPoint, error) {
	if days <= 0 || days > 3650 {
		days = 365
	}
	rows, err := s.pool.Query(ctx, `
		SELECT snapshot_date, level, total_value, equity_pct, debt_pct, other_pct,
		       genome, sector_exposure, volatility_buckets
		FROM portfolio_dna_snapshots
		WHERE user_id = $1
		ORDER BY snapshot_date DESC
		LIMIT $2
	`, userID, days)
	if err != nil {
		return nil, fmt.Errorf("query dna snapshots: %w", err)
	}
	defer rows.Close()

	points := make([]paDomain.DNAHistoryPoint, 0, days)
	for rows.Next() {
		var (
			p       paDomain.DNAHistoryPoint
			date    time.Time
			genomeB []byte
			sectorB []byte
			volB    []byte
		)
		if err := rows.Scan(&date, &p.Level, &p.TotalValue, &p.EquityPct, &p.DebtPct, &p.OtherPct,
			&genomeB, &sectorB, &volB); err != nil {
			return nil, fmt.Errorf("scan dna snapshot: %w", err)
		}
		p.Date = apitime.Epoch(date)
		_ = json.Unmarshal(genomeB, &p.Genome)
		_ = json.Unmarshal(sectorB, &p.SectorExposure)
		_ = json.Unmarshal(volB, &p.VolatilityBuckets)
		if p.SectorExposure == nil {
			p.SectorExposure = []paDomain.SectorExposure{}
		}
		if p.VolatilityBuckets == nil {
			p.VolatilityBuckets = []paDomain.VolatilityBucket{}
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dna snapshots: %w", err)
	}

	// Reverse to oldest-first (query is DESC so LIMIT keeps the most recent).
	for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
		points[i], points[j] = points[j], points[i]
	}
	return points, nil
}
