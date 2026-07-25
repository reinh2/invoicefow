package processing

import (
	"context"

	"github.com/reinhlord/invoiceflow/internal/metrics"
)

// StatusCounts reports how many documents sit in each state and how many
// durable jobs sit in each job status. It is read at scrape time rather than
// counted in memory, so a restarted process reports the truth immediately
// instead of counting up from zero.
//
// Both groupings are aggregate counts of server-owned status values. No
// document id, hash, storage key, supplier, or amount is exposed.
func (r *Repository) StatusCounts(ctx context.Context) (metrics.StatusCounts, error) {
	documents, err := r.countByStatus(ctx, `SELECT status, count(*) FROM documents GROUP BY status`)
	if err != nil {
		return metrics.StatusCounts{}, err
	}
	jobs, err := r.countByStatus(ctx, `SELECT status, count(*) FROM jobs GROUP BY status`)
	if err != nil {
		return metrics.StatusCounts{}, err
	}
	return metrics.StatusCounts{DocumentsByStatus: documents, JobsByStatus: jobs}, nil
}

func (r *Repository) countByStatus(ctx context.Context, query string) ([]metrics.LabeledValue, error) {
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var counts []metrics.LabeledValue
	for rows.Next() {
		var value metrics.LabeledValue
		if err := rows.Scan(&value.Label, &value.Value); err != nil {
			return nil, err
		}
		counts = append(counts, value)
	}
	return counts, rows.Err()
}
