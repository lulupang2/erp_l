package factory

import (
	"testing"

	"github.com/google/uuid"
)

func TestInputQtyEnforcesIntegerBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		q    int64
		min  int64
		good bool
	}{
		{"zero rejected", 0, 1, false},
		{"one accepted", 1, 1, true},
		{"million accepted", 1_000_000, 1, true},
		{"million plus one rejected", 1_000_001, 1, false},
		{"negative rejected", -1, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := inputQty(tc.q, tc.min); (got == nil) != tc.good {
				t.Fatalf("inputQty(%d, %d) error=%v, good=%v", tc.q, tc.min, got, tc.good)
			}
		})
	}
}

func TestInputQtyAllowsZeroOnlyWhenExplicitlyRequested(t *testing.T) {
	if err := inputQty(0, 0); err != nil {
		t.Fatalf("zero quantity should be valid for optional document fields: %v", err)
	}
	if err := inputQty(0, 1); err == nil {
		t.Fatal("zero quantity must be rejected for consuming operations")
	}
}

func TestDocumentRoleDefaultsDenyUnknownKinds(t *testing.T) {
	if roles := documentRole("unknown"); len(roles) != 0 {
		t.Fatalf("unknown document kind unexpectedly authorized: %v", roles)
	}
	if err := requireRole(Actor{ID: uuid.New(), Roles: []string{"operator"}}, documentRole("unknown")...); err == nil {
		t.Fatal("unknown document kind must not pass role enforcement")
	}
}

func TestHashInputIsStableAndInputScoped(t *testing.T) {
	left := struct {
		OrderID  string `json:"order_id"`
		Quantity int64  `json:"quantity"`
	}{OrderID: "a", Quantity: 1}
	right := struct {
		OrderID  string `json:"order_id"`
		Quantity int64  `json:"quantity"`
	}{OrderID: "a", Quantity: 2}
	if hashInput(left) == hashInput(right) {
		t.Fatal("different command payloads must not share a replay hash")
	}
	if hashInput(left) != hashInput(left) {
		t.Fatal("same command payload must have a stable replay hash")
	}
}

func TestRequireStateDefaultsDeny(t *testing.T) {
	if err := requireState("held", "issued", "in_progress"); err == nil {
		t.Fatal("held state must be rejected when it is not explicitly allowed")
	}
	if err := requireState("in_progress", "issued", "in_progress"); err != nil {
		t.Fatalf("explicitly allowed state was rejected: %v", err)
	}
}

func TestStartAllowanceUsesWorkOrderCap(t *testing.T) {
	if err := inputQty(1_000_000, 1); err != nil {
		t.Fatalf("maximum work-order quantity should be valid: %v", err)
	}
	if err := inputQty(1_000_001, 1); err == nil {
		t.Fatal("work-order quantity above one million must be rejected")
	}
}
