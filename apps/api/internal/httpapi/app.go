package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"example.com/assembly-erp/api/internal/catalog"
	"example.com/assembly-erp/api/internal/domain"
	"example.com/assembly-erp/api/internal/inventory"
	"example.com/assembly-erp/api/internal/platform"
	"example.com/assembly-erp/api/internal/production"
	"example.com/assembly-erp/api/internal/store"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/google/uuid"
)

type endpoint func(context.Context, fiber.Ctx) (any, int, error)

func wrap(handler endpoint) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Fiber request contexts are not retained or treated as disconnect signals.
		ctx, cancel := context.WithTimeout(context.Background(), platform.OperationTimeout)
		defer cancel()
		value, status, err := handler(ctx, c)
		if err != nil {
			return err
		}
		return c.Status(status).JSON(value)
	}
}

func one(value any, status int, err error) (any, int, error) {
	return fiber.Map{"data": value}, status, err
}
func creationStatus(replay bool) int {
	if replay {
		return 200
	}
	return 201
}

func New(database *store.Store) *fiber.App {
	app := fiber.New(fiber.Config{
		Immutable: true, BodyLimit: 128 * 1024, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second,
		ErrorHandler: func(c fiber.Ctx, err error) error {
			var framework *fiber.Error
			if errors.As(err, &framework) {
				switch framework.Code {
				case 404:
					err = domain.Missing()
				case 400, 405, 413, 415, 422:
					err = domain.Input("요청 형식이나 크기가 올바르지 않습니다.")
				}
			}
			public := domain.PublicError(err)
			if public.Status == 503 {
				c.Set("Retry-After", "1")
			}
			return c.Status(public.Status).JSON(fiber.Map{"error": public})
		},
	})
	app.Use(recover.New())
	app.Use(func(c fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
		c.Set("X-Content-Type-Options", "nosniff")
		host := string(c.Request().Host())
		hostname, _, err := net.SplitHostPort(host)
		if err != nil {
			hostname = host
		}
			if !platform.RequestHostAllowed(hostname, os.Getenv("PUBLIC_APP_HOST")) {
				return domain.Input("허용된 애플리케이션 호스트에서만 접근할 수 있습니다.")
		}
		if c.Method() == "POST" || c.Method() == "PUT" {
			if origin := c.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host != host || u.User != nil {
						return domain.Input("동일한 애플리케이션 출처의 요청만 허용합니다.")
				}
			}
			if c.Get("Sec-Fetch-Site") == "cross-site" {
				return domain.Input("다른 사이트의 쓰기 요청은 허용하지 않습니다.")
			}
		}
		return c.Next()
	})
	items, stock, orders := catalog.New(database), inventory.New(database), production.New(database)
	api := app.Group("/api/v1")
	api.Get("/health", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		if err := database.Pool.Ping(ctx); err != nil {
			return nil, 503, domain.Unavailable()
		}
		return one(fiber.Map{"status": "ok"}, 200, nil)
	}))
	api.Post("/items", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		var in domain.ItemInput
		if err := body(c, &in, "code", "name", "kind", "unit"); err != nil {
			return nil, 400, err
		}
		value, err := items.Create(ctx, in)
		return one(value, 201, err)
	}))
	api.Get("/items", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		q, err := query(c)
		if err != nil {
			return nil, 400, err
		}
		value, err := items.List(ctx, q)
		return value, 200, err
	}))
	api.Get("/items/:id", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		id, err := pathID(c)
		if err != nil {
			return nil, 400, err
		}
		value, err := items.Get(ctx, id)
		return one(value, 200, err)
	}))
	api.Get("/items/:id/bom", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		id, err := pathID(c)
		if err != nil {
			return nil, 400, err
		}
		value, err := items.BOM(ctx, id)
		return one(value, 200, err)
	}))
	api.Put("/items/:id/bom", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		id, err := pathID(c)
		if err != nil {
			return nil, 400, err
		}
		var in domain.BOMInput
		if err = body(c, &in, "components"); err != nil {
			return nil, 400, err
		}
		value, err := items.ReplaceBOM(ctx, id, in)
		return one(value, 200, err)
	}))
	api.Post("/stock-receipts", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		var in domain.ReceiptInput
		if err := body(c, &in, "item_id", "quantity"); err != nil {
			return nil, 400, err
		}
		value, replay, err := stock.Receive(ctx, c.Get("Idempotency-Key"), in)
		return one(value, creationStatus(replay), err)
	}))
	api.Get("/stock-receipts/:id", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		id, err := pathID(c)
		if err != nil {
			return nil, 400, err
		}
		value, err := stock.Receipt(ctx, id)
		return one(value, 200, err)
	}))
	api.Get("/inventory", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		q, err := query(c)
		if err != nil {
			return nil, 400, err
		}
		value, err := stock.List(ctx, q)
		return value, 200, err
	}))
	api.Get("/stock-movements", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		q, err := query(c)
		if err != nil {
			return nil, 400, err
		}
		value, err := stock.Movements(ctx, q)
		return value, 200, err
	}))
	api.Post("/production-orders", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		var in domain.OrderInput
		if err := body(c, &in, "finished_item_id", "planned_quantity"); err != nil {
			return nil, 400, err
		}
		value, replay, err := orders.Create(ctx, c.Get("Idempotency-Key"), in)
		return one(value, creationStatus(replay), err)
	}))
	api.Get("/production-orders", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		q, err := query(c)
		if err != nil {
			return nil, 400, err
		}
		value, err := orders.List(ctx, q)
		return value, 200, err
	}))
	api.Get("/production-orders/:id", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		id, err := pathID(c)
		if err != nil {
			return nil, 400, err
		}
		value, err := orders.Get(ctx, id)
		return one(value, 200, err)
	}))
	api.Post("/production-orders/:id/results", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		id, err := pathID(c)
		if err != nil {
			return nil, 400, err
		}
		var in domain.ResultInput
		if err = body(c, &in, "good_quantity", "defective_quantity"); err != nil {
			return nil, 400, err
		}
		value, replay, err := orders.Record(ctx, id, c.Get("Idempotency-Key"), in)
		return one(value, creationStatus(replay), err)
	}))
	api.Get("/production-orders/:id/results", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		id, err := pathID(c)
		if err != nil {
			return nil, 400, err
		}
		q, err := query(c)
		if err != nil {
			return nil, 400, err
		}
		value, err := orders.Results(ctx, id, q)
		return value, 200, err
	}))
	api.Get("/production-results/:id", wrap(func(ctx context.Context, c fiber.Ctx) (any, int, error) {
		id, err := pathID(c)
		if err != nil {
			return nil, 400, err
		}
		value, err := orders.Result(ctx, id)
		return one(value, 200, err)
	}))
	app.Use(func(c fiber.Ctx) error { return domain.Missing() })
	return app
}

func pathID(c fiber.Ctx) (uuid.UUID, error) { return domain.ID(c.Params("id")) }

func query(c fiber.Ctx) (domain.Query, error) {
	q := domain.Query{Kind: c.Query("kind"), Search: strings.TrimSpace(c.Query("q")), Status: c.Query("status")}
	var err error
	q.Page, err = strconv.ParseInt(c.Query("page", "1"), 10, 64)
	if err != nil {
		return q, domain.Input("페이지 번호가 올바르지 않습니다.")
	}
	q.PageSize, err = strconv.ParseInt(c.Query("page_size", "20"), 10, 64)
	if err != nil {
		return q, domain.Input("페이지 크기가 올바르지 않습니다.")
	}
	if value := c.Query("item_id"); value != "" {
		id, err := domain.ID(value)
		if err != nil {
			return q, err
		}
		q.ItemID = &id
	}
	return q, domain.ValidateQuery(q)
}

func body(c fiber.Ctx, target any, required ...string) error {
	media, _, err := mime.ParseMediaType(c.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return domain.Input("Content-Type: application/json이 필요합니다.")
	}
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return domain.Input("JSON 필드와 정수 수량을 확인해 주세요.")
	}
	if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return domain.Input("하나의 JSON 객체만 허용합니다.")
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(c.Body(), &fields); err != nil || fields == nil {
		return domain.Input("JSON 객체가 필요합니다.")
	}
	for _, name := range required {
		if _, exists := fields[name]; !exists {
			return domain.Input("필수 입력 항목이 누락되었습니다.")
		}
	}
	for _, raw := range fields {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return domain.Input("입력값에 null을 사용할 수 없습니다.")
		}
	}
	if err := uniqueJSON(json.NewDecoder(bytes.NewReader(c.Body()))); err != nil {
		return domain.Input("JSON에 중복된 필드가 있습니다.")
	}
	return nil
}

func uniqueJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	seen := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("duplicate JSON key")
			}
			seen[name] = true
		}
		if err := uniqueJSON(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
