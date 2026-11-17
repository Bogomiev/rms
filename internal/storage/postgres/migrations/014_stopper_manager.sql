-- Users with the manager role manage stoppers on a separate page; they log in without a store.
INSERT INTO roles(name) SELECT 'manager' WHERE NOT EXISTS(SELECT 1 FROM roles WHERE name='manager');
INSERT INTO positions(name) SELECT 'manager' WHERE NOT EXISTS(SELECT 1 FROM positions WHERE name='manager');
INSERT INTO position_roles(position_id,role_id)
SELECT p.id,r.id FROM positions p JOIN roles r ON r.name='manager' WHERE p.name='manager'
AND NOT EXISTS(SELECT 1 FROM position_roles pr WHERE pr.position_id=p.id AND pr.role_id=r.id);
