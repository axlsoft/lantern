package httperr

import (
	"encoding/json"
	"net/http"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func write(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: errorDetail{Code: code, Message: message}})
}

func BadRequest(w http.ResponseWriter, message string)  { write(w, 400, "bad_request", message) }
func Unauthorized(w http.ResponseWriter, message string) { write(w, 401, "unauthorized", message) }
func Forbidden(w http.ResponseWriter, message string)   { write(w, 403, "forbidden", message) }
func NotFound(w http.ResponseWriter, message string)    { write(w, 404, "not_found", message) }
func Conflict(w http.ResponseWriter, message string)    { write(w, 409, "conflict", message) }
func UnsupportedMediaType(w http.ResponseWriter, message string) {
	write(w, 415, "unsupported_media_type", message)
}
func TooLarge(w http.ResponseWriter, message string)     { write(w, 413, "payload_too_large", message) }
func TooManyRequests(w http.ResponseWriter, retryAfter string) {
	w.Header().Set("Retry-After", retryAfter)
	write(w, 429, "too_many_requests", "rate limit exceeded; retry after "+retryAfter+" seconds")
}
func Internal(w http.ResponseWriter, message string) { write(w, 500, "internal_error", message) }
