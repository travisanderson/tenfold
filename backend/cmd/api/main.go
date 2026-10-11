// Command api serves the Tenfold API.
//
// For now it serves only the health check, the contract and its docs. The /v1 handlers
// implement api.StrictServerInterface and are mounted in the backend session.
package main

import (
	"log"
	"net/http"
	"os"

	"tenfold/backend/internal/api"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.HealthHandler)
	mux.HandleFunc("GET /openapi.yaml", api.SpecHandler)
	mux.HandleFunc("GET /docs", api.DocsHandler)

	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
