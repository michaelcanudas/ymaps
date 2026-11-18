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

func TestDebugRoutesNeedDebugFlag(t *testing.T) {
	debug := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	routes := &Routes{Debug: debug}
	status := func() int {
		response := httptest.NewRecorder()
		Handler(routes).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/veo/spots", nil))
		return response.Code
	}

	t.Setenv("YMAPS_DEBUG", "")
	if code := status(); code != http.StatusNotFound {
		t.Fatalf("without flag: status %d", code)
	}
	t.Setenv("YMAPS_DEBUG", "1")
	if code := status(); code != http.StatusTeapot {
		t.Fatalf("with flag: status %d", code)
	}
}
