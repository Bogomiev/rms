package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"rms/internal/domain/models"
	"rms/internal/services"
	"rms/internal/services/returnrequest"
)

const messageColumns = `id, queue_name, task_handler, payload, status, priority, attempts, max_attempts, available_at, error_message, created_at`
const readyMessage = `status = 0 AND attempts < max_attempts AND available_at <= NOW()`

func scanMessage(row interface{ Scan(...any) error }) (models.Message, error) {
	var m models.Message
	var payload []byte
	err := row.Scan(&m.ID, &m.QueueName, &m.Handler, &payload, &m.Status, &m.Priority, &m.Attempts, &m.MaxAttempts, &m.AvailableAt, &m.ErrorMessage, &m.CreatedAt)
	m.Payload = payload
	if errors.Is(err, sql.ErrNoRows) {
		err = services.ErrNotFound
	}
	return m, err
}

func (s *Storage) AddMessage(ctx context.Context, m models.Message) (models.Message, error) {
	return addMessage(ctx, s.db, m)
}

func addMessage(ctx context.Context, q returnQueries, m models.Message) (models.Message, error) {
	var available any
	if !m.AvailableAt.IsZero() {
		available = m.AvailableAt
	}
	message, err := scanMessage(q.QueryRowContext(ctx, `INSERT INTO message_queue
        (queue_name, task_handler, payload, status, priority, attempts, max_attempts, available_at, error_message)
        VALUES (COALESCE(NULLIF($1,''),'default'), $2, $3, $4, $5, $6, COALESCE(NULLIF($7,0),10), COALESCE($8,NOW()), $9)
        RETURNING `+messageColumns, m.QueueName, m.Handler, string(m.Payload), m.Status, m.Priority, m.Attempts, m.MaxAttempts, available, m.ErrorMessage))
	if isErrorPg(err, ERR_UniqueViolation) {
		return message, services.ErrConflict
	}
	return message, err
}

func (s *Storage) SaveMessage(ctx context.Context, m models.Message) (models.Message, error) {
	return scanMessage(s.db.QueryRowContext(ctx, `UPDATE message_queue SET
        queue_name=$2, task_handler=$3, payload=$4, status=$5, priority=$6, attempts=$7, max_attempts=$8, available_at=$9, error_message=$10
        WHERE id=$1 RETURNING `+messageColumns, m.ID, m.QueueName, m.Handler, string(m.Payload), m.Status, m.Priority, m.Attempts, m.MaxAttempts, m.AvailableAt, m.ErrorMessage))
}

func (s *Storage) PendingMessages(ctx context.Context, queue string) ([]models.Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+messageColumns+` FROM message_queue
        WHERE queue_name = COALESCE(NULLIF($1,''),'default') AND `+readyMessage+` ORDER BY priority DESC, id`, queue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []models.Message{}
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (s *Storage) Message(ctx context.Context, id int64) (models.Message, error) {
	return scanMessage(s.db.QueryRowContext(ctx, `SELECT `+messageColumns+` FROM message_queue WHERE id=$1`, id))
}

func (s *Storage) SetMessageStatus(ctx context.Context, id int64, status models.MessageStatus) error {
	result, err := s.db.ExecContext(ctx, `UPDATE message_queue SET status=$2 WHERE id=$1`, id, status)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return services.ErrNotFound
	}
	return err
}

func (s *Storage) ProcessReturnMessage(ctx context.Context, id int64, process func(context.Context, models.Message, returnrequest.Transaction) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Keep the row locked until the outcome is saved, even if the handler times out.
	tx, err := s.db.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	message, err := scanMessage(tx.QueryRowContext(ctx, `SELECT `+messageColumns+` FROM message_queue WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	if message.Status == models.MessageDone {
		return nil
	}
	if message.Status != models.MessagePending || message.Attempts >= message.MaxAttempts || message.AvailableAt.After(time.Now()) {
		if message.ErrorMessage != nil {
			return &returnrequest.OneCError{Err: errors.New(*message.ErrorMessage)}
		}
		return services.ErrConflict
	}

	if _, err := tx.ExecContext(ctx, `SAVEPOINT return_message`); err != nil {
		return err
	}
	processErr := process(ctx, message, returnTx{tx})
	outcomeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if processErr == nil {
		if err := setMessageOutcome(outcomeCtx, tx, id, models.MessageDone, nil); err != nil {
			return err
		}
		return tx.Commit()
	}
	if _, err := tx.ExecContext(outcomeCtx, `ROLLBACK TO SAVEPOINT return_message`); err != nil {
		return errors.Join(processErr, err)
	}
	status := models.MessagePending
	if message.Attempts+1 >= message.MaxAttempts {
		status = models.MessageFailed
	}
	if err := setMessageOutcome(outcomeCtx, tx, id, status, processErr); err != nil {
		return errors.Join(processErr, err)
	}
	if err := tx.Commit(); err != nil {
		return errors.Join(processErr, err)
	}
	return processErr
}

func setMessageOutcome(ctx context.Context, tx *sql.Tx, id int64, status models.MessageStatus, failure error) error {
	var message any
	if failure != nil {
		message = failure.Error()
	}
	_, err := tx.ExecContext(ctx, `UPDATE message_queue SET status=$2::smallint, error_message=$3::text,
        attempts=attempts+CASE WHEN $3::text IS NULL THEN 0 ELSE 1 END,
        available_at=CASE WHEN $2::smallint=0 THEN statement_timestamp()+INTERVAL '5 seconds' ELSE available_at END WHERE id=$1`, id, status, message)
	if err != nil {
		return fmt.Errorf("update queue message: %w", err)
	}
	return nil
}
