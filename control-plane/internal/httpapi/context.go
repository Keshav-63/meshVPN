package httpapi

import (
	"context"
	"net/http"

	"MeshVPN-slef-hosting/control-plane/internal/logs"
)

// ContextKey is a custom type for context keys
type ContextKey string

const (
	ContextKeyUserID       ContextKey = "user_id"
	ContextKeyDeploymentID ContextKey = "deployment_id"
	ContextKeyWorkerID     ContextKey = "worker_id"
	ContextKeyRequiredAuth ContextKey = "required_auth"
)

// ExtractUserID extracts user ID from request context
// Returns empty string if not found
func ExtractUserID(r *http.Request) string {
	userID := r.Context().Value(ContextKeyUserID)
	if userID != nil {
		if id, ok := userID.(string); ok && id != "" {
			return id
		}
	}
	logs.Debugf("httpapi-context", "user_id not found in request context")
	return ""
}

// ExtractDeploymentID extracts deployment ID from URL path or query parameter
// Priority: URL parameter > query parameter
func ExtractDeploymentID(r *http.Request) string {
	deploymentID := r.Context().Value(ContextKeyDeploymentID)
	if deploymentID != nil {
		if id, ok := deploymentID.(string); ok && id != "" {
			return id
		}
	}

	// Fallback to query parameter
	if id := r.URL.Query().Get("deployment_id"); id != "" {
		return id
	}

	logs.Debugf("httpapi-context", "deployment_id not found in request context or query")
	return ""
}

// ExtractWorkerID extracts worker ID from request context or query parameter
func ExtractWorkerID(r *http.Request) string {
	workerID := r.Context().Value(ContextKeyWorkerID)
	if workerID != nil {
		if id, ok := workerID.(string); ok && id != "" {
			return id
		}
	}

	// Fallback to query parameter
	if id := r.URL.Query().Get("worker_id"); id != "" {
		return id
	}

	logs.Debugf("httpapi-context", "worker_id not found in request context or query")
	return ""
}

// WithUserID returns a new context with user ID embedded
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ContextKeyUserID, userID)
}

// WithDeploymentID returns a new context with deployment ID embedded
func WithDeploymentID(ctx context.Context, deploymentID string) context.Context {
	return context.WithValue(ctx, ContextKeyDeploymentID, deploymentID)
}

// WithWorkerID returns a new context with worker ID embedded
func WithWorkerID(ctx context.Context, workerID string) context.Context {
	return context.WithValue(ctx, ContextKeyWorkerID, workerID)
}

// GetRequestContext returns the request context safely
// This is a standard accessor to avoid repeated r.Request.Context() calls
func GetRequestContext(r *http.Request) context.Context {
	if r == nil {
		logs.Errorf("httpapi-context", "GetRequestContext called with nil request")
		return context.Background()
	}
	return r.Context()
}
