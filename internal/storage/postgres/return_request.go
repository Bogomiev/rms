package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"rms/internal/domain/models"
	"rms/internal/services"
	"rms/internal/services/returnrequest"
)

type returnQueries interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type returnTx struct{ *sql.Tx }

const returnHeader = `r.id, r.date, r.number, r.manager, r.store_id, r.seller, r.status`
const returnCount = `(SELECT count(DISTINCT i.line) FROM return_request_items i WHERE i.return_request_id = r.id)`

func returnFields(r *models.ReturnRequest) []any {
	return []any{&r.ID, &r.Date, &r.Number, &r.Manager, &r.StoreID, &r.Seller, &r.Status}
}

func getReturn(ctx context.Context, q returnQueries, store, id uuid.UUID) (models.ReturnRequest, error) {
	var r models.ReturnRequest
	var items []byte
	err := q.QueryRowContext(ctx, `SELECT `+returnHeader+`,
        COALESCE((SELECT jsonb_agg(jsonb_build_object('line', i.line, 'product_id', i.product_id, 'amount', i.amount,
            'marks', i.marks) ORDER BY i.line)
            FROM (SELECT line, product_id, sum(amount) AS amount,
                jsonb_agg(mark ORDER BY mark) FILTER (WHERE mark IS NOT NULL) AS marks
                FROM return_request_items WHERE return_request_id = r.id
                GROUP BY line, product_id) i), '[]'::jsonb)
        FROM return_request r WHERE r.store_id = $1 AND r.id = $2`, store, id).Scan(append(returnFields(&r), &items)...)
	if errors.Is(err, sql.ErrNoRows) {
		return r, services.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(items, &r.Items)
	r.ItemCount = len(r.Items)
	return r, err
}

func (s *Storage) GetReturn(ctx context.Context, store, id uuid.UUID) (models.ReturnRequest, error) {
	return getReturn(ctx, s.db, store, id)
}

func (tx returnTx) GetReturn(ctx context.Context, store, id uuid.UUID) (models.ReturnRequest, error) {
	return getReturn(ctx, tx, store, id)
}

func (s *Storage) SyncReturnStatus(ctx context.Context, store, id uuid.UUID, previous, status models.ReturnStatus) error {
	if status != previous {
		result, err := s.db.ExecContext(ctx, `UPDATE return_request SET status=$4
			WHERE store_id=$1 AND id=$2 AND status=$3`, store, id, previous, status)
		if err != nil {
			return err
		}
		updated, err := result.RowsAffected()
		if err != nil || updated == 1 {
			return err
		}
	}
	var current models.ReturnStatus
	err := s.db.QueryRowContext(ctx, `SELECT status FROM return_request WHERE store_id=$1 AND id=$2`, store, id).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return services.ErrNotFound
	}
	if err != nil {
		return err
	}
	if current != status {
		return services.ErrConflict
	}
	return nil
}

func (s *Storage) ActiveReturnIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM return_request WHERE status NOT IN ($1, $2)`,
		models.ReturnCancelled, models.ReturnCompleted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Storage) SyncReturnStatuses(ctx context.Context, statuses map[uuid.UUID]models.ReturnStatus) error {
	if len(statuses) == 0 {
		return nil
	}
	payload, err := json.Marshal(statuses)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE return_request AS r SET status = result.value
		FROM jsonb_each_text($1::jsonb) AS result
		WHERE r.id = result.key::uuid AND r.status IS DISTINCT FROM result.value`, string(payload))
	return err
}

func (s *Storage) ListReturns(ctx context.Context, store uuid.UUID, from, to time.Time, history bool) ([]models.ReturnRequest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+returnHeader+`, `+returnCount+`
        FROM return_request r WHERE r.store_id = $1 AND r.date >= $2 AND r.date < $3
        AND ($4 OR r.status IN ('NEW', 'SENT', 'PROCESSED'))
        ORDER BY r.date DESC, r.id`, store, from, to, history)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []models.ReturnRequest{}
	for rows.Next() {
		var r models.ReturnRequest
		if err := rows.Scan(append(returnFields(&r), &r.ItemCount)...); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Storage) SubmitReturn(ctx context.Context, message models.Message, save func(returnrequest.Transaction) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := addMessage(ctx, tx, message); err != nil {
		return err
	}
	if err := save(returnTx{tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func (tx returnTx) ValidateReturn(ctx context.Context, r models.ReturnRequest) error {
	payload, err := json.Marshal(r.Items)
	if err != nil {
		return err
	}
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM stores WHERE id=$1)
		AND EXISTS(SELECT 1 FROM users WHERE id=$2)
		AND NOT EXISTS (
			SELECT 1 FROM jsonb_to_recordset($3::jsonb) AS item(product_id uuid, marks jsonb)
			LEFT JOIN products p ON p.id=item.product_id
			WHERE p.id IS NULL OR (p.marking_type NOT IN ('', 'БезОсобенностейУчета') AND COALESCE(jsonb_array_length(item.marks), 0)=0)
		)`, r.StoreID, r.Manager, string(payload)).Scan(&valid)
	if err == nil && !valid {
		return services.ErrInvalid
	}
	return err
}

func (tx returnTx) SaveReturn(ctx context.Context, r models.ReturnRequest, create bool) error {
	if create {
		if _, err := tx.ExecContext(ctx, `INSERT INTO return_request (id, date, number, manager, store_id, seller, status)
            VALUES ($1, $2, $3, $4, $5, $6, $7)`, r.ID, r.Date, r.Number, r.Manager, r.StoreID, r.Seller, r.Status); err != nil {
			if isErrorPg(err, ERR_UniqueViolation) {
				return services.ErrConflict
			}
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE return_request SET manager=$2, seller=$3 WHERE id=$1`,
			r.ID, r.Manager, r.Seller); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM return_request_items WHERE return_request_id=$1`, r.ID); err != nil {
			return err
		}
	}
	items, err := json.Marshal(r.Items)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO return_request_items (return_request_id, line, product_id, amount, mark)
        SELECT $1, item.line, item.product_id, CASE WHEN marks.mark IS NULL THEN item.amount ELSE 1 END, marks.mark
        FROM jsonb_to_recordset($2::jsonb) AS item(line smallint, product_id uuid, amount numeric, marks jsonb)
        LEFT JOIN LATERAL jsonb_array_elements_text(item.marks) AS marks(mark) ON true`, r.ID, string(items))
	return err
}

func (tx returnTx) SetReturnNumber(ctx context.Context, id uuid.UUID, number string) error {
	_, err := tx.ExecContext(ctx, `UPDATE return_request SET number=$2 WHERE id=$1`, id, number)
	return err
}

func (tx returnTx) SetReturnStatus(ctx context.Context, id uuid.UUID, status models.ReturnStatus) error {
	_, err := tx.ExecContext(ctx, `UPDATE return_request SET status=$2 WHERE id=$1`, id, status)
	return err
}
