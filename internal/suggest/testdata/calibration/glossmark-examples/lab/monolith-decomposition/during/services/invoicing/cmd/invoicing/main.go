package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("invoicing listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
