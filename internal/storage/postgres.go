package storage

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samil/notification/internal/domain"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateBatch(ctx context.Context, batch *domain.Batch, notifications []*domain.Notification) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`INSERT INTO batches (id, idempotency_key, status, total_count, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		batch.ID, batch.IdempotencyKey, string(batch.Status), batch.TotalCount, batch.CreatedAt, batch.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert batch: %w", err)
	}

	if len(notifications) > 0 {
		rows := make([][]any, len(notifications))
		for i, n := range notifications {
			rows[i] = []any{
				n.ID, n.BatchID, n.Recipient, string(n.Channel), n.Content,
				string(n.Priority), string(n.Status), n.CreatedAt, n.UpdatedAt,
			}
		}

		_, err = tx.CopyFrom(ctx,
			pgx.Identifier{"notifications"},
			[]string{"id", "batch_id", "recipient", "channel", "content", "priority", "status", "created_at", "updated_at"},
			pgx.CopyFromRows(rows),
		)
		if err != nil {
			return fmt.Errorf("copy from notifications: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (r *PostgresRepository) GetBatchByIdempotencyKey(ctx context.Context, key uuid.UUID) (*domain.Batch, error) {
	var b domain.Batch
	var status string

	err := r.pool.QueryRow(ctx,
		`SELECT id, idempotency_key, status, total_count, created_at, updated_at
		 FROM batches WHERE idempotency_key = $1`,
		key,
	).Scan(&b.ID, &b.IdempotencyKey, &status, &b.TotalCount, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("query batch by idempotency key: %w", err)
	}

	b.Status = domain.BatchStatus(status)
	return &b, nil
}