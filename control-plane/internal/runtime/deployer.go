package runtime

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	buildlogs "MeshVPN-slef-hosting/control-plane/internal/logs"
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

type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) DeployRepo(repo string, id string, subdomain string, port int, runtimeEnv map[string]string, buildArgs map[string]string, cpuCores float64, memoryMB int) (DeploymentResult, string, error) {
	var logs strings.Builder
	buildlogs.Infof("runtime", "deploy start deployment_id=%s repo=%s subdomain=%s port=%d cpu=%.2f memory_mb=%d", id, repo, subdomain, port, cpuCores, memoryMB)

	buildlogs.AppendSection(&logs, "clone", fmt.Sprintf("repo=%s", repo))
	appPath, cloneOutput, err := cloneRepo(repo, id)
	buildlogs.AppendSection(&logs, "clone output", cloneOutput)
	if err != nil {
		return DeploymentResult{}, logs.String(), err
	}

	if err := ensureDockerfile(appPath); err != nil {
		buildlogs.AppendSection(&logs, "dockerfile check", err.Error())
		return DeploymentResult{}, logs.String(), err
	}

	if err := ensureProxyNetwork(); err != nil {
		buildlogs.AppendSection(&logs, "proxy network", err.Error())
		return DeploymentResult{}, logs.String(), err
	}

	image := imageName(id)
	buildlogs.AppendSection(&logs, "build", fmt.Sprintf("image=%s", image))
	buildOutput, err := buildImage(image, appPath, buildArgs)
	buildlogs.AppendSection(&logs, "build output", buildOutput)
	if err != nil {
		return DeploymentResult{}, logs.String(), err
	}

	normalizedSubdomain := sanitizeSubdomain(subdomain)
	container := containerName(id)
	buildlogs.AppendSection(&logs, "run", fmt.Sprintf("container=%s", container))
	runOutput, err := runContainer(container, image, normalizedSubdomain, port, runtimeEnv, cpuCores, memoryMB)
	buildlogs.AppendSection(&logs, "run output", runOutput)
	if err != nil {
		buildlogs.Errorf("runtime", "run failed deployment_id=%s err=%v", id, err)
		return DeploymentResult{}, logs.String(), err
	}
	buildlogs.Infof("runtime", "deploy finished deployment_id=%s container=%s", id, container)

	return DeploymentResult{
		DeploymentID: id,
		Repo:         repo,
		RepoPath:     appPath,
		Image:        image,
		Container:    container,
		Subdomain:    normalizedSubdomain,
		URL:          deploymentURL(normalizedSubdomain),
		Port:         port,
		BuildLogs:    logs.String(),
	}, logs.String(), nil
}

func (r *Runner) ContainerLogs(container string, tail int) (string, error) {
	if tail <= 0 {
		tail = 200
	}

	output, err := runCommand("", "docker", "logs", "--tail", fmt.Sprintf("%d", tail), container)
	if err != nil {
		return output, fmt.Errorf("get container logs: %w", err)
	}

	return output, nil
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
	cmd := exec.Command(name, args...)
	cmd.Dir = dir

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

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
