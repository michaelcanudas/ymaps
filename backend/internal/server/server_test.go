package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSayHello(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/hello", nil)
	response := httptest.NewRecorder()

	Handler(nil).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type %s", response.Header().Get("Content-Type"))
	}

	var body HelloResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Message != "hello" {
		t.Fatalf("message %q", body.Message)
	}
}
