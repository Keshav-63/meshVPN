package runtime

import (
	"fmt"
	"strings"

	buildlogs "MeshVPN-slef-hosting/control-plane/internal/logs"
)

type DockerDriver struct{}

func NewDockerDriver() DeploymentDriver {
	return &DockerDriver{}
}

func (d *DockerDriver) DeployRepo(repo string, id string, subdomain string, port int, runtimeEnv map[string]string, buildArgs map[string]string, cpuCores float64, memoryMB int) (DeploymentResult, string, error) {
	return d.deployRepo(repo, id, subdomain, port, runtimeEnv, buildArgs, cpuCores, memoryMB, nil)
}

func (d *DockerDriver) DeployRepoWithUpdates(repo string, id string, subdomain string, port int, runtimeEnv map[string]string, buildArgs map[string]string, cpuCores float64, memoryMB int, onUpdate BuildLogUpdateFunc) (DeploymentResult, string, error) {
	return d.deployRepo(repo, id, subdomain, port, runtimeEnv, buildArgs, cpuCores, memoryMB, onUpdate)
}

func (d *DockerDriver) deployRepo(repo string, id string, subdomain string, port int, runtimeEnv map[string]string, buildArgs map[string]string, cpuCores float64, memoryMB int, onUpdate BuildLogUpdateFunc) (DeploymentResult, string, error) {
	var logs strings.Builder
	buildlogs.Infof("runtime", "deploy start deployment_id=%s repo=%s subdomain=%s port=%d cpu=%.2f memory_mb=%d", id, repo, subdomain, port, cpuCores, memoryMB)

	emitBuildSection(&logs, onUpdate, "clone", fmt.Sprintf("repo=%s", repo))
	appPath, cloneOutput, err := cloneRepo(repo, id)
	emitBuildSection(&logs, onUpdate, "clone output", cloneOutput)
	if err != nil {
		return DeploymentResult{}, logs.String(), err
	}

	if err := ensureDockerfile(appPath); err != nil {
		emitBuildSection(&logs, onUpdate, "dockerfile check", err.Error())
		return DeploymentResult{}, logs.String(), err
	}

	if err := ensureProxyNetwork(); err != nil {
		emitBuildSection(&logs, onUpdate, "proxy network", err.Error())
		return DeploymentResult{}, logs.String(), err
	}

	image := imageName(id)
	emitBuildSection(&logs, onUpdate, "build", fmt.Sprintf("image=%s", image))
	buildOutput, err := buildImageWithStream(image, appPath, buildArgs, onUpdate)
	emitBuildSection(&logs, onUpdate, "build output", buildOutput)
	if err != nil {
		return DeploymentResult{}, logs.String(), err
	}

	normalizedSubdomain := sanitizeSubdomain(subdomain)
	container := containerName(id)
	emitBuildSection(&logs, onUpdate, "run", fmt.Sprintf("container=%s", container))
	runOutput, err := runContainerWithStream(container, image, normalizedSubdomain, port, runtimeEnv, cpuCores, memoryMB, onUpdate)
	emitBuildSection(&logs, onUpdate, "run output", runOutput)
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

func emitBuildSection(sb *strings.Builder, onUpdate BuildLogUpdateFunc, title string, content string) {
	var section strings.Builder
	buildlogs.AppendSection(&section, title, content)
	chunk := section.String()
	sb.WriteString(chunk)
	if onUpdate != nil {
		onUpdate(chunk)
	}
}

func buildImageWithStream(image string, path string, buildArgs map[string]string, onOutput BuildLogUpdateFunc) (string, error) {
	args := []string{"build", "-t", image}
	for _, key := range sortedMapKeys(buildArgs) {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, buildArgs[key]))
	}
	args = append(args, path)

	output, err := runCommandStream("", onOutput, "docker", args...)
	if err != nil {
		return output, fmt.Errorf("build image: %w", err)
	}

	return output, nil
}

func runContainerWithStream(container string, image string, subdomain string, port int, runtimeEnv map[string]string, cpuCores float64, memoryMB int, onOutput BuildLogUpdateFunc) (string, error) {
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

	output, err := runCommandStream("", onOutput, "docker", args...)
	if err != nil {
		return output, fmt.Errorf("run container: %w", err)
	}

	return output, nil
}

func (d *DockerDriver) ContainerLogs(container string, tail int) (string, error) {
	if tail <= 0 {
		tail = 200
	}

	output, err := runCommand("", "docker", "logs", "--tail", fmt.Sprintf("%d", tail), container)
	if err != nil {
		return output, fmt.Errorf("get container logs: %w", err)
	}

	return output, nil
}

func (d *DockerDriver) ApplyCPUAutoscaling(workload string, minReplicas int, maxReplicas int, targetUtilization int) (string, error) {
	return "docker backend does not use HPA", nil
}
