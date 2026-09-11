package httpapi

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRejectMalformedRequestsBeforeDatabaseAccess(t *testing.T) {
	app := New(nil)
	defer app.Shutdown()
	const id = "3e10a4c4-d8a4-4efb-bb7a-c29fdfe096d2"
	for _, tc := range []struct{ path, body string }{
		{"/items", `null`},
		{"/items", `[]`},
		{"/items", `{"code":"A","name":"A","kind":"component"}`},
		{"/items", `{"code":"A","code":"B","name":"A","kind":"component","unit":"EA"}`},
		{"/items", `{"code":"A","name":"A","kind":"component","unit":"EA","unknown":1}`},
		{"/items", `{"code":"A","name":"A","kind":"component","unit":"EA"} {}`},
		{"/stock-receipts", `{"item_id":"` + id + `","quantity":0.5}`},
		{"/stock-receipts", `{"item_id":"` + id + `","quantity":"1"}`},
		{"/stock-receipts", `{"item_id":"` + id + `","quantity":null}`},
		{"/production-orders/" + id + "/results", `{"good_quantity":null,"defective_quantity":1}`},
		{"/production-orders/" + id + "/results", `{"good_quantity":1}`},
	} {
		t.Run(tc.body, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://127.0.0.1/api/v1"+tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", id)
			response, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != 400 {
				t.Fatalf("got HTTP %d; want 400", response.StatusCode)
			}
			var decoded map[string]any
			if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil || decoded["error"] == nil {
				t.Fatal("missing JSON error envelope")
			}
		})
	}
}

func TestLocalHostAndOriginGuard(t *testing.T) {
	app := New(nil)
	defer app.Shutdown()
	for _, tc := range []struct{ host, origin string }{{"evil.example:8080", ""}, {"127.0.0.1:8080", "https://evil.example"}, {"127.0.0.1:8080", "null"}} {
		req := httptest.NewRequest("POST", "http://"+tc.host+"/api/v1/items", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		response, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatal("unsafe browser origin or Host accepted")
		}
	}
}

func TestInvalidReadQueries(t *testing.T) {
	app := New(nil)
	defer app.Shutdown()
	for _, path := range []string{"/items?page=0", "/items?page_size=101", "/inventory?kind=invalid", "/production-orders?status=invalid", "/stock-movements?item_id=invalid", "/items/not-a-uuid"} {
		response, err := app.Test(httptest.NewRequest("GET", "http://127.0.0.1/api/v1"+path, nil))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatal("invalid read query accepted")
		}
	}
}
