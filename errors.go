package apiauto

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	maxRetryAfterSeconds = 60
	maxErrorText         = 500
)

// Sentinels for [errors.Is]. Every failed request returns an [*Error] that matches one of them.
var (
	// ErrConnection: the request got no response: a network failure or a timeout.
	ErrConnection = errors.New("apiauto: connection error")
	// ErrBadRequest is 400: a bad parameter, a limit above the plan's, or offset given.
	ErrBadRequest = errors.New("apiauto: bad request")
	// ErrAuthentication is 401: no key, a wrong or revoked key, or the subscription has ended.
	ErrAuthentication = errors.New("apiauto: authentication error")
	// ErrQuotaExceeded is 402: the period quota is spent; ResetAt says when it renews.
	ErrQuotaExceeded = errors.New("apiauto: quota exceeded")
	// ErrPermissionDenied is 403: the market or feature is not in the plan.
	ErrPermissionDenied = errors.New("apiauto: permission denied")
	// ErrNotFound is 404: no such car or path.
	ErrNotFound = errors.New("apiauto: not found")
	// ErrRateLimit is 429: more requests per second than the plan allows; see RetryAfter.
	ErrRateLimit = errors.New("apiauto: rate limit")
	// ErrServer is any 5xx: a server-side failure.
	ErrServer = errors.New("apiauto: server error")
	// ErrServiceUnavailable is 503: the provider of live US search is unavailable; see RetryAfter.
	// It also matches ErrServer.
	ErrServiceUnavailable = errors.New("apiauto: service unavailable")
	// ErrCursorLoop: the API gave a cursor it had already given, so the iteration stopped. It is
	// not an [*Error].
	ErrCursorLoop = errors.New("apiauto: the API repeated a cursor")
)

// Error is a failed request. API errors are JSON such as {"error": "quota_exhausted", "reset_at":
// "..."}; their fields are copied here. See https://apiauto.space/en/docs/errors/.
type Error struct {
	// Message is a human-readable summary.
	Message string
	// StatusCode is the HTTP status, 0 when no response came.
	StatusCode int
	// Code is the error field of the body, such as quota_exhausted.
	Code string
	// Detail is the detail field: what exactly is wrong with the request. Like Code and ResetAt, it
	// has no control characters and at most 500 characters.
	Detail string
	// Body is the decoded JSON body, or the raw text when it is not JSON.
	Body any
	// RetryAfter is how many seconds to wait before retrying (429 and 503), from 0 to 60; nil when
	// not given. A larger or a negative value from the server is brought into that range.
	RetryAfter *int
	// ResetAt is when the quota renews, ISO 8601 (402).
	ResetAt string

	cause error
}

func (e *Error) Error() string { return "apiauto: " + e.Message }

// Unwrap returns the network error behind a connection failure.
func (e *Error) Unwrap() error { return e.cause }

// Is matches the package's sentinels by status.
func (e *Error) Is(target error) bool {
	switch target {
	case ErrConnection:
		return e.StatusCode == 0
	case ErrBadRequest:
		return e.StatusCode == http.StatusBadRequest
	case ErrAuthentication:
		return e.StatusCode == http.StatusUnauthorized
	case ErrQuotaExceeded:
		return e.StatusCode == http.StatusPaymentRequired
	case ErrPermissionDenied:
		return e.StatusCode == http.StatusForbidden
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound
	case ErrRateLimit:
		return e.StatusCode == http.StatusTooManyRequests
	case ErrServer:
		return e.StatusCode >= 500
	case ErrServiceUnavailable:
		return e.StatusCode == http.StatusServiceUnavailable
	}
	return false
}

func connectionError(err error) *Error {
	kind := "failed"
	if timeout, ok := err.(interface{ Timeout() bool }); ok && timeout.Timeout() {
		kind = "timed out"
	}
	return &Error{Message: fmt.Sprintf("the request to the API %s: %v", kind, err), cause: err}
}

func errorFromResponse(status int, retryAfterHeader string, body []byte) *Error {
	result := &Error{StatusCode: status, Body: string(body)}
	var fields map[string]any
	isJSON := json.Unmarshal(body, &fields) == nil
	if isJSON {
		result.Body = fields
		result.Code = textField(fields, "error")
		result.Detail = textField(fields, "detail")
		result.ResetAt = textField(fields, "reset_at")
		result.RetryAfter = retryAfterField(fields["retry_after"])
	}
	if result.RetryAfter == nil {
		result.RetryAfter = parseRetryAfter(retryAfterHeader)
	}

	result.Message = "HTTP " + strconv.Itoa(status)
	if result.Code != "" {
		result.Message += " " + result.Code
	}
	if result.Detail != "" {
		result.Message += ": " + result.Detail
	} else if text := cleanText(string(body)); !isJSON && text != "" {
		result.Message += ": " + text
	}
	return result
}

func textField(fields map[string]any, name string) string {
	value, _ := fields[name].(string)
	return cleanText(value)
}

// cleanText takes control characters out and keeps at most 500 characters: API text ends up in
// logs and terminals.
func cleanText(text string) string {
	cleaned := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text))
	if runes := []rune(cleaned); len(runes) > maxErrorText {
		return string(runes[:maxErrorText])
	}
	return cleaned
}

func retryAfterField(value any) *int {
	switch typed := value.(type) {
	case float64:
		return clampRetryAfter(typed)
	case string:
		return parseRetryAfter(typed)
	}
	return nil
}

func clampRetryAfter(seconds float64) *int {
	bounded := int(math.Ceil(math.Max(0, math.Min(seconds, maxRetryAfterSeconds))))
	return &bounded
}

// parseRetryAfter reads Retry-After: seconds or an HTTP date, brought to 0-60 seconds.
func parseRetryAfter(value string) *int {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil && strings.Trim(value, "-0123456789") == "" {
		return clampRetryAfter(seconds)
	}
	moment, err := http.ParseTime(value)
	if err != nil {
		return nil
	}
	return clampRetryAfter(time.Until(moment).Seconds())
}
