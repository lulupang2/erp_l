package domain

import (
	"time"

	"github.com/google/uuid"
)

const MaxInput int64 = 1_000_000
const MaxStock int64 = 1_000_000_000_000

type Item struct {
	ID        uuid.UUID `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Unit      string    `json:"unit"`
	CreatedAt time.Time `json:"created_at"`
}

type Material struct {
	ItemID          uuid.UUID `json:"item_id"`
	Code            string    `json:"code"`
	Name            string    `json:"name"`
	Unit            string    `json:"unit"`
	QuantityPerUnit int64     `json:"quantity_per_unit"`
}

type BOM struct {
	ID             uuid.UUID  `json:"id"`
	FinishedItemID uuid.UUID  `json:"finished_item_id"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Components     []Material `json:"components"`
}

type Inventory struct {
	ItemID    uuid.UUID `json:"item_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Unit      string    `json:"unit"`
	Quantity  int64     `json:"quantity"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Receipt struct {
	ID        uuid.UUID `json:"id"`
	ItemID    uuid.UUID `json:"item_id"`
	Quantity  int64     `json:"quantity"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

type Movement struct {
	ID           uuid.UUID  `json:"id"`
	ItemID       uuid.UUID  `json:"item_id"`
	Delta        int64      `json:"delta"`
	MovementType string     `json:"movement_type"`
	ReceiptID    *uuid.UUID `json:"receipt_id"`
	ResultID     *uuid.UUID `json:"result_id"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Order struct {
	ID                uuid.UUID `json:"id"`
	FinishedItemID    uuid.UUID `json:"finished_item_id"`
	PlannedQuantity   int64     `json:"planned_quantity"`
	GoodQuantity      int64     `json:"good_quantity"`
	DefectiveQuantity int64     `json:"defective_quantity"`
	RemainingQuantity int64     `json:"remaining_quantity"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
}

type OrderDetail struct {
	Order
	Materials []Material `json:"materials"`
}

type Result struct {
	ID                uuid.UUID `json:"id"`
	OrderID           uuid.UUID `json:"order_id"`
	GoodQuantity      int64     `json:"good_quantity"`
	DefectiveQuantity int64     `json:"defective_quantity"`
	Note              string    `json:"note"`
	CreatedAt         time.Time `json:"created_at"`
}

type Pagination struct {
	Page     int64 `json:"page"`
	PageSize int64 `json:"page_size"`
	Total    int64 `json:"total"`
}

type Page[T any] struct {
	Data       []T        `json:"data"`
	Pagination Pagination `json:"pagination"`
}

type Query struct {
	Page     int64
	PageSize int64
	Kind     string
	Search   string
	Status   string
	ItemID   *uuid.UUID
}

func (q Query) Offset() int64                     { return (q.Page - 1) * q.PageSize }
func (q Query) Pagination(total int64) Pagination { return Pagination{q.Page, q.PageSize, total} }

type ItemInput struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	Unit string `json:"unit"`
}
type ComponentInput struct {
	ItemID          string `json:"item_id"`
	QuantityPerUnit int64  `json:"quantity_per_unit"`
}
type BOMInput struct {
	Components []ComponentInput `json:"components"`
}
type ReceiptInput struct {
	ItemID   string `json:"item_id"`
	Quantity int64  `json:"quantity"`
	Note     string `json:"note"`
}
type OrderInput struct {
	FinishedItemID  string `json:"finished_item_id"`
	PlannedQuantity int64  `json:"planned_quantity"`
}
type ResultInput struct {
	GoodQuantity      int64  `json:"good_quantity"`
	DefectiveQuantity int64  `json:"defective_quantity"`
	Note              string `json:"note"`
}

type Shortage struct {
	ItemID            uuid.UUID `json:"item_id"`
	RequiredQuantity  int64     `json:"required_quantity"`
	AvailableQuantity int64     `json:"available_quantity"`
}

func OrderState(planned, good, defective int64) (remaining int64, status string) {
	remaining = planned - good - defective
	switch {
	case good+defective == 0:
		status = "pending"
	case remaining == 0:
		status = "completed"
	default:
		status = "in_progress"
	}
	return
}
