-- +goose Up
CREATE SCHEMA IF NOT EXISTS v2;
REVOKE ALL ON SCHEMA v2 FROM PUBLIC;

CREATE TABLE v2.users (
    id uuid PRIMARY KEY,
    username text NOT NULL UNIQUE CHECK (username ~ '^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,63}$'),
    password_hash text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE v2.user_roles (
    user_id uuid NOT NULL REFERENCES v2.users(id),
    role text NOT NULL CHECK (role IN ('admin','planner','materials','operator','quality')),
    PRIMARY KEY (user_id, role)
);
-- Login sessions are not factory work sessions. Only a digest of the bearer token is stored.
CREATE TABLE v2.sessions (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id uuid NOT NULL REFERENCES v2.users(id),
    csrf_token text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked boolean NOT NULL DEFAULT false,
    CHECK (expires_at > created_at)
);
CREATE INDEX sessions_user_id_idx ON v2.sessions(user_id);
CREATE INDEX sessions_expiry_idx ON v2.sessions(expires_at);
CREATE TABLE v2.auth_audit (
    id uuid PRIMARY KEY,
    actor_id uuid REFERENCES v2.users(id),
    target_id uuid REFERENCES v2.users(id),
    action text NOT NULL,
    details jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
-- Durable identity is the account, never the login-session identifier.
CREATE TABLE v2.account_requests (
    actor_id uuid NOT NULL REFERENCES v2.users(id),
    operation text NOT NULL,
    key uuid NOT NULL,
    request_hash bytea NOT NULL,
    password_hash text NOT NULL,
    response jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, operation, key)
);

-- +goose Down
DROP TABLE v2.account_requests;
DROP TABLE v2.auth_audit;
DROP TABLE v2.sessions;
DROP TABLE v2.user_roles;
DROP TABLE v2.users;
DROP SCHEMA v2;
