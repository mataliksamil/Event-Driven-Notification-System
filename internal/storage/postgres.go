package storage

import (
	"context"
	"fmt"
	"strings"

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

func (r *PostgresRepository) GetBatchByID(ctx context.Context, id uuid.UUID) (*domain.Batch, error) {
	var b domain.Batch
	var status string

	err := r.pool.QueryRow(ctx,
		`SELECT id, idempotency_key, status, total_count, created_at, updated_at
		 FROM batches WHERE id = $1`,
		id,
	).Scan(&b.ID, &b.IdempotencyKey, &status, &b.TotalCount, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query batch by id: %w", err)
	}

	b.Status = domain.BatchStatus(status)
	return &b, nil
}

func (r *PostgresRepository) ListNotifications(ctx context.Context, filter domain.NotificationFilter) ([]*domain.Notification, int, error) {
	var conditions []string
	var args []any
	argIdx := 1

	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(*filter.Status))
		argIdx++
	}
	if filter.Channel != nil {
		conditions = append(conditions, fmt.Sprintf("channel = $%d", argIdx))
		args = append(args, string(*filter.Channel))
		argIdx++
	}
	if filter.BatchID != nil {
		conditions = append(conditions, fmt.Sprintf("batch_id = $%d", argIdx))
		args = append(args, *filter.BatchID)
		argIdx++
	}
	if filter.StartDate != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filter.StartDate)
		argIdx++
	}
	if filter.EndDate != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, *filter.EndDate)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM notifications %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}

	offset := (filter.Page - 1) * filter.Limit
	dataQuery := fmt.Sprintf(
		`SELECT id, batch_id, recipient, channel, content, priority, status, error_message, retry_count, created_at, updated_at
		 FROM notifications %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		whereClause, argIdx, argIdx+1,
	)
	args = append(args, filter.Limit, offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var notifications []*domain.Notification
	for rows.Next() {
		var n domain.Notification
		var channel, priority, status string
		if err := rows.Scan(&n.ID, &n.BatchID, &n.Recipient, &channel, &n.Content, &priority, &status, &n.ErrorMessage, &n.RetryCount, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan notification: %w", err)
		}
		n.Channel = domain.Channel(channel)
		n.Priority = domain.Priority(priority)
		n.Status = domain.NotificationStatus(status)
		notifications = append(notifications, &n)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate notifications: %w", err)
	}

	return notifications, total, nil
}

func (r *PostgresRepository) CancelNotification(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE notifications SET status = 'cancelled', updated_at = NOW() WHERE id = $1 AND status = 'pending'`,
		id,
	)
	if err != nil {
		return fmt.Errorf("cancel notification: %w", err)
	}

	if tag.RowsAffected() == 0 {
		var status string
		err := r.pool.QueryRow(ctx, `SELECT status FROM notifications WHERE id = $1`, id).Scan(&status)
		if err != nil {
			return &domain.ErrNotFound{Resource: "notification", ID: id.String()}
		}
		return &domain.ErrNotCancellable{ID: id.String(), Status: status}
	}

	return nil
}

func (r *PostgresRepository) CountByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT status, COUNT(*) FROM notifications GROUP BY status`,
	)
	if err != nil {
		return nil, fmt.Errorf("count notifications by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan notification count: %w", err)
		}
		counts[status] = count
	}
	return counts, rows.Err()
}