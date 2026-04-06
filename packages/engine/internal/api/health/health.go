// Package health provides HTTP health-check endpoints for the Mantle API server.
package health

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type response struct {
	Status  string            `json:"status"`
	Details map[string]string `json:"details,omitempty"`
}

// LivenessChecker reports whether a component is alive.
type LivenessChecker interface {
	IsAlive() bool
	Name() string
}

// HealthzHandler returns an HTTP handler that always responds 200 OK, used as
// a liveness probe.
func HealthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response{Status: "ok"})
	}
}

// ReadyzHandler returns an HTTP handler used as a readiness probe. It pings
// the database and checks each LivenessChecker; it responds 503 if any check
// fails.
func ReadyzHandler(database *sql.DB, checkers ...LivenessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if database == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(response{Status: "unavailable"})
			return
		}

		if err := database.PingContext(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(response{Status: "unavailable"})
			return
		}

		// Check liveness of registered components.
		details := make(map[string]string)
		allAlive := true
		for _, c := range checkers {
			if c.IsAlive() {
				details[c.Name()] = "ok"
			} else {
				details[c.Name()] = "degraded"
				allAlive = false
			}
		}

		if !allAlive {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(response{Status: "degraded", Details: details})
			return
		}

		w.WriteHeader(http.StatusOK)
		if len(details) > 0 {
			json.NewEncoder(w).Encode(response{Status: "ok", Details: details})
		} else {
			json.NewEncoder(w).Encode(response{Status: "ok"})
		}
	}
}
