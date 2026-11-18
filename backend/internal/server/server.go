package server

import (
	"encoding/json"
	"net/http"
	"os"
)

type HelloResponse struct {
	Message string `json:"message"`
}

func Listen(address string, routes *Routes) error {
	return http.ListenAndServe(address, Handler(routes))
}

func Handler(routes *Routes) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/hello", writeHello)
	if routes != nil {
		mux.HandleFunc("POST /v1/routes", routes.plan)
		mux.HandleFunc("GET /v1/routes/{routeId}", routes.refresh)
		if routes.Debug != nil && os.Getenv("YMAPS_DEBUG") == "1" {
			mux.Handle("GET /debug/", routes.Debug)
		}
	}
	return withCORS(mux)
}

func writeHello(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HelloResponse{Message: "hello"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if request.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, request)
	})
}
