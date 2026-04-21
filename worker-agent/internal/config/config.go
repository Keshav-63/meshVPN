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
	ID                string `yaml:"id"`
	Name              string `yaml:"name"`
	TailscaleIP       string `yaml:"tailscale_ip"`
	MaxConcurrentJobs int    `yaml:"max_concurrent_jobs"`
}

type ControlPlaneConfig struct {
	URL string `yaml:"url"`
}

type RuntimeConfig struct {
	Type          string `yaml:"type"` // kubernetes, docker
	Kubeconfig    string `yaml:"kubeconfig"`
	Namespace     string `yaml:"namespace"`
	KubectlBin    string `yaml:"kubectl_bin"`
	MetricsPort   int    `yaml:"metrics_port"`
	ImagePrefix   string `yaml:"image_prefix"`    // e.g., ghcr.io/keshav-63
	AppBaseDomain string `yaml:"app_base_domain"` // e.g., keshavstack.tech
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
	if cfg.Runtime.MetricsPort == 0 {
		// 9090 is commonly used by system Prometheus on Linux hosts.
		cfg.Runtime.MetricsPort = 9091
	}
	if cfg.Runtime.MetricsPort < 1 || cfg.Runtime.MetricsPort > 65535 {
		return nil, fmt.Errorf("runtime.metrics_port must be between 1 and 65535")
	}
	if cfg.Runtime.ImagePrefix == "" {
		return nil, fmt.Errorf("runtime.image_prefix is required (e.g., ghcr.io/your-username)")
	}
	if cfg.Runtime.AppBaseDomain == "" {
		cfg.Runtime.AppBaseDomain = "keshavstack.tech"
	}

	return &cfg, nil
}
