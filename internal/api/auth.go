package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func TokenAuth(valid []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(valid) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			for _, v := range valid {
				if subtle.ConstantTimeCompare([]byte(token), []byte(v)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized\n"))
		})
	}
}
