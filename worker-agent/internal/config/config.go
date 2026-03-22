package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Worker       WorkerConfig       `yaml:"worker"`
	ControlPlane ControlPlaneConfig `yaml:"control_plane"`
	Runtime      RuntimeConfig      `yaml:"runtime"`
	Capabilities Capabilities       `yaml:"capabilities"`
}

type WorkerConfig struct {
	ID              string `yaml:"id"`
	Name            string `yaml:"name"`
	TailscaleIP     string `yaml:"tailscale_ip"`
	MaxConcurrentJobs int  `yaml:"max_concurrent_jobs"`
}

type ControlPlaneConfig struct {
	URL          string `yaml:"url"`
	SharedSecret string `yaml:"shared_secret"`
}

type RuntimeConfig struct {
	Type       string `yaml:"type"` // kubernetes, docker
	Kubeconfig string `yaml:"kubeconfig"`
	Namespace  string `yaml:"namespace"`
	KubectlBin string `yaml:"kubectl_bin"`
}

type Capabilities struct {
	MemoryGB          int      `yaml:"memory_gb"`
	CPUCores          int      `yaml:"cpu_cores"`
	SupportedPackages []string `yaml:"supported_packages"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}

	// Validate required fields
	if cfg.Worker.ID == "" {
		return nil, fmt.Errorf("worker.id is required")
	}
	if cfg.Worker.Name == "" {
		return nil, fmt.Errorf("worker.name is required")
	}
	if cfg.ControlPlane.URL == "" {
		return nil, fmt.Errorf("control_plane.url is required")
	}
	if cfg.Runtime.Type == "" {
		cfg.Runtime.Type = "kubernetes"
	}

	return &cfg, nil
}
