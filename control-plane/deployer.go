package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const proxyNetworkName = "laptopcloud-proxy"

var invalidSubdomainChars = regexp.MustCompile(`[^a-z0-9-]+`)

type DeploymentResult struct {
	DeploymentID string `json:"deployment_id"`
	Repo         string `json:"repo"`
	RepoPath     string `json:"repo_path"`
	Image        string `json:"image"`
	Container    string `json:"container"`
	Subdomain    string `json:"subdomain"`
	URL          string `json:"url"`
	Port         int    `json:"port"`
}

func deployRepo(repo string, id string, subdomain string, port int) (DeploymentResult, error) {
	appPath, err := cloneRepo(repo, id)
	if err != nil {
		return DeploymentResult{}, err
	}

	if err := ensureDockerfile(appPath); err != nil {
		return DeploymentResult{}, err
	}

	if err := ensureProxyNetwork(); err != nil {
		return DeploymentResult{}, err
	}

	image := imageName(id)
	if err := buildImage(image, appPath); err != nil {
		return DeploymentResult{}, err
	}

	normalizedSubdomain := sanitizeSubdomain(subdomain)
	container := containerName(id)
	if err := runContainer(container, image, normalizedSubdomain, port); err != nil {
		return DeploymentResult{}, err
	}

	return DeploymentResult{
		DeploymentID: id,
		Repo:         repo,
		RepoPath:     appPath,
		Image:        image,
		Container:    container,
		Subdomain:    normalizedSubdomain,
		URL:          fmt.Sprintf("http://%s", deploymentHost(normalizedSubdomain)),
		Port:         port,
	}, nil
}

func cloneRepo(repo string, id string) (string, error) {
	appsRoot, err := appsDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve apps directory: %w", err)
	}

	if err := os.MkdirAll(appsRoot, os.ModePerm); err != nil {
		return "", fmt.Errorf("create apps directory: %w", err)
	}

	path := filepath.Join(appsRoot, id)
	if err := os.RemoveAll(path); err != nil {
		return "", fmt.Errorf("reset app directory: %w", err)
	}

	if err := runCommand("", "git", "clone", "--depth", "1", repo, path); err != nil {
		return "", fmt.Errorf("clone repo: %w", err)
	}

	return path, nil
}

func buildImage(image string, path string) error {
	if err := runCommand("", "docker", "build", "-t", image, path); err != nil {
		return fmt.Errorf("build image: %w", err)
	}

	return nil
}

func runContainer(container string, image string, subdomain string, port int) error {
	router := fmt.Sprintf("router-%s", container)
	service := fmt.Sprintf("service-%s", container)
	host := deploymentHost(subdomain)

	args := []string{
		"run",
		"-d",
		"--name", container,
		"--restart", "unless-stopped",
		"--network", proxyNetworkName,
		"--label", "traefik.enable=true",
		"--label", fmt.Sprintf("traefik.docker.network=%s", proxyNetworkName),
		"--label", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s`)", router, host),
		"--label", fmt.Sprintf("traefik.http.routers.%s.entrypoints=web", router),
		"--label", fmt.Sprintf("traefik.http.routers.%s.service=%s", router, service),
		"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%d", service, port),
		image,
	}

	if err := runCommand("", "docker", args...); err != nil {
		return fmt.Errorf("run container: %w", err)
	}

	return nil
}

func ensureProxyNetwork() error {
	inspect := exec.Command("docker", "network", "inspect", proxyNetworkName)
	if err := inspect.Run(); err == nil {
		return nil
	}

	if err := runCommand("", "docker", "network", "create", proxyNetworkName); err != nil {
		return fmt.Errorf("create proxy network: %w", err)
	}

	return nil
}

func ensureDockerfile(path string) error {
	dockerfilePath := filepath.Join(path, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		return fmt.Errorf("repo must contain a Dockerfile at %s", dockerfilePath)
	}

	return nil
}

func appsDirectory() (string, error) {
	return filepath.Abs(filepath.Join("..", "apps"))
}

func imageName(id string) string {
	return fmt.Sprintf("laptopcloud-%s", id)
}

func containerName(id string) string {
	return fmt.Sprintf("laptopcloud-%s", id)
}

func deploymentHost(subdomain string) string {
	return fmt.Sprintf("%s.localhost", subdomain)
}

func sanitizeSubdomain(subdomain string) string {
	normalized := strings.ToLower(strings.TrimSpace(subdomain))
	normalized = invalidSubdomainChars.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	if normalized == "" {
		return "app"
	}

	return normalized
}

func runCommand(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	if output.Len() > 0 {
		fmt.Print(output.String())
	}

	if err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}

	return nil
}
