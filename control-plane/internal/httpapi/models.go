package httpapi

// HealthResponse represents the health check response
type HealthResponse struct {
	Status string `json:"status" example:"LaptopCloud running"`
}

// WhoAmIResponse represents the authentication information response
type WhoAmIResponse struct {
	Sub      string `json:"sub" example:"user123"`
	Email    string `json:"email" example:"user@example.com"`
	Provider string `json:"provider" example:"github"`
}

// DeployResponse represents a successful deployment creation response
type DeployResponse struct {
	Message               string  `json:"message" example:"deployment queued"`
	DeploymentID          string  `json:"deployment_id" example:"dep-123456"`
	Status                string  `json:"status" example:"pending"`
	Repo                  string  `json:"repo" example:"https://github.com/user/repo"`
	Subdomain             string  `json:"subdomain" example:"myapp"`
	URL                   string  `json:"url" example:"https://myapp.keshavstack.tech"`
	Port                  int     `json:"port" example:"3000"`
	Package               string  `json:"package" example:"small"`
	CPUCores              float64 `json:"cpu_cores" example:"0.5"`
	MemoryMB              int     `json:"memory_mb" example:"512"`
	ScalingMode           string  `json:"scaling_mode" example:"horizontal"`
	MinReplicas           int     `json:"min_replicas" example:"1"`
	MaxReplicas           int     `json:"max_replicas" example:"3"`
	CPUTargetUtilization  int     `json:"cpu_target_utilization" example:"70"`
	AutoscalingEnabled    bool    `json:"autoscaling_enabled" example:"true"`
}

// DeploymentListResponse represents the list of deployments
type DeploymentListResponse struct {
	Deployments []DeploymentInfo `json:"deployments"`
}

// DeploymentInfo represents individual deployment information
type DeploymentInfo struct {
	DeploymentID string `json:"deployment_id" example:"dep-123456"`
	Repo         string `json:"repo" example:"https://github.com/user/repo"`
	Status       string `json:"status" example:"running"`
	Subdomain    string `json:"subdomain" example:"myapp"`
	Port         int    `json:"port" example:"3000"`
	Package      string `json:"package" example:"small"`
	ScalingMode  string `json:"scaling_mode" example:"horizontal"`
}

// BuildLogsResponse represents the build logs response
type BuildLogsResponse struct {
	DeploymentID string `json:"deployment_id" example:"dep-123456"`
	Status       string `json:"status" example:"building"`
	BuildLogs    string `json:"build_logs" example:"Step 1/5 : FROM node:18..."`
}

// AppLogsResponse represents the application logs response
type AppLogsResponse struct {
	DeploymentID    string `json:"deployment_id" example:"dep-123456"`
	Container       string `json:"container" example:"myapp-container"`
	Tail            int    `json:"tail" example:"200"`
	ApplicationLogs string `json:"application_logs" example:"Server listening on port 3000..."`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error" example:"Invalid request"`
}
