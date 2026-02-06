package http

import (
	"net/http"
)

func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		//nolint: errcheck
		w.Write([]byte("OK"))
	}
}
