package runtime

import "strings"

func NewDriverFromBackend(backend string, namespace string) DeploymentDriver {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "k8s", "kubernetes", "k3s":
		return NewKubernetesDriver(namespace)
	default:
		return NewDockerDriver()
	}
}
