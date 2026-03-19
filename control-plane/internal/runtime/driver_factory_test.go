package runtime

import "testing"

func TestNewDriverFromBackendDefaultsToDocker(t *testing.T) {
	driver := NewDriverFromBackend("", "default")
	if _, ok := driver.(*DockerDriver); !ok {
		t.Fatalf("expected docker driver by default")
	}
}

func TestNewDriverFromBackendKubernetes(t *testing.T) {
	driver := NewDriverFromBackend("k3s", "meshvpn-apps")
	if _, ok := driver.(*KubernetesDriver); !ok {
		t.Fatalf("expected kubernetes driver for k3s backend")
	}
}
