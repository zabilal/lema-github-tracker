package handler

import (
    "encoding/json"
    "net/http"

    "github-service/pkg/errors"
)

// Response represents a standard API response
type Response struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data,omitempty"`
    Error   *Error      `json:"error,omitempty"`
}

// Error represents an error response
type Error struct {
    Type    string      `json:"type"`
    Message string      `json:"message"`
    Details interface{} `json:"details,omitempty"`
}

// WriteJSON writes a JSON response
func (h *GitHubHandler) WriteJSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)

    resp := Response{
        Success: status >= 200 && status < 300,
    }

    if !resp.Success {
        if err, ok := data.(error); ok {
            resp.Error = &Error{
                Type:    "unknown_error",
                Message: "An unexpected error occurred",
            }

            if appErr, ok := err.(*errors.Error); ok {
                resp.Error.Type = string(appErr.Type)
                resp.Error.Message = appErr.Message
                resp.Error.Details = appErr.Details
            }
        }
    } else {
        resp.Data = data
    }

    if err := json.NewEncoder(w).Encode(resp); err != nil {
        h.logger.Error("Failed to encode response", "error", err)
    }
}

// HandleError handles errors and writes an appropriate response
func (h *GitHubHandler) HandleError(w http.ResponseWriter, r *http.Request, err error) {
    status := http.StatusInternalServerError
    var errorType string

    if appErr, ok := err.(*errors.Error); ok {
        status = appErr.HTTPCode
        errorType = string(appErr.Type)
    } else {
        errorType = "internal_server_error"
    }

    // Log the error with request context
    h.logger.Error("Request error",
        "method", r.Method,
        "path", r.URL.Path,
        "status", status,
        "error", err.Error(),
        "type", errorType,
    )

    h.WriteJSON(w, status, err)
}