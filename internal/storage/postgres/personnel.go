package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"rms/internal/domain/models"
	"rms/internal/storage"
)

func personnelError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return storage.ErrNotFound
	}
	if isErrorPg(err, ERR_ForeignKeyViolation) || isErrorPg(err, ERR_UniqueViolation) {
		return storage.ErrConflict
	}
	if isErrorPg(err, "23514") || isErrorPg(err, "22001") {
		return storage.ErrInvalid
	}
	return err
}
func affected(result sql.Result, err error) error {
	if err != nil {
		return personnelError(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return storage.ErrNotFound
	}
	return nil
}
func (s *Storage) Roles(ctx context.Context, page models.UserPage) ([]models.Role, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name FROM roles ORDER BY id LIMIT $1 OFFSET $2", page.Limit, page.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Role{}
	for rows.Next() {
		var v models.Role
		if err = rows.Scan(&v.ID, &v.Name); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Storage) Role(ctx context.Context, id int64) (models.Role, error) {
	var v models.Role
	err := s.db.QueryRowContext(ctx, "SELECT id,name FROM roles WHERE id=$1", id).Scan(&v.ID, &v.Name)
	return v, personnelError(err)
}
func (s *Storage) AddRole(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, "INSERT INTO roles(name) VALUES($1) RETURNING id", name).Scan(&id)
	return id, personnelError(err)
}
func (s *Storage) SaveRole(ctx context.Context, id int64, name string) error {
	return affected(s.db.ExecContext(ctx, "UPDATE roles SET name=$2 WHERE id=$1", id, name))
}
func (s *Storage) DeleteRole(ctx context.Context, id int64) error {
	return affected(s.db.ExecContext(ctx, "DELETE FROM roles WHERE id=$1", id))
}
func (s *Storage) Positions(ctx context.Context, page models.UserPage) ([]models.Position, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.id,p.name,p.seller_selection_available,p.store_selection_available,p.store_selection_required,COALESCE(json_agg(json_build_object('id',r.id,'name',r.name) ORDER BY r.id) FILTER(WHERE r.id IS NOT NULL),'[]') FROM positions p LEFT JOIN position_roles pr ON pr.position_id=p.id LEFT JOIN roles r ON r.id=pr.role_id GROUP BY p.id ORDER BY p.id LIMIT $1 OFFSET $2`, page.Limit, page.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Position{}
	for rows.Next() {
		var v models.Position
		var data []byte
		if err = rows.Scan(&v.ID, &v.Name, &v.SellerSelectionAvailable, &v.StoreSelectionAvailable, &v.StoreSelectionRequired, &data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &v.Roles); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Storage) Position(ctx context.Context, id int64) (models.Position, error) {
	var v models.Position
	err := s.db.QueryRowContext(ctx, "SELECT id,name,seller_selection_available,store_selection_available,store_selection_required FROM positions WHERE id=$1", id).Scan(&v.ID, &v.Name, &v.SellerSelectionAvailable, &v.StoreSelectionAvailable, &v.StoreSelectionRequired)
	if err != nil {
		return v, personnelError(err)
	}
	v.Roles, err = s.positionRoles(ctx, id)
	return v, err
}
func (s *Storage) positionRoles(ctx context.Context, id int64) ([]models.Role, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT r.id,r.name FROM roles r JOIN position_roles pr ON pr.role_id=r.id WHERE pr.position_id=$1 ORDER BY r.id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Role{}
	for rows.Next() {
		var v models.Role
		if err = rows.Scan(&v.ID, &v.Name); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Storage) AddPosition(ctx context.Context, in models.PositionInput) (int64, error) {
	return s.writePosition(ctx, 0, in)
}
func (s *Storage) SavePosition(ctx context.Context, id int64, in models.PositionInput) error {
	_, err := s.writePosition(ctx, id, in)
	return err
}
func (s *Storage) writePosition(ctx context.Context, id int64, in models.PositionInput) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if id == 0 {
		err = tx.QueryRowContext(ctx, "INSERT INTO positions(name,store_selection_available,store_selection_required,seller_selection_available) VALUES($1,$2,$3,$4) RETURNING id", in.Name, in.StoreSelectionAvailable, in.StoreSelectionRequired, in.SellerSelectionAvailable).Scan(&id)
	} else {
		err = affected(tx.ExecContext(ctx, "UPDATE positions SET name=$2,store_selection_available=$3,store_selection_required=$4,seller_selection_available=$5 WHERE id=$1", id, in.Name, in.StoreSelectionAvailable, in.StoreSelectionRequired, in.SellerSelectionAvailable))
	}
	if err != nil {
		return 0, personnelError(err)
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM position_roles WHERE position_id=$1", id); err != nil {
		return 0, err
	}
	var adminID int64
	for _, role := range in.RoleIDs {
		var name string
		if err = tx.QueryRowContext(ctx, "SELECT name FROM roles WHERE id=$1", role).Scan(&name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, storage.ErrConflict
			}
			return 0, personnelError(err)
		}
		if name == "admin" {
			adminID = role
		}
	}
	if adminID != 0 {
		in.RoleIDs = []int64{adminID}
	}
	for _, role := range in.RoleIDs {
		if _, err = tx.ExecContext(ctx, "INSERT INTO position_roles(position_id,role_id) VALUES($1,$2)", id, role); err != nil {
			return 0, personnelError(err)
		}
	}
	return id, personnelError(tx.Commit())
}
func (s *Storage) DeletePosition(ctx context.Context, id int64) error {
	return affected(s.db.ExecContext(ctx, "DELETE FROM positions WHERE id=$1", id))
}
func (s *Storage) UserByID(ctx context.Context, id int64) (models.UserDetails, error) {
	var v models.UserDetails
	err := s.db.QueryRowContext(ctx, "SELECT u.id,u.name,u.user_token,u.is_admin,u.position,p.name,COALESCE(p.seller_selection_available,FALSE),COALESCE(p.store_selection_available,FALSE),COALESCE(p.store_selection_required,FALSE),u.login_blocked_until FROM users u LEFT JOIN positions p ON p.id=u.position WHERE u.id=$1", id).Scan(&v.ID, &v.Name, &v.UserToken, &v.IsAdmin, &v.Position, &v.PositionName, &v.SellerSelectionAvailable, &v.StoreSelectionAvailable, &v.StoreSelectionRequired, &v.LoginBlockedUntil)
	if err != nil {
		return v, personnelError(err)
	}
	v.Roles = []models.Role{}
	if v.Position != nil {
		v.Roles, err = s.positionRoles(ctx, *v.Position)
	}
	return v, err
}
func (s *Storage) AddUserWithPosition(ctx context.Context, in models.UserInput) (int64, error) {
	var err error
	in, err = s.normalizeUserPosition(ctx, in)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, "INSERT INTO users(name,user_token,password,is_admin,position,login_blocked_until) VALUES($1,$2,$3,$4,$5,$6) RETURNING id", in.Name, in.UserToken, in.Password, in.IsAdmin, in.Position, in.LoginBlockedUntil).Scan(&id)
	return id, personnelError(err)
}
func (s *Storage) SaveUser(ctx context.Context, id int64, in models.UserInput) error {
	var err error
	in, err = s.normalizeUserPosition(ctx, in)
	if err != nil {
		return err
	}
	return affected(s.db.ExecContext(ctx, "UPDATE users SET name=$2,user_token=$3,password=CASE WHEN $4='' THEN password ELSE $4 END,is_admin=$5,position=$6,login_blocked_until=$7,updated_at=NOW() WHERE id=$1", id, in.Name, in.UserToken, in.Password, in.IsAdmin, in.Position, in.LoginBlockedUntil))
}
func (s *Storage) DeleteUser(ctx context.Context, id int64) error {
	return affected(s.db.ExecContext(ctx, "DELETE FROM users WHERE id=$1", id))
}

func (s *Storage) normalizeUserPosition(ctx context.Context, in models.UserInput) (models.UserInput, error) {
	if in.IsAdmin {
		var id int64
		err := s.db.QueryRowContext(ctx, "SELECT id FROM positions WHERE name='admin'").Scan(&id)
		if err != nil {
			return in, personnelError(err)
		}
		in.Position = &id
	} else if in.Position != nil {
		var name string
		err := s.db.QueryRowContext(ctx, "SELECT name FROM positions WHERE id=$1", *in.Position).Scan(&name)
		if err != nil {
			return in, personnelError(err)
		}
		in.IsAdmin = name == "admin"
	}
	return in, nil
}
