package v2auth

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"

	"example.com/assembly-erp/api/internal/domain"
	"example.com/assembly-erp/api/internal/v2authdb"
	"example.com/assembly-erp/api/internal/v2identity"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const actorKey = "v2_actor"
const csrfKey = "v2_csrf"
const dummyBcryptHash = "$2a$12$7EqJtq98hPqEX7fNZaFWoO6t3Q1nCzJ6Q4mWj7QY1n7nqvK4xjQ8G"

type handler struct {
	s      *service
	secure bool
}

// Register installs only v2 auth endpoints and returns middleware for the
// factory group. It never applies v2 auth to anonymous v1 routes.
func Register(app *fiber.App, pool *pgxpool.Pool, secure bool) fiber.Handler {
	h := &handler{s: &service{pool: pool, dummyHash: []byte(dummyBcryptHash)}, secure: secure}
	app.Post("/api/v2/auth/login", h.login)
	app.Get("/api/v2/auth/health", h.health)
	app.Get("/api/v2/auth/me", h.sessionRequired(h.me))
	app.Post("/api/v2/auth/logout", h.sessionRequired(h.logout))
	app.Get("/api/v2/users", h.sessionRequired(h.list))
	app.Post("/api/v2/users", h.sessionRequired(h.create))
	app.Put("/api/v2/users/:id", h.sessionRequired(h.update))
	return h.load
}

func cookieOpts(secure bool) fiber.Cookie {
	return fiber.Cookie{Name: "v2_auth", Path: "/", HTTPOnly: true, SameSite: "Strict", Secure: secure, MaxAge: int(sessionLifetime.Seconds())}
}

func publicError(err error) (int, any) {
	if err == nil {
		return 200, nil
	}
	var de *domain.Error
	if errors.As(err, &de) {
		return de.Status, fiber.Map{"error": de}
	}
	return 500, fiber.Map{"error": domain.PublicError(err)}
}

func (h *handler) load(c fiber.Ctx) error {
	cookie := c.Cookies("v2_auth")
	if cookie == "" {
		return c.Next()
	}
	sess, err := loadSession(c.Context(), v2authdb.New(h.s.pool), tokenHash(cookie))
	if err != nil {
		c.ClearCookie("v2_auth")
		return c.Next()
	}
	c.Locals(actorKey, sess.Actor)
	c.Locals(csrfKey, sess.CSRFToken)
	if err := sameOriginCheck(c); err != nil {
		status, body := publicError(err)
		return c.Status(status).JSON(body)
	}
	return c.Next()
}

func (h *handler) loadSession(c fiber.Ctx) (session, error) {
	cookie := c.Cookies("v2_auth")
	if cookie == "" {
		return session{}, unauthorized()
	}
	sess, err := loadSession(c.Context(), v2authdb.New(h.s.pool), tokenHash(cookie))
	if err != nil {
		return session{}, err
	}
	c.Locals(actorKey, sess.Actor)
	c.Locals(csrfKey, sess.CSRFToken)
	return sess, nil
}

func (h *handler) sessionRequired(next fiber.Handler) fiber.Handler {
	return func(c fiber.Ctx) error {
		if _, err := h.loadSession(c); err != nil {
			status, body := publicError(err)
			return c.Status(status).JSON(body)
		}
		if err := sameOriginCheck(c); err != nil {
			status, body := publicError(err)
			return c.Status(status).JSON(body)
		}
		return next(c)
	}
}

func CurrentActor(c fiber.Ctx) (v2identity.Actor, error) {
	actor, ok := c.Locals(actorKey).(v2identity.Actor)
	if !ok || actor.ID == uuid.Nil {
		return v2identity.Actor{}, unauthorized()
	}
	return actor, nil
}

func sameOriginCheck(c fiber.Ctx) error {
	if c.Method() == "GET" || c.Method() == "HEAD" {
		return nil
	}
	host := string(c.Request().Host())
	if origin := c.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host != host || u.User != nil {
			return domain.Input("동일한 애플리케이션 출처의 요청만 허용합니다.")
		}
	}
	if c.Get("Sec-Fetch-Site") == "cross-site" {
		return domain.Input("다른 사이트의 쓰기 요청은 허용하지 않습니다.")
	}
	csrf, _ := c.Locals(csrfKey).(string)
	provided := c.Get("X-CSRF-Token")
	if csrf == "" || provided == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(provided)) != 1 {
		return domain.Input("CSRF 토큰이 없거나 일치하지 않습니다.")
	}
	return nil
}

func (h *handler) me(c fiber.Ctx) error {
	actor, err := CurrentActor(c)
	if err != nil {
		status, body := publicError(err)
		return c.Status(status).JSON(body)
	}
	csrf, _ := c.Locals(csrfKey).(string)
	return c.Status(200).JSON(fiber.Map{"data": loginResult{User: actor, CSRFToken: csrf}})
}

func (h *handler) login(c fiber.Ctx) error {
	var in credentials
	if err := c.Bind().Body(&in); err != nil || strings.TrimSpace(in.Username) == "" || in.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": domain.Input("아이디와 비밀번호를 모두 입력해 주세요.")})
	}
	res, token, err := h.s.login(c.Context(), in, c.Cookies("v2_auth"))
	if err != nil {
		status, body := publicError(err)
		return c.Status(status).JSON(body)
	}
	c.Cookie(&fiber.Cookie{Name: "v2_auth", Value: token, Path: "/", HTTPOnly: true, SameSite: "Strict", Secure: h.secure, MaxAge: int(sessionLifetime.Seconds())})
	return c.Status(200).JSON(fiber.Map{"data": res})
}

func (h *handler) logout(c fiber.Ctx) error {
	if token := c.Cookies("v2_auth"); token != "" {
		if err := v2authdb.New(h.s.pool).RevokeSession(c.Context(), tokenHash(token)); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": domain.PublicError(err)})
		}
	}
	c.ClearCookie("v2_auth")
	return c.Status(200).JSON(fiber.Map{"data": true})
}

func (h *handler) health(c fiber.Ctx) error { return c.Status(200).JSON(fiber.Map{"data": "ok"}) }

func (h *handler) list(c fiber.Ctx) error {
	actor, err := CurrentActor(c)
	if err != nil {
		status, body := publicError(err)
		return c.Status(status).JSON(body)
	}
	if !slices.Contains(actor.Roles, "admin") {
		return c.Status(403).JSON(fiber.Map{"error": forbidden()})
	}
	q := v2authdb.New(h.s.pool)
	rows, err := q.ListUsers(c.Context())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": domain.PublicError(err)})
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		roles, err := q.ListRoles(c.Context(), row.ID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": domain.PublicError(err)})
		}
		result = append(result, map[string]any{"id": row.ID, "username": row.Username, "active": row.Active, "roles": roles})
	}
	return c.Status(200).JSON(fiber.Map{"data": result})
}

func (h *handler) create(c fiber.Ctx) error {
	actor, err := CurrentActor(c)
	if err != nil {
		status, body := publicError(err)
		return c.Status(status).JSON(body)
	}
	if !slices.Contains(actor.Roles, "admin") {
		return c.Status(403).JSON(fiber.Map{"error": forbidden()})
	}
	var payload accountInput
	if err := c.Bind().Body(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": domain.Input("요청 형식이 올바르지 않습니다.")})
	}
	key, err := commandKey(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err})
	}
	current, err := h.loadSession(c)
	if err != nil {
		status, body := publicError(err)
		return c.Status(status).JSON(body)
	}
	data, replay, err := h.s.account(c.Context(), current, key, uuid.Nil, payload)
	if err != nil {
		status, body := publicError(err)
		return c.Status(status).JSON(body)
	}
	status := 201
	if replay {
		status = 200
	}
	return c.Status(status).JSON(fiber.Map{"data": json.RawMessage(data)})
}

func (h *handler) update(c fiber.Ctx) error {
	actor, err := CurrentActor(c)
	if err != nil {
		status, body := publicError(err)
		return c.Status(status).JSON(body)
	}
	if !slices.Contains(actor.Roles, "admin") {
		return c.Status(403).JSON(fiber.Map{"error": forbidden()})
	}
	target, err := uuid.Parse(c.Params("id"))
	if err != nil || target == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": domain.Input("잘못된 사용자 ID입니다.")})
	}
	var payload accountInput
	if err := c.Bind().Body(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": domain.Input("요청 형식이 올바르지 않습니다.")})
	}
	key, err := commandKey(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err})
	}
	current, err := h.loadSession(c)
	if err != nil {
		status, body := publicError(err)
		return c.Status(status).JSON(body)
	}
	data, replay, err := h.s.account(c.Context(), current, key, target, payload)
	if err != nil {
		status, body := publicError(err)
		return c.Status(status).JSON(body)
	}
	_ = replay
	return c.Status(200).JSON(fiber.Map{"data": json.RawMessage(data)})
}

func commandKey(c fiber.Ctx) (uuid.UUID, error) {
	key, err := uuid.Parse(c.Get("Idempotency-Key"))
	if err != nil || key == uuid.Nil {
		return uuid.Nil, domain.Input("Idempotency-Key UUID가 필요합니다.")
	}
	return key, nil
}
