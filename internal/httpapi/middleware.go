package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

type contextKey string

const requestIDKey contextKey = "requestId"

func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			b := make([]byte, 8)
			if _, err := rand.Read(b); err == nil {
				id = hex.EncodeToString(b)
			} else {
				id = time.Now().UTC().Format("20060102150405.000000000")
			}
		}
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestID(r *http.Request) string {
	value, _ := r.Context().Value(requestIDKey).(string)
	return value
}
