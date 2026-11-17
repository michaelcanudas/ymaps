package main

import (
	"log"
	"os"

	"ymaps/backend/internal/server"
)

func main() {
	log.Fatal(server.Listen(listenAddress()))
}

func listenAddress() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return ":" + port
}
