// Package server provides the Mantle HTTP API server.
//
//	@title          Mantle API
//	@version        1.0
//	@description    Headless AI workflow automation — BYOK, IaC-first, enterprise-grade.
//	@contact.name   Mantle
//	@contact.url    https://github.com/dvflw/mantle
//	@license.name   BSL 1.1
//	@license.url    https://github.com/dvflw/mantle/blob/main/LICENSE
//
//	@basePath    /
//
//	@securityDefinitions.apikey ApiKeyAuth
//	@in header
//	@name Authorization
//	@description Bearer API key. Format: "Bearer mk_..."
//
//	@securityDefinitions.apikey OIDCAuth
//	@in header
//	@name Authorization
//	@description Bearer OIDC JWT. Format: "Bearer <jwt>"
package server

// HealthResponse is the response body for /healthz.
type HealthResponse struct {
	Status string `json:"status"`
}

// ReadyzResponse is the response body for /readyz.
type ReadyzResponse struct {
	Status  string            `json:"status"`
	Details map[string]string `json:"details,omitempty"`
}

// healthzDoc is a swag documentation stub for GET /healthz.
// The actual handler is health.HealthzHandler() registered in Start().
//
//	@Summary  Liveness probe
//	@Tags     system
//	@Success  200  {object}  HealthResponse
//	@Router   /healthz [get]
func healthzDoc() {} //nolint:unused,deadcode

// readyzDoc is a swag documentation stub for GET /readyz.
// The actual handler is health.ReadyzHandler() registered in Start().
// Returns 503 when the database or worker/reaper components are unhealthy.
//
//	@Summary  Readiness probe
//	@Tags     system
//	@Success  200  {object}  ReadyzResponse
//	@Failure  503  {object}  ReadyzResponse
//	@Router   /readyz [get]
func readyzDoc() {} //nolint:unused,deadcode
