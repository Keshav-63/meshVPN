package runtime

type BuildLogUpdateFunc func(chunk string)

type DeploymentDriver interface {
	DeployRepo(repo string, id string, subdomain string, port int, runtimeEnv map[string]string, buildArgs map[string]string, cpuCores float64, memoryMB int) (DeploymentResult, string, error)
	ContainerLogs(container string, tail int) (string, error)
	ApplyCPUAutoscaling(workload string, minReplicas int, maxReplicas int, targetUtilization int) (string, error)
}

// StreamingDeploymentDriver can emit incremental build logs while a deployment is in progress.
type StreamingDeploymentDriver interface {
	DeployRepoWithUpdates(repo string, id string, subdomain string, port int, runtimeEnv map[string]string, buildArgs map[string]string, cpuCores float64, memoryMB int, onUpdate BuildLogUpdateFunc) (DeploymentResult, string, error)
}
