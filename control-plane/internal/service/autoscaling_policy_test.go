package service

import "testing"

func TestCPUFirstAutoscalingPolicyHorizontalDefaults(t *testing.T) {
	policy := NewCPUFirstAutoscalingPolicy()

	normalized, err := policy.Normalize(DeployRequest{
		ScalingMode: "horizontal",
		CPUCores:    0.5,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if normalized.MinReplicas != 2 || normalized.MaxReplicas != 10 {
		t.Fatalf("unexpected replica defaults: min=%d max=%d", normalized.MinReplicas, normalized.MaxReplicas)
	}
	if normalized.CPUTarget != 65 {
		t.Fatalf("expected default cpu target 65, got %d", normalized.CPUTarget)
	}
	if normalized.CPURequest != 500 {
		t.Fatalf("expected cpu request 500 from cpu cores, got %d", normalized.CPURequest)
	}
}

func TestCPUFirstAutoscalingPolicyRejectsBadReplicas(t *testing.T) {
	policy := NewCPUFirstAutoscalingPolicy()

	_, err := policy.Normalize(DeployRequest{
		ScalingMode: "horizontal",
		MinReplicas: 5,
		MaxReplicas: 2,
		CPURequest:  500,
	})
	if err == nil {
		t.Fatalf("expected validation error for replica bounds")
	}
}

func TestCPUFirstAutoscalingPolicyAllowsNone(t *testing.T) {
	policy := NewCPUFirstAutoscalingPolicy()

	normalized, err := policy.Normalize(DeployRequest{ScalingMode: "none"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if normalized.ScalingMode != "none" {
		t.Fatalf("expected scaling_mode none, got %s", normalized.ScalingMode)
	}
}
