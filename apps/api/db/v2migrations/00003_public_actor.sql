-- +goose Up
-- The portfolio runs as one shared demo workspace. Authentication and user
-- administration are deliberately outside this project scope.
INSERT INTO v2.users(id, username, password_hash, active)
VALUES ('00000000-0000-0000-0000-000000000002', 'portfolio', 'disabled', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO v2.user_roles(user_id, role)
VALUES
  ('00000000-0000-0000-0000-000000000002', 'admin'),
  ('00000000-0000-0000-0000-000000000002', 'planner'),
  ('00000000-0000-0000-0000-000000000002', 'materials'),
  ('00000000-0000-0000-0000-000000000002', 'operator'),
  ('00000000-0000-0000-0000-000000000002', 'quality')
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM v2.user_roles WHERE user_id = '00000000-0000-0000-0000-000000000002';
DELETE FROM v2.users WHERE id = '00000000-0000-0000-0000-000000000002';
