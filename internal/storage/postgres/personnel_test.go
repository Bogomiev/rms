package postgres

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"rms/internal/domain/models"
	"rms/internal/lib/password"
	"rms/internal/services"
	"rms/internal/services/position"
	"rms/internal/services/role"
	"rms/internal/services/user"
	"testing"
	"time"
)

func TestPersonnelLifecycle(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	roles := role.New(db)
	positions := position.New(db)
	users := user.New(slog.New(slog.NewTextHandler(io.Discard, nil)), db)
	// Free a permitted name before exercising role creation.
	if _, err := db.db.ExecContext(ctx, "DELETE FROM position_roles WHERE role_id=(SELECT id FROM roles WHERE name='orders'); DELETE FROM roles WHERE name='orders'"); err != nil {
		t.Fatal(err)
	}
	roleID, err := roles.AddRole(ctx, "orders")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = roles.AddRole(ctx, "orders"); !errors.Is(err, services.ErrConflict) {
		t.Fatal("duplicate role", err)
	}
	if err = roles.SaveRole(ctx, roleID, "orders"); err != nil {
		t.Fatal(err)
	}
	if err = roles.SaveRole(ctx, roleID, "products"); !errors.Is(err, services.ErrConflict) {
		t.Fatal("duplicate role rename", err)
	}
	posID, err := positions.AddPosition(ctx, models.PositionInput{Name: "Cashier", SellerSelectionAvailable: true, StoreSelectionAvailable: true, StoreSelectionRequired: true, RoleIDs: []int64{roleID}})
	if err != nil {
		t.Fatal(err)
	}
	otherID, err := positions.AddPosition(ctx, models.PositionInput{Name: "Manager", RoleIDs: []int64{roleID}})
	if err != nil {
		t.Fatal("shared role", err)
	}

	roleList, err := roles.RoleList(ctx, models.UserPage{Limit: 50})
	if err != nil || len(roleList) != 6 || roleList[len(roleList)-1].Name != "orders" {
		t.Fatal("roles list", roleList, err)
	}
	roleRecord, err := roles.RoleByID(ctx, roleID)
	if err != nil || roleRecord.Name != "orders" {
		t.Fatal("role record", roleRecord, err)
	}
	positionList, err := positions.PositionList(ctx, models.UserPage{Limit: 50})
	if err != nil || len(positionList) != 4 || len(positionList[2].Roles) != 1 || len(positionList[3].Roles) != 1 {
		t.Fatal("positions list", positionList, err)
	}
	if err = roles.DeleteRole(ctx, roleID); !errors.Is(err, services.ErrConflict) {
		t.Fatal("referenced role deleted", err)
	}
	if _, err = positions.AddPosition(ctx, models.PositionInput{Name: "Bad", RoleIDs: []int64{roleID, roleID}}); !errors.Is(err, services.ErrInvalid) {
		t.Fatal("duplicate mapping", err)
	}
	if err = positions.SavePosition(ctx, posID, models.PositionInput{Name: "Changed", RoleIDs: []int64{999999}}); !errors.Is(err, services.ErrConflict) {
		t.Fatal("missing role", err)
	}
	pos, err := positions.PositionByID(ctx, posID)
	if err != nil || pos.Name != "Cashier" || !pos.SellerSelectionAvailable || !pos.StoreSelectionAvailable || !pos.StoreSelectionRequired || len(pos.Roles) != 1 {
		t.Fatalf("rollback: %+v %v", pos, err)
	}
	in := models.UserInput{Name: "User", UserToken: "employee", Password: "password", Position: &posID}
	uid, err := users.AddUser(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := users.UserByID(ctx, uid)
	if err != nil || got.Password != "" || !got.SellerSelectionAvailable || !got.StoreSelectionAvailable || !got.StoreSelectionRequired || got.Position == nil || *got.Position != posID || got.PositionName == nil || *got.PositionName != "Cashier" || len(got.Roles) != 1 || got.Roles[0].ID != roleID {
		t.Fatalf("user details: %+v %v", got, err)
	}
	login, err := db.User(ctx, in.UserToken)
	if err != nil || !login.SellerSelectionAvailable || !login.StoreSelectionAvailable || !login.StoreSelectionRequired || login.PositionName == nil || *login.PositionName != "Cashier" {
		t.Fatal("login position", login, err)
	}
	if err = password.CheckPassword(in.Password, login.Password); err != nil {
		t.Fatal("unhashed password", err)
	}
	list, err := users.UserList(ctx, models.UserPage{Limit: 100})
	if err != nil || len(list) != 1 || list[0].Password != "" || list[0].Position == nil {
		t.Fatal("user list", list, err)
	}
	if err = positions.DeletePosition(ctx, posID); !errors.Is(err, services.ErrConflict) {
		t.Fatal("referenced position deleted", err)
	}
	in.Password = ""
	in.Name = "Updated"
	if err = users.SaveUser(ctx, uid, in); err != nil {
		t.Fatal(err)
	}
	after, err := db.User(ctx, in.UserToken)
	if err != nil || after.Password != login.Password || after.Name != "Updated" {
		t.Fatal("password preservation", after, err)
	}
	if _, err = db.CreateSession(ctx, &models.Session{ID: "personnel-session", UserID: uid, RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err = users.DeleteUser(ctx, uid); !errors.Is(err, services.ErrConflict) {
		t.Fatal("referenced user deleted", err)
	}
	if _, err = db.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id=$1", uid); err != nil {
		t.Fatal(err)
	}
	if err = users.DeleteUser(ctx, uid); err != nil {
		t.Fatal(err)
	}
	if _, err = users.UserByID(ctx, uid); !errors.Is(err, services.ErrNotFound) {
		t.Fatal("missing user", err)
	}
	for _, id := range []int64{posID, otherID} {
		if err = positions.SavePosition(ctx, id, models.PositionInput{Name: map[int64]string{posID: "Cashier", otherID: "Manager"}[id]}); err != nil {
			t.Fatal(err)
		}
		if err = positions.DeletePosition(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err = roles.DeleteRole(ctx, roleID); err != nil {
		t.Fatal(err)
	}
}

func TestMigration007SeedsExistingUsers(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	// Recreate the state just before 007 inside this test's isolated schema.
	_, err := db.db.ExecContext(ctx, `ALTER TABLE users DROP COLUMN position;
 DROP TABLE position_roles; DROP TABLE positions; DROP TABLE roles;
 DELETE FROM schema_migrations WHERE version='007_roles_positions.sql';
 INSERT INTO users(name,user_token,password,is_admin) VALUES('Admin','admin','hash',false),('Employee','employee','hash',true);`)
	if err != nil {
		t.Fatal(err)
	}
	if err = migrate(ctx, db.db); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token, position string
		admin           bool
		roles           int
	}{{"admin", "admin", true, 1}, {"employee", "seller", false, 5}} {
		u, err := db.User(ctx, tc.token)
		if err != nil {
			t.Fatal(err)
		}
		details, err := db.UserByID(ctx, u.ID)
		if err != nil || details.PositionName == nil || *details.PositionName != tc.position || details.IsAdmin != tc.admin || len(details.Roles) != tc.roles {
			t.Fatalf("%s: %+v %v", tc.token, details, err)
		}
		for _, r := range details.Roles {
			if (r.Name == "admin") != tc.admin {
				t.Fatal("incorrect role", tc.token, r)
			}
		}
	}
}

func TestPersonnelAdminAndPasswordUpdates(t *testing.T) {
	db := authTestStorage(t)
	ctx := context.Background()
	roles, _ := db.Roles(ctx, models.UserPage{Limit: 100})
	positions, _ := db.Positions(ctx, models.UserPage{Limit: 100})
	var adminPos, sellerPos, adminRole, orderRole int64
	for _, p := range positions {
		if p.Name == "admin" {
			adminPos = p.ID
		}
		if p.Name == "seller" {
			sellerPos = p.ID
		}
	}
	for _, r := range roles {
		if r.Name == "admin" {
			adminRole = r.ID
		}
		if r.Name == "order" {
			orderRole = r.ID
		}
	}
	if err := db.SavePosition(ctx, adminPos, models.PositionInput{Name: "admin", RoleIDs: []int64{adminRole, orderRole}}); err != nil {
		t.Fatal(err)
	}
	p, err := db.Position(ctx, adminPos)
	if err != nil || len(p.Roles) != 1 || p.Roles[0].Name != "admin" {
		t.Fatal("admin must have only admin role", p, err)
	}
	users := user.New(slog.New(slog.NewTextHandler(io.Discard, nil)), db)
	in := models.UserInput{Name: "User", UserToken: "test-admin", Password: "initial-password", IsAdmin: true, Position: &sellerPos}
	id, err := users.AddUser(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.UserByID(ctx, id)
	if err != nil || !u.IsAdmin || u.Position == nil || *u.Position != adminPos {
		t.Fatal("admin flag must select admin position", u, err)
	}
	in.IsAdmin = false
	in.Position = &adminPos
	in.Password = "changed-password"
	until := time.Now().Add(time.Hour).Truncate(time.Second)
	in.LoginBlockedUntil = &until
	if err = users.SaveUser(ctx, id, in); err != nil {
		t.Fatal(err)
	}
	u, err = users.UserByID(ctx, id)
	if err != nil || !u.IsAdmin || u.LoginBlockedUntil == nil || !u.LoginBlockedUntil.Equal(until) {
		t.Fatal("admin position and blocking", u, err)
	}
	stored, err := db.User(ctx, in.UserToken)
	if err != nil || password.CheckPassword("changed-password", stored.Password) != nil {
		t.Fatal("new password not hashed", err)
	}
	in.IsAdmin = false
	in.Position = &sellerPos
	in.Password = ""
	in.LoginBlockedUntil = nil
	if err = users.SaveUser(ctx, id, in); err != nil {
		t.Fatal(err)
	}
	u, _ = users.UserByID(ctx, id)
	if u.IsAdmin || u.LoginBlockedUntil != nil || len(u.Roles) != 5 {
		t.Fatal("seller or unblock", u)
	}
}
