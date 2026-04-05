package server

import (
	_ "embed"
	"net/http"
)

//go:embed docs/swagger.json
var swaggerJSON []byte

// handleOpenAPISpec serves the embedded OpenAPI/Swagger spec as JSON.
func handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(swaggerJSON) //nolint:errcheck
}
