package service

import (
	"fmt"
	"math"
	"strings"
)

const (
	ScalingModeNone       = "none"
	ScalingModeHorizontal = "horizontal"

	defaultMinReplicas = 2
	defaultMaxReplicas = 10
	defaultCPUTarget   = 65
)

type AutoscalingPolicy interface {
	Normalize(req DeployRequest) (DeployRequest, error)
}

type CPUFirstAutoscalingPolicy struct{}

func NewCPUFirstAutoscalingPolicy() AutoscalingPolicy {
	return CPUFirstAutoscalingPolicy{}
}

func (CPUFirstAutoscalingPolicy) Normalize(req DeployRequest) (DeployRequest, error) {
	mode := strings.ToLower(strings.TrimSpace(req.ScalingMode))
	if mode == "" {
		mode = ScalingModeNone
	}

	switch mode {
	case ScalingModeNone, ScalingModeHorizontal:
		req.ScalingMode = mode
	default:
		return req, fmt.Errorf("invalid scaling_mode: %s", req.ScalingMode)
	}

	if req.CPURequest <= 0 && req.CPUCores > 0 {
		req.CPURequest = int(math.Round(req.CPUCores * 1000))
	}
	if req.CPUCores <= 0 && req.CPURequest > 0 {
		req.CPUCores = float64(req.CPURequest) / 1000
	}

	if req.ScalingMode == ScalingModeNone {
		return req, nil
	}

	if req.MinReplicas <= 0 {
		req.MinReplicas = defaultMinReplicas
	}
	if req.MaxReplicas <= 0 {
		req.MaxReplicas = defaultMaxReplicas
	}
	if req.MaxReplicas < req.MinReplicas {
		return req, fmt.Errorf("max_replicas must be greater than or equal to min_replicas")
	}

	if req.CPUTarget <= 0 {
		req.CPUTarget = defaultCPUTarget
	}
	if req.CPUTarget < 1 || req.CPUTarget > 100 {
		return req, fmt.Errorf("cpu_target_utilization must be between 1 and 100")
	}

	if req.CPURequest <= 0 {
		return req, fmt.Errorf("cpu_request_milli is required when scaling_mode is horizontal")
	}
	if req.CPULimit > 0 && req.CPULimit < req.CPURequest {
		return req, fmt.Errorf("cpu_limit_milli must be greater than or equal to cpu_request_milli")
	}

	return req, nil
}
