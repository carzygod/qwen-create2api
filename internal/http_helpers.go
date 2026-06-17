package internal

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorBody{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
			Type:    "qianwen_web_error",
		},
	})
}

func requireBearer(w http.ResponseWriter, r *http.Request, key string) bool {
	if key == "" {
		return true
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == key {
		return true
	}
	writeAPIError(w, http.StatusUnauthorized, "unauthorized", "Missing or invalid bearer token.")
	return false
}

func requireAPIAuth(w http.ResponseWriter, r *http.Request) bool {
	return requireBearer(w, r, Cfg.AuthKey)
}

func requireAdminAuth(w http.ResponseWriter, r *http.Request) bool {
	keys := []string{}
	for _, key := range []string{Cfg.AdminKey, os.Getenv("ADMIN_KEY"), Cfg.AuthKey, os.Getenv("AUTH_KEY")} {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		seen := false
		for _, existing := range keys {
			if existing == key {
				seen = true
				break
			}
		}
		if !seen {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return true
	}
	candidates := []string{
		strings.TrimSpace(r.URL.Query().Get("key")),
		strings.TrimSpace(r.Header.Get("X-Admin-Key")),
		strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")),
	}
	for _, candidate := range candidates {
		for _, key := range keys {
			if candidate != "" && candidate == key {
				return true
			}
		}
	}
	writeAPIError(w, http.StatusUnauthorized, "admin_unauthorized", "Missing or invalid admin key.")
	return false
}

func decodeJSON(r *http.Request, target interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	return dec.Decode(target)
}
