package runtime

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	buildlogs "MeshVPN-slef-hosting/control-plane/internal/logs"
)

type KubernetesDriver struct {
	namespace string
	kubectl   string
}

func NewKubernetesDriver(namespace string) DeploymentDriver {
	if namespace == "" {
		namespace = "default"
	}

	kubectl := strings.TrimSpace(os.Getenv("KUBECTL_BIN"))
	if kubectl == "" {
		kubectl = "kubectl"
	}

	return &KubernetesDriver{namespace: namespace, kubectl: kubectl}
}

func (d *KubernetesDriver) DeployRepo(repo string, id string, subdomain string, port int, runtimeEnv map[string]string, buildArgs map[string]string, cpuCores float64, memoryMB int) (DeploymentResult, string, error) {
	var logs strings.Builder
	buildlogs.Infof("runtime-k8s", "deploy start deployment_id=%s repo=%s subdomain=%s port=%d", id, repo, subdomain, port)

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

	image, imageErr := d.buildAndPushImage(id, appPath, buildArgs)
	if imageErr != nil {
		buildlogs.AppendSection(&logs, "image build/push", imageErr.Error())
		return DeploymentResult{}, logs.String(), imageErr
	}
	buildlogs.AppendSection(&logs, "image", image)

	if err := d.ensureNamespace(); err != nil {
		buildlogs.AppendSection(&logs, "namespace", err.Error())
		return DeploymentResult{}, logs.String(), err
	}

	normalizedSubdomain := sanitizeSubdomain(subdomain)
	deployment := "app-" + id
	service := "svc-" + id
	ingress := "ing-" + id
	host := deploymentHost(normalizedSubdomain)

	manifest := d.renderWorkloadManifest(deployment, service, ingress, host, image, port, runtimeEnv, cpuCores, memoryMB)
	applyOutput, err := runCommandWithInput("", manifest, d.kubectl, "-n", d.namespace, "apply", "-f", "-")
	buildlogs.AppendSection(&logs, "kubectl apply", applyOutput)
	if err != nil {
		return DeploymentResult{}, logs.String(), fmt.Errorf("apply k8s manifests: %w", err)
	}

	rolloutOutput, err := runCommand("", d.kubectl, "-n", d.namespace, "rollout", "status", "deployment/"+deployment, "--timeout=600s")
	buildlogs.AppendSection(&logs, "rollout", rolloutOutput)
	if err != nil {
		return DeploymentResult{}, logs.String(), fmt.Errorf("wait deployment rollout: %w", err)
	}

	return DeploymentResult{
		DeploymentID: id,
		Repo:         repo,
		RepoPath:     appPath,
		Image:        image,
		Container:    deployment,
		Subdomain:    normalizedSubdomain,
		URL:          deploymentURL(normalizedSubdomain),
		Port:         port,
		BuildLogs:    logs.String(),
	}, logs.String(), nil
}

func (d *KubernetesDriver) ContainerLogs(container string, tail int) (string, error) {
	if tail <= 0 {
		tail = 200
	}

	output, err := runCommand("", d.kubectl, "-n", d.namespace, "logs", "deployment/"+container, "--tail", fmt.Sprintf("%d", tail), "--all-containers=true")
	if err != nil {
		return output, fmt.Errorf("get deployment logs: %w", err)
	}

	return output, nil
}

func (d *KubernetesDriver) ApplyCPUAutoscaling(workload string, minReplicas int, maxReplicas int, targetUtilization int) (string, error) {
	if strings.TrimSpace(workload) == "" {
		return "", fmt.Errorf("workload is required")
	}
	if minReplicas <= 0 || maxReplicas <= 0 || maxReplicas < minReplicas {
		return "", fmt.Errorf("invalid replica bounds for HPA")
	}
	if targetUtilization <= 0 || targetUtilization > 100 {
		return "", fmt.Errorf("invalid cpu target utilization for HPA")
	}

	memoryTargetUtilization := parsePercentEnv("HPA_MEMORY_TARGET_UTILIZATION", 75)
	if memoryTargetUtilization <= 0 || memoryTargetUtilization > 100 {
		return "", fmt.Errorf("invalid memory target utilization for HPA")
	}

	scaleDownStabilization := parseIntEnv("HPA_SCALE_DOWN_STABILIZATION_SECONDS", 60)
	if scaleDownStabilization < 0 {
		scaleDownStabilization = 60
	}

	scaleUpStabilization := parseIntEnv("HPA_SCALE_UP_STABILIZATION_SECONDS", 0)
	if scaleUpStabilization < 0 {
		scaleUpStabilization = 0
	}

	hpaName := "hpa-" + strings.TrimPrefix(workload, "app-")
	manifest := fmt.Sprintf(`apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: %s
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: %s
  minReplicas: %d
  maxReplicas: %d
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: %d
	- type: Resource
		resource:
			name: memory
			target:
				type: Utilization
				averageUtilization: %d
  behavior:
		scaleUp:
			stabilizationWindowSeconds: %d
			selectPolicy: Max
			policies:
			- type: Percent
				value: 100
				periodSeconds: 15
			- type: Pods
				value: 4
				periodSeconds: 15
    scaleDown:
			stabilizationWindowSeconds: %d
			selectPolicy: Max
			policies:
			- type: Percent
				value: 50
				periodSeconds: 30
			- type: Pods
				value: 2
				periodSeconds: 30
`, hpaName, workload, minReplicas, maxReplicas, targetUtilization, memoryTargetUtilization, scaleUpStabilization, scaleDownStabilization)

	// Defensive normalization: kubectl rejects tab indentation in YAML.
	manifest = strings.ReplaceAll(manifest, "\t", "  ")

	output, err := runCommandWithInput("", manifest, d.kubectl, "-n", d.namespace, "apply", "-f", "-")
	if err != nil {
		return output, fmt.Errorf("apply hpa: %w", err)
	}

	return output, nil
}

func parseIntEnv(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}

	return parsed
}

func parsePercentEnv(name string, defaultValue int) int {
	value := parseIntEnv(name, defaultValue)
	if value < 1 || value > 100 {
		return defaultValue
	}
	return value
}

func ingressMiddlewareAnnotation() string {
	return strings.TrimSpace(os.Getenv("TRAEFIK_INGRESS_MIDDLEWARE"))
}

func (d *KubernetesDriver) buildAndPushImage(id string, appPath string, buildArgs map[string]string) (string, error) {
	prefix := strings.TrimSpace(os.Getenv("K8S_IMAGE_PREFIX"))
	if prefix == "" {
		return "", fmt.Errorf("K8S_IMAGE_PREFIX is required for kubernetes backend")
	}

	prefix = strings.ToLower(strings.TrimRight(prefix, "/"))
	if strings.Contains(prefix, " ") {
		return "", fmt.Errorf("invalid K8S_IMAGE_PREFIX: must not contain spaces")
	}

	image := fmt.Sprintf("%s/laptopcloud-%s:%s", prefix, id, id)
	if _, err := buildImage(image, appPath, buildArgs); err != nil {
		return "", err
	}

	if _, err := runCommand("", "docker", "push", image); err != nil {
		return "", fmt.Errorf("push image: %w", err)
	}

	return image, nil
}

func (d *KubernetesDriver) ensureNamespace() error {
	if _, err := runCommand("", d.kubectl, "get", "namespace", d.namespace); err == nil {
		return nil
	}

	if _, err := runCommand("", d.kubectl, "create", "namespace", d.namespace); err != nil {
		return fmt.Errorf("ensure namespace %s: %w", d.namespace, err)
	}

	return nil
}

func (d *KubernetesDriver) renderWorkloadManifest(deployment string, service string, ingress string, host string, image string, appPort int, runtimeEnv map[string]string, cpuCores float64, memoryMB int) string {
	var sb strings.Builder

	sb.WriteString("apiVersion: apps/v1\n")
	sb.WriteString("kind: Deployment\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", deployment))
	sb.WriteString("spec:\n")
	sb.WriteString("  replicas: 1\n")
	sb.WriteString("  selector:\n")
	sb.WriteString("    matchLabels:\n")
	sb.WriteString(fmt.Sprintf("      app: %s\n", deployment))
	sb.WriteString("  template:\n")
	sb.WriteString("    metadata:\n")
	sb.WriteString("      labels:\n")
	sb.WriteString(fmt.Sprintf("        app: %s\n", deployment))
	sb.WriteString("    spec:\n")
	sb.WriteString("      containers:\n")
	sb.WriteString("      - name: app\n")
	sb.WriteString(fmt.Sprintf("        image: %s\n", image))
	sb.WriteString("        imagePullPolicy: Always\n")
	sb.WriteString("        ports:\n")
	sb.WriteString(fmt.Sprintf("        - containerPort: %d\n", appPort))

	if len(runtimeEnv) > 0 {
		sb.WriteString("        env:\n")
		keys := sortedMapKeys(runtimeEnv)
		for _, key := range keys {
			sb.WriteString("        - name: ")
			sb.WriteString(key)
			sb.WriteString("\n")
			sb.WriteString("          value: ")
			sb.WriteString(fmt.Sprintf("%q\n", runtimeEnv[key]))
		}
	}

	// Keep a tiny baseline when values are missing.
	if cpuCores <= 0 {
		cpuCores = 0.05 // 50 millicores baseline for idle pods
	}
	if memoryMB <= 0 {
		memoryMB = 64 // 64MB baseline
	}

	cpuMilli := int(cpuCores * 1000)

	sb.WriteString("        resources:\n")
	sb.WriteString("          requests:\n")
	sb.WriteString(fmt.Sprintf("            cpu: %dm\n", cpuMilli))
	sb.WriteString(fmt.Sprintf("            memory: %dMi\n", memoryMB))

	// Use package-scaled limits and guarantee limits >= requests.
	limitCpu := cpuMilli * 2
	if limitCpu < cpuMilli {
		limitCpu = cpuMilli
	}
	limitMem := memoryMB * 2
	if limitMem < memoryMB {
		limitMem = memoryMB
	}

	sb.WriteString("          limits:\n")
	sb.WriteString(fmt.Sprintf("            cpu: %dm\n", limitCpu))
	sb.WriteString(fmt.Sprintf("            memory: %dMi\n", limitMem))

	sb.WriteString("---\n")
	sb.WriteString("apiVersion: v1\n")
	sb.WriteString("kind: Service\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", service))
	sb.WriteString("spec:\n")
	sb.WriteString("  selector:\n")
	sb.WriteString(fmt.Sprintf("    app: %s\n", deployment))
	sb.WriteString("  ports:\n")
	sb.WriteString("  - name: http\n")
	sb.WriteString("    port: 80\n")
	sb.WriteString(fmt.Sprintf("    targetPort: %d\n", appPort))

	sb.WriteString("---\n")
	sb.WriteString("apiVersion: networking.k8s.io/v1\n")
	sb.WriteString("kind: Ingress\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", ingress))
	sb.WriteString("  annotations:\n")
	sb.WriteString("    traefik.ingress.kubernetes.io/router.entrypoints: web\n")
	if middleware := ingressMiddlewareAnnotation(); middleware != "" {
		sb.WriteString(fmt.Sprintf("    traefik.ingress.kubernetes.io/router.middlewares: %s\n", middleware))
	}
	sb.WriteString("  labels:\n")
	sb.WriteString(fmt.Sprintf("    app: %s\n", deployment))
	sb.WriteString(fmt.Sprintf("    deployment-id: \"%s\"\n", strings.TrimPrefix(deployment, "app-")))
	sb.WriteString("spec:\n")
	sb.WriteString("  ingressClassName: traefik\n")
	sb.WriteString("  rules:\n")
	sb.WriteString(fmt.Sprintf("  - host: %s\n", host))
	sb.WriteString("    http:\n")
	sb.WriteString("      paths:\n")
	sb.WriteString("      - path: /\n")
	sb.WriteString("        pathType: Prefix\n")
	sb.WriteString("        backend:\n")
	sb.WriteString("          service:\n")
	sb.WriteString(fmt.Sprintf("            name: %s\n", service))
	sb.WriteString("            port:\n")
	sb.WriteString("              number: 80\n")

	return sb.String()
}
