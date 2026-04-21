package runtime

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const proxyNetworkName = "laptopcloud-proxy"
const defaultBaseDomain = "localhost"

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
	BuildLogs    string `json:"build_logs,omitempty"`
}

type Runner struct {
	driver DeploymentDriver
}

func NewRunner() *Runner {
	return NewRunnerWithDriver(NewDockerDriver())
}

func NewRunnerWithDriver(driver DeploymentDriver) *Runner {
	if driver == nil {
		driver = NewDockerDriver()
	}

	return &Runner{driver: driver}
}

func (r *Runner) DeployRepo(repo string, id string, subdomain string, port int, runtimeEnv map[string]string, buildArgs map[string]string, cpuCores float64, memoryMB int) (DeploymentResult, string, error) {
	return r.driver.DeployRepo(repo, id, subdomain, port, runtimeEnv, buildArgs, cpuCores, memoryMB)
}

func (r *Runner) DeployRepoWithUpdates(repo string, id string, subdomain string, port int, runtimeEnv map[string]string, buildArgs map[string]string, cpuCores float64, memoryMB int, onUpdate BuildLogUpdateFunc) (DeploymentResult, string, error) {
	if streamingDriver, ok := r.driver.(StreamingDeploymentDriver); ok {
		return streamingDriver.DeployRepoWithUpdates(repo, id, subdomain, port, runtimeEnv, buildArgs, cpuCores, memoryMB, onUpdate)
	}

	return r.driver.DeployRepo(repo, id, subdomain, port, runtimeEnv, buildArgs, cpuCores, memoryMB)
}

func (r *Runner) ContainerLogs(container string, tail int) (string, error) {
	return r.driver.ContainerLogs(container, tail)
}

func (r *Runner) ApplyCPUAutoscaling(workload string, minReplicas int, maxReplicas int, targetUtilization int) (string, error) {
	return r.driver.ApplyCPUAutoscaling(workload, minReplicas, maxReplicas, targetUtilization)
}

func cloneRepo(repo string, id string) (string, string, error) {
	appsRoot, err := appsDirectory()
	if err != nil {
		return "", "", fmt.Errorf("resolve apps directory: %w", err)
	}

	if err := os.MkdirAll(appsRoot, os.ModePerm); err != nil {
		return "", "", fmt.Errorf("create apps directory: %w", err)
	}

	path := filepath.Join(appsRoot, id)
	if err := os.RemoveAll(path); err != nil {
		return "", "", fmt.Errorf("reset app directory: %w", err)
	}

	output, err := runCommand("", "git", "clone", "--depth", "1", repo, path)
	if err != nil {
		return "", output, fmt.Errorf("clone repo: %w", err)
	}

	return path, output, nil
}

func buildImage(image string, path string, buildArgs map[string]string) (string, error) {
	args := []string{"build", "-t", image}
	for _, key := range sortedMapKeys(buildArgs) {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, buildArgs[key]))
	}
	args = append(args, path)

	output, err := runCommand("", "docker", args...)
	if err != nil {
		return output, fmt.Errorf("build image: %w", err)
	}

	return output, nil
}

func runContainer(container string, image string, subdomain string, port int, runtimeEnv map[string]string, cpuCores float64, memoryMB int) (string, error) {
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
	}

	for _, key := range sortedMapKeys(runtimeEnv) {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, runtimeEnv[key]))
	}

	if cpuCores > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.2f", cpuCores))
	}
	if memoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", memoryMB))
	}

	args = append(args, image)

	output, err := runCommand("", "docker", args...)
	if err != nil {
		return output, fmt.Errorf("run container: %w", err)
	}

	return output, nil
}

func ensureProxyNetwork() error {
	inspect := exec.Command("docker", "network", "inspect", proxyNetworkName)
	if err := inspect.Run(); err == nil {
		return nil
	}

	if _, err := runCommand("", "docker", "network", "create", proxyNetworkName); err != nil {
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
	return fmt.Sprintf("%s.%s", subdomain, baseDomain())
}

func deploymentURL(subdomain string) string {
	host := deploymentHost(subdomain)
	if baseDomain() == defaultBaseDomain {
		return fmt.Sprintf("http://%s", host)
	}

	return fmt.Sprintf("https://%s", host)
}

func baseDomain() string {
	raw := strings.TrimSpace(os.Getenv("APP_BASE_DOMAIN"))
	if raw == "" {
		return defaultBaseDomain
	}

	normalized := strings.Trim(strings.ToLower(raw), ".")
	if normalized == "" {
		return defaultBaseDomain
	}

	return normalized
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

func sortedMapKeys(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

func runCommand(dir string, name string, args ...string) (string, error) {
	return runCommandStream(dir, nil, name, args...)
}

func runCommandStream(dir string, onOutput BuildLogUpdateFunc, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if filepath.Base(name) == "kubectl" {
		kubeconfig := strings.TrimSpace(os.Getenv("K8S_CONFIG_PATH"))
		if kubeconfig != "" {
			cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig)
		}
	}

	var output bytes.Buffer
	mw := io.MultiWriter(&output, buildLogWriter{onOutput: onOutput})
	cmd.Stdout = mw
	cmd.Stderr = mw

	err := cmd.Run()
	outputText := output.String()
	if output.Len() > 0 {
		fmt.Print(outputText)
	}

	if err != nil {
		return outputText, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}

	return outputText, nil
}

func runCommandWithInput(dir string, input string, name string, args ...string) (string, error) {
	return runCommandWithInputStream(dir, input, nil, name, args...)
}

func runCommandWithInputStream(dir string, input string, onOutput BuildLogUpdateFunc, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if filepath.Base(name) == "kubectl" {
		kubeconfig := strings.TrimSpace(os.Getenv("K8S_CONFIG_PATH"))
		if kubeconfig != "" {
			cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig)
		}
	}

	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}

	var output bytes.Buffer
	mw := io.MultiWriter(&output, buildLogWriter{onOutput: onOutput})
	cmd.Stdout = mw
	cmd.Stderr = mw

	err := cmd.Run()
	outputText := output.String()
	if output.Len() > 0 {
		fmt.Print(outputText)
	}

	if err != nil {
		return outputText, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}

	return outputText, nil
}

type buildLogWriter struct {
	onOutput BuildLogUpdateFunc
}

func (w buildLogWriter) Write(p []byte) (int, error) {
	if w.onOutput != nil && len(p) > 0 {
		w.onOutput(string(p))
	}

	return len(p), nil
}
