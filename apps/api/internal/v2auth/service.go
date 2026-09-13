package v2auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"time"

	"example.com/assembly-erp/api/internal/domain"
	"example.com/assembly-erp/api/internal/v2authdb"
	"example.com/assembly-erp/api/internal/v2identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const sessionLifetime = 12 * time.Hour
const passwordCost = 12

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,63}$`)
var allowedRoles = []string{"admin", "planner", "materials", "operator", "quality"}

type service struct {
	pool      *pgxpool.Pool
	dummyHash []byte
}

type user struct {
	v2identity.Actor
	Active bool `json:"active"`
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type accountInput struct {
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	Roles    []string `json:"roles"`
	Active   *bool    `json:"active,omitempty"`
}

type loginResult struct {
	User      v2identity.Actor `json:"user"`
	CSRFToken string           `json:"csrf_token"`
}

type session struct {
	Actor     v2identity.Actor
	Hash      []byte
	CSRFToken string
	ExpiresAt time.Time
}

func unauthorized() error {
	return &domain.Error{Status: 401, Code: "UNAUTHENTICATED", Message: "로그인이 필요하거나 세션이 만료되었습니다."}
}

func forbidden() error {
	return &domain.Error{Status: 403, Code: "FORBIDDEN", Message: "이 작업을 수행할 권한이 없습니다."}
}

func passwordHash(password string) (string, error) {
	if len(password) < 12 || len(password) > 72 {
		return "", domain.Input("비밀번호는 UTF-8 기준 12~72바이트여야 합니다.")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	return string(hash), err
}

func randomToken() string {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(token[:])
}

func tokenHash(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

func (s *service) transaction(ctx context.Context, work func(*v2authdb.Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := work(v2authdb.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func audit(ctx context.Context, q *v2authdb.Queries, actor, target *uuid.UUID, action string, details any) error {
	data, err := json.Marshal(details)
	if err != nil {
		return err
	}
	return q.CreateAuthAudit(ctx, v2authdb.CreateAuthAuditParams{ID: uuid.New(), ActorID: actor, TargetID: target, Action: action, Details: data})
}

func loadSession(ctx context.Context, q *v2authdb.Queries, hash []byte) (session, error) {
	row, err := q.GetSession(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return session{}, unauthorized()
	}
	if err != nil {
		return session{}, err
	}
	roles, err := q.ListRoles(ctx, row.UserID)
	if err != nil {
		return session{}, err
	}
	if len(roles) == 0 {
		return session{}, forbidden()
	}
	return session{Actor: v2identity.Actor{ID: row.UserID, Username: row.Username, Roles: roles}, Hash: row.TokenHash, CSRFToken: row.CsrfToken, ExpiresAt: row.ExpiresAt}, nil
}

func (s *service) login(ctx context.Context, in credentials, oldToken string) (loginResult, string, error) {
	q := v2authdb.New(s.pool)
	row, lookupErr := q.GetUserByUsername(ctx, in.Username)
	if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
		return loginResult{}, "", lookupErr
	}
	hash := s.dummyHash
	if lookupErr == nil {
		hash = []byte(row.PasswordHash)
	}
	match := bcrypt.CompareHashAndPassword(hash, []byte(in.Password)) == nil
	if lookupErr != nil || !match || !row.Active {
		if err := audit(ctx, q, nil, nil, "login_failed", map[string]string{"username": in.Username}); err != nil {
			return loginResult{}, "", err
		}
		return loginResult{}, "", unauthorized()
	}
	token, csrf := randomToken(), randomToken()
	var result loginResult
	err := s.transaction(ctx, func(q *v2authdb.Queries) error {
		locked, err := q.LockUser(ctx, row.ID)
		if err != nil {
			return err
		}
		if !locked.Active || locked.PasswordHash != row.PasswordHash {
			return unauthorized()
		}
		roles, err := q.ListRoles(ctx, row.ID)
		if err != nil {
			return err
		}
		if len(roles) == 0 {
			return forbidden()
		}
		if oldToken != "" {
			if err := q.RevokeSession(ctx, tokenHash(oldToken)); err != nil {
				return err
			}
		}
		if err := q.CreateSession(ctx, v2authdb.CreateSessionParams{TokenHash: tokenHash(token), UserID: row.ID, CsrfToken: csrf, ExpiresAt: time.Now().UTC().Add(sessionLifetime)}); err != nil {
			return err
		}
		if err := audit(ctx, q, &row.ID, &row.ID, "login", map[string]any{}); err != nil {
			return err
		}
		result = loginResult{User: v2identity.Actor{ID: row.ID, Username: row.Username, Roles: roles}, CSRFToken: csrf}
		return nil
	})
	return result, token, err
}

func requireAdmin(ctx context.Context, q *v2authdb.Queries, current session) error {
	// Serialize account mutations, then lock the actor before inspecting live privileges.
	if err := q.LockAccountAdministration(ctx); err != nil {
		return err
	}
	row, err := q.LockUser(ctx, current.Actor.ID)
	if err != nil {
		return err
	}
	if !row.Active {
		return unauthorized()
	}
	live, err := loadSession(ctx, q, current.Hash)
	if err != nil {
		return err
	}
	if live.Actor.ID != current.Actor.ID || !slices.Contains(live.Actor.Roles, "admin") {
		return forbidden()
	}
	return nil
}

func validateAccount(in *accountInput, create bool) error {
	if create && !usernamePattern.MatchString(in.Username) {
		return domain.Input("사용자 이름은 영문·숫자로 시작하는 3~64자의 영문·숫자·점·밑줄·하이픈이어야 합니다.")
	}
	if !create && (in.Username != "" || in.Active == nil) {
		return domain.Input("계정 변경에는 active와 roles를 지정하며 사용자 이름은 변경할 수 없습니다.")
	}
	if len(in.Roles) == 0 || len(in.Roles) > len(allowedRoles) {
		return domain.Input("하나 이상의 유효한 역할을 지정해 주세요.")
	}
	slices.Sort(in.Roles)
	for i, role := range in.Roles {
		if !slices.Contains(allowedRoles, role) || (i > 0 && role == in.Roles[i-1]) {
			return domain.Input("역할이 올바르지 않거나 중복되었습니다.")
		}
	}
	return nil
}

func (s *service) account(ctx context.Context, current session, key, target uuid.UUID, in accountInput) (json.RawMessage, bool, error) {
	create := target == uuid.Nil
	if err := validateAccount(&in, create); err != nil {
		return nil, false, err
	}
	var result json.RawMessage
	var replay bool
	err := s.transaction(ctx, func(q *v2authdb.Queries) error {
		if err := requireAdmin(ctx, q, current); err != nil {
			return err
		}
		operation := "create"
		if !create {
			operation = "update:" + target.String()
		}
		// Passwords never enter a fast digest: replay verifies the original salted bcrypt hash.
		canonical := in
		canonical.Password = ""
		encoded, err := json.Marshal(canonical)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(encoded)
		previous, err := q.GetAccountRequest(ctx, v2authdb.GetAccountRequestParams{ActorID: current.Actor.ID, Operation: operation, Key: key})
		if err == nil {
			passwordMatches := previous.PasswordHash == "" && in.Password == ""
			if previous.PasswordHash != "" {
				passwordMatches = bcrypt.CompareHashAndPassword([]byte(previous.PasswordHash), []byte(in.Password)) == nil
			}
			if !bytes.Equal(digest[:], previous.RequestHash) || !passwordMatches {
				return domain.Conflict("IDEMPOTENCY_CONFLICT", "같은 요청 키를 다른 입력에 사용할 수 없습니다.")
			}
			result, replay = previous.Response, true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		password := ""
		if create || in.Password != "" {
			password, err = passwordHash(in.Password)
			if err != nil {
				return err
			}
		}
		active := true
		username := in.Username
		if create {
			target = uuid.New()
			if in.Active != nil && !*in.Active {
				return domain.Input("새 계정은 활성 상태로 생성됩니다.")
			}
			if err := q.CreateUser(ctx, v2authdb.CreateUserParams{ID: target, Username: username, PasswordHash: password}); err != nil {
				return err
			}
		} else {
			row, err := q.LockUser(ctx, target)
			if err != nil {
				return err
			}
			active, username = *in.Active, row.Username
			oldRoles, err := q.ListRoles(ctx, target)
			if err != nil {
				return err
			}
			if row.Active && slices.Contains(oldRoles, "admin") && (!active || !slices.Contains(in.Roles, "admin")) {
				count, err := q.CountActiveAdmins(ctx)
				if err != nil {
					return err
				}
				if count <= 1 {
					return domain.Conflict("LAST_ADMIN", "마지막 활성 관리자의 권한을 제거할 수 없습니다.")
				}
			}
			if err := q.UpdateUser(ctx, v2authdb.UpdateUserParams{ID: target, Active: active, PasswordHash: password}); err != nil {
				return err
			}
			if err := q.DeleteRoles(ctx, target); err != nil {
				return err
			}
			if err := q.RevokeUserSessions(ctx, target); err != nil {
				return err
			}
		}
		for _, role := range in.Roles {
			if err := q.AddRole(ctx, v2authdb.AddRoleParams{UserID: target, Role: role}); err != nil {
				return err
			}
		}
		if err := audit(ctx, q, &current.Actor.ID, &target, "account_"+operation, map[string]any{"roles": in.Roles, "active": active, "password_changed": password != ""}); err != nil {
			return err
		}
		result, err = json.Marshal(user{Actor: v2identity.Actor{ID: target, Username: username, Roles: in.Roles}, Active: active})
		if err != nil {
			return err
		}
		return q.SaveAccountRequest(ctx, v2authdb.SaveAccountRequestParams{ActorID: current.Actor.ID, Operation: operation, Key: key, RequestHash: digest[:], PasswordHash: password, Response: result})
	})
	return result, replay, err
}

// BootstrapAdmin creates one explicitly supplied administrator; it never resets or promotes an existing account.
func BootstrapAdmin(ctx context.Context, pool *pgxpool.Pool, username, password string) (v2identity.Actor, error) {
	in := accountInput{Username: username, Roles: []string{"admin"}}
	if err := validateAccount(&in, true); err != nil {
		return v2identity.Actor{}, err
	}
	hash, err := passwordHash(password)
	if err != nil {
		return v2identity.Actor{}, err
	}
	actor := v2identity.Actor{ID: uuid.New(), Username: username, Roles: in.Roles}
	s := &service{pool: pool}
	err = s.transaction(ctx, func(q *v2authdb.Queries) error {
		if err := q.LockAccountAdministration(ctx); err != nil {
			return err
		}
		if err := q.CreateUser(ctx, v2authdb.CreateUserParams{ID: actor.ID, Username: actor.Username, PasswordHash: hash}); err != nil {
			return err
		}
		if err := q.AddRole(ctx, v2authdb.AddRoleParams{UserID: actor.ID, Role: "admin"}); err != nil {
			return err
		}
		return audit(ctx, q, &actor.ID, &actor.ID, "admin_bootstrap", map[string]any{})
	})
	return actor, err
}
