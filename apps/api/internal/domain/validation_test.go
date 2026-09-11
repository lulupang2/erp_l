package domain

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestNormalizeItem(t *testing.T) {
	item, err := NormalizeItem(ItemInput{Code: " ab \t_12-가", Name: "이름", Kind: "component", Unit: "개"})
	if err == nil {
		t.Fatal("non-ASCII item code accepted", item)
	}
	item, err = NormalizeItem(ItemInput{Code: " ab\u2003_12- ", Name: " 키보드 ", Kind: "finished_good", Unit: " 개 "})
	if err != nil || item.Code != "AB_12-" || item.Name != "키보드" || item.Unit != "개" {
		t.Fatalf("normalization failed: %v", err)
	}
	for _, change := range []func(*ItemInput){
		func(v *ItemInput) { v.Code = strings.Repeat("A", 41) },
		func(v *ItemInput) { v.Name = " " },
		func(v *ItemInput) { v.Name = strings.Repeat("가", 101) },
		func(v *ItemInput) { v.Unit = strings.Repeat("개", 21) },
		func(v *ItemInput) { v.Name = "nul\x00byte" },
		func(v *ItemInput) { v.Kind = "other" },
	} {
		invalid := item
		change(&invalid)
		if _, err := NormalizeItem(invalid); err == nil {
			t.Fatal("invalid item accepted")
		}
	}
	if _, err := Text(strings.Repeat("가", 500), 0, 500, "메모"); err != nil {
		t.Fatal("Unicode rune lengths must be supported")
	}
}

func TestUUIDAndQuantities(t *testing.T) {
	const valid = "3e10a4c4-d8a4-4efb-bb7a-c29fdfe096d2"
	id, err := ID(strings.ToUpper(valid))
	if err != nil || id.String() != valid {
		t.Fatal("canonical UUID failed")
	}
	for _, value := range []string{"", "bad", "00000000-0000-0000-0000-000000000000", "urn:uuid:" + valid, strings.ReplaceAll(valid, "-", "")} {
		if _, err := ID(value); err == nil {
			t.Fatal("invalid UUID accepted")
		}
	}
	for _, value := range []int64{0, 1, MaxInput} {
		if err := Quantity(value, 0); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []int64{-1, MaxInput + 1} {
		if err := Quantity(value, 0); err == nil {
			t.Fatal("quantity range not enforced")
		}
	}
	if Quantity(0, 1) == nil {
		t.Fatal("positive quantity required")
	}
}

func TestOrderState(t *testing.T) {
	for _, tc := range []struct {
		good, defective, remaining int64
		status                     string
	}{{0, 0, 10, "pending"}, {4, 1, 5, "in_progress"}, {7, 3, 0, "completed"}, {0, 10, 0, "completed"}} {
		remaining, status := OrderState(10, tc.good, tc.defective)
		if remaining != tc.remaining || status != tc.status {
			t.Fatal("incorrect derived order state")
		}
	}
}

func TestPublicErrorsAreSanitized(t *testing.T) {
	const sensitive = "SENSITIVE-SENTINEL"
	for _, tc := range []struct {
		err    error
		status int
	}{
		{errors.New(sensitive), 500},
		{&pgconn.PgError{Code: "23505", Message: sensitive, Detail: sensitive}, 409},
		{&pgconn.PgError{Code: "40P01", Message: sensitive}, 503},
		{&pgconn.PgError{Code: "55P03", Message: sensitive}, 503},
		{context.DeadlineExceeded, 503},
	} {
		public := PublicError(tc.err)
		if public.Status != tc.status || strings.Contains(public.Message, sensitive) || public.Details != nil {
			t.Fatal("unsafe error mapping")
		}
	}
}
