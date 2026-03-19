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
