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

func (r *PostgresRepository) GetNotificationByID(ctx context.Context, id uuid.UUID) (*domain.Notification, error) {
	var n domain.Notification
	var channel, priority, status string

	err := r.pool.QueryRow(ctx,
		`SELECT id, batch_id, recipient, channel, content, priority, status, error_message, retry_count, created_at, updated_at
		 FROM notifications WHERE id = $1`,
		id,
	).Scan(&n.ID, &n.BatchID, &n.Recipient, &channel, &n.Content, &priority, &status, &n.ErrorMessage, &n.RetryCount, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("query notification by id: %w", err)
	}

	n.Channel = domain.Channel(channel)
	n.Priority = domain.Priority(priority)
	n.Status = domain.NotificationStatus(status)
	return &n, nil
}

func (r *PostgresRepository) UpdateNotificationStatus(ctx context.Context, id uuid.UUID, status domain.NotificationStatus, errMsg *string, retryCount int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET status = $2, error_message = $3, retry_count = $4, updated_at = NOW() WHERE id = $1`,
		id, string(status), errMsg, retryCount,
	)
	if err != nil {
		return fmt.Errorf("update notification status: %w", err)
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