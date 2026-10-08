package api

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writePlain(w http.ResponseWriter, code int, body []byte) {
	w.WriteHeader(code)
	if len(body) > 0 {
		_, _ = w.Write(body)
	}
}
