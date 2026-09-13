-- name: GetUserByUsername :one
SELECT id, username, password_hash, active FROM v2.users WHERE username = $1;

-- name: LockUser :one
SELECT id, username, password_hash, active FROM v2.users WHERE id = $1 FOR UPDATE;

-- name: GetUser :one
SELECT id, username, active FROM v2.users WHERE id = $1;

-- name: ListUsers :many
SELECT id, username, active FROM v2.users ORDER BY username;

-- name: ListRoles :many
SELECT role FROM v2.user_roles WHERE user_id = $1 ORDER BY role;

-- name: CreateUser :exec
INSERT INTO v2.users(id, username, password_hash, active) VALUES ($1, $2, $3, true);

-- name: UpdateUser :exec
UPDATE v2.users SET active = $2, password_hash = CASE WHEN sqlc.arg(password_hash)::text = '' THEN password_hash ELSE sqlc.arg(password_hash)::text END WHERE id = $1;

-- name: DeleteRoles :exec
DELETE FROM v2.user_roles WHERE user_id = $1;

-- name: AddRole :exec
INSERT INTO v2.user_roles(user_id, role) VALUES ($1, $2);

-- name: CountActiveAdmins :one
SELECT count(*) FROM v2.users u JOIN v2.user_roles r ON r.user_id = u.id WHERE u.active AND r.role = 'admin';

-- name: LockAccountAdministration :exec
SELECT pg_advisory_xact_lock(224027, 1);

-- name: CreateSession :exec
INSERT INTO v2.sessions(token_hash, user_id, csrf_token, expires_at) VALUES ($1, $2, $3, $4);

-- name: GetSession :one
SELECT s.token_hash, s.user_id, s.csrf_token, s.expires_at, u.username
FROM v2.sessions s JOIN v2.users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND NOT s.revoked AND s.expires_at > clock_timestamp() AND u.active;

-- name: RevokeSession :exec
UPDATE v2.sessions SET revoked = true WHERE token_hash = $1;

-- name: RevokeUserSessions :exec
UPDATE v2.sessions SET revoked = true WHERE user_id = $1 AND NOT revoked;

-- name: CreateAuthAudit :exec
INSERT INTO v2.auth_audit(id, actor_id, target_id, action, details) VALUES ($1, $2, $3, $4, $5);

-- name: ListAuthAudit :many
SELECT id, actor_id, target_id, action, details, created_at FROM v2.auth_audit ORDER BY created_at DESC, id DESC LIMIT 500;

-- name: GetAccountRequest :one
SELECT request_hash, password_hash, response FROM v2.account_requests WHERE actor_id = $1 AND operation = $2 AND key = $3;

-- name: SaveAccountRequest :exec
INSERT INTO v2.account_requests(actor_id, operation, key, request_hash, password_hash, response) VALUES ($1, $2, $3, $4, $5, $6);
