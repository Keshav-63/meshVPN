package executor

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"worker-agent/internal/config"
)

type JobExecutor struct {
	runtime config.RuntimeConfig
}

func New(runtime config.RuntimeConfig) *JobExecutor {
	return &JobExecutor{runtime: runtime}
}

func (e *JobExecutor) Execute(ctx context.Context, job map[string]interface{}) error {
	// Extract job details
	deploymentID, _ := job["deployment_id"].(string)
	repo, _ := job["repo"].(string)
	subdomain, _ := job["subdomain"].(string)
	port := int(job["port"].(float64))

	log.Printf("Executing job: deployment_id=%s repo=%s subdomain=%s", deploymentID, repo, subdomain)

	if e.runtime.Type == "kubernetes" {
		return e.executeKubernetes(ctx, deploymentID, repo, subdomain, port, job)
	}

	return fmt.Errorf("unsupported runtime: %s", e.runtime.Type)
}

func (e *JobExecutor) executeKubernetes(ctx context.Context, deploymentID, repo, subdomain string, port int, job map[string]interface{}) error {
	// Step 1: Clone repository
	log.Printf("=== clone ===\nrepo=%s", repo)
	if err := e.cloneRepo(repo, deploymentID); err != nil {
		return fmt.Errorf("clone repo: %w", err)
	}

	// Step 2: Build Docker image
	imageName := fmt.Sprintf("%s/%s:latest", e.runtime.ImagePrefix, deploymentID)
	log.Printf("=== image ===\n%s", imageName)
	if err := e.buildImage(deploymentID, imageName); err != nil {
		return fmt.Errorf("build image: %w", err)
	}

	// Step 3: Push image to registry
	log.Printf("=== push ===\n%s", imageName)
	if err := e.pushImage(imageName); err != nil {
		return fmt.Errorf("push image: %w", err)
	}

	// Step 4: Create Kubernetes deployment
	log.Printf("Creating Kubernetes deployment")
	if err := e.createDeployment(deploymentID, imageName, subdomain, port, job); err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}

	log.Printf("Job execution completed successfully")
	return nil
}

func (e *JobExecutor) cloneRepo(repo, deploymentID string) error {
	repoDir := fmt.Sprintf("/tmp/%s", deploymentID)
	if err := os.RemoveAll(repoDir); err != nil {
		return fmt.Errorf("cleanup old repo dir failed: %w", err)
	}

	cmd := exec.Command("git", "clone", repo, repoDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			return fmt.Errorf("git clone failed: %s", string(output))
		}
		return fmt.Errorf("git clone failed: %w", err)
	}
	logSection("clone output", string(output))
	return nil
}

func (e *JobExecutor) buildImage(deploymentID, imageName string) error {
	repoDir := fmt.Sprintf("/tmp/%s", deploymentID)
	dockerfilePath, buildContext, err := resolveBuildPaths(repoDir)
	if err != nil {
		return err
	}

	cmd := exec.Command("docker", "build", "-t", imageName, "-f", dockerfilePath, buildContext)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			return fmt.Errorf("docker build failed: %s", string(output))
		}
		return fmt.Errorf("docker build failed: %w", err)
	}
	logSection("docker build output", string(output))
	return nil
}

func resolveBuildPaths(repoDir string) (string, string, error) {
	rootDockerfile := filepath.Join(repoDir, "Dockerfile")
	if _, err := os.Stat(rootDockerfile); err == nil {
		return rootDockerfile, repoDir, nil
	}

	candidates := make([]string, 0)
	err := filepath.WalkDir(repoDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", ".next", "dist", "build", "vendor", ".venv", "venv":
				return filepath.SkipDir
			}
			return nil
		}

		if d.Name() == "Dockerfile" {
			candidates = append(candidates, path)
		}
		return nil
	})
	if err != nil {
		return "", "", fmt.Errorf("scan repo for Dockerfile failed: %w", err)
	}

	if len(candidates) == 0 {
		return "", "", fmt.Errorf("docker build failed: no Dockerfile found in repository (expected %s)", rootDockerfile)
	}

	if len(candidates) > 1 {
		sort.Strings(candidates)
		return "", "", fmt.Errorf("docker build failed: multiple Dockerfiles found: %s", strings.Join(candidates, ", "))
	}

	buildContext := filepath.Dir(candidates[0])
	return candidates[0], buildContext, nil
}

func (e *JobExecutor) pushImage(imageName string) error {
	cmd := exec.Command("docker", "push", imageName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker push failed: %s", string(output))
	}
	logSection("docker push output", string(output))
	return nil
}

func (e *JobExecutor) createDeployment(deploymentID, imageName, subdomain string, port int, job map[string]interface{}) error {
	resourceName := deploymentResourceName(deploymentID)

	// Extract resource specifications
	cpuCores := 0.5
	if cpu, ok := job["cpu_cores"].(float64); ok {
		cpuCores = cpu
	}

	memoryMB := 512
	if mem, ok := job["memory_mb"].(float64); ok {
		memoryMB = int(mem)
	}

	// Create Kubernetes manifest
	manifest := e.generateManifest(resourceName, imageName, subdomain, port, cpuCores, memoryMB)

	// Apply manifest using kubectl
	kubectlBin := e.runtime.KubectlBin
	if kubectlBin == "" {
		kubectlBin = "kubectl"
	}

	if e.runtime.Namespace != "" {
		if err := e.ensureNamespace(kubectlBin, e.runtime.Namespace); err != nil {
			return fmt.Errorf("ensure namespace: %w", err)
		}
	}

	cmd := exec.Command(kubectlBin, "apply", "-f", "-")
	args := cmd.Args[1:]
	if e.runtime.Namespace != "" {
		args = append(args, "-n", e.runtime.Namespace)
	}

	output, err := e.runKubectlWithRetry(kubectlBin, args, manifest)
	if err != nil {
		return fmt.Errorf("kubectl apply failed: %s", string(output))
	}
	logSection("kubectl apply", string(output))

	log.Printf("Waiting for rollout to complete: deployment/%s", resourceName)
	if err := e.waitForDeploymentRollout(kubectlBin, resourceName); err != nil {
		return err
	}

	return nil
}

func (e *JobExecutor) waitForDeploymentRollout(kubectlBin, resourceName string) error {
	args := []string{"rollout", "status", fmt.Sprintf("deployment/%s", resourceName), "--timeout=180s"}
	if e.runtime.Namespace != "" {
		args = append(args, "-n", e.runtime.Namespace)
	}

	output, err := e.runKubectlWithRetry(kubectlBin, args, "")
	if err != nil {
		if out := strings.TrimSpace(string(output)); out != "" {
			return fmt.Errorf("kubectl rollout status failed: %s", out)
		}
		return fmt.Errorf("kubectl rollout status failed: %w", err)
	}
	logSection("rollout", string(output))

	return nil
}

func logSection(title, content string) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		log.Printf("=== %s ===\n(no output)", title)
		return
	}
	log.Printf("=== %s ===\n%s", title, trimmed)
}

func (e *JobExecutor) kubectlEnv() []string {
	if e.runtime.Kubeconfig == "" {
		return nil
	}

	env := os.Environ()
	env = append(env, fmt.Sprintf("KUBECONFIG=%s", e.runtime.Kubeconfig))
	return env
}

func (e *JobExecutor) ensureNamespace(kubectlBin, namespace string) error {
	checkOutput, checkErr := e.runKubectlWithRetry(kubectlBin, []string{"get", "namespace", namespace}, "")
	if checkErr == nil {
		_ = checkOutput
		return nil
	}

	output, err := e.runKubectlWithRetry(kubectlBin, []string{"create", "namespace", namespace}, "")
	if err != nil {
		out := strings.TrimSpace(string(output))
		if strings.Contains(out, "AlreadyExists") {
			return nil
		}
		if out == "" {
			return fmt.Errorf("create namespace %q failed: %w", namespace, err)
		}
		return fmt.Errorf("create namespace %q failed: %s", namespace, out)
	}
	return nil
}

func (e *JobExecutor) runKubectlWithRetry(kubectlBin string, args []string, stdin string) ([]byte, error) {
	var lastOutput []byte
	var lastErr error

	for attempt := 1; attempt <= 3; attempt++ {
		cmd := exec.Command(kubectlBin, args...)
		cmd.Env = e.kubectlEnv()
		if stdin != "" {
			cmd.Stdin = strings.NewReader(stdin)
		}

		output, err := cmd.CombinedOutput()
		if err == nil {
			return output, nil
		}

		lastOutput = output
		lastErr = err

		if !isTransientKubectlError(string(output), err) || attempt == 3 {
			break
		}

		out := strings.TrimSpace(string(output))
		if out == "" {
			out = "(no kubectl output)"
		}
		log.Printf(
			"Transient kubectl error (attempt %d/3) for 'kubectl %s', retrying: %v | output: %s",
			attempt,
			strings.Join(args, " "),
			err,
			truncateForLog(out, 300),
		)
		time.Sleep(time.Duration(attempt) * time.Second)
	}

	return lastOutput, lastErr
}

func isTransientKubectlError(output string, err error) bool {
	combined := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v %s", err, output)))
	transientMarkers := []string{
		"unable to connect to the server",
		"connection reset by peer",
		"context deadline exceeded",
		"i/o timeout",
		"tls handshake timeout",
		"eof",
		"connection refused",
	}

	for _, marker := range transientMarkers {
		if strings.Contains(combined, marker) {
			return true
		}
	}

	return false
}

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func deploymentResourceName(deploymentID string) string {
	// Prefix deployment ID with "app-" to ensure K8s naming compliance.
	return fmt.Sprintf("app-%s", deploymentID)
}

func (e *JobExecutor) generateManifest(resourceName, imageName, subdomain string, port int, cpuCores float64, memoryMB int) string {
	baseDomain := strings.Trim(strings.ToLower(strings.TrimSpace(e.runtime.AppBaseDomain)), ".")
	if baseDomain == "" {
		baseDomain = "keshavstack.tech"
	}

	return fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
      - name: app
        image: %s
        ports:
        - containerPort: %d
        resources:
          requests:
            cpu: "%dm"
            memory: "%dMi"
          limits:
            cpu: "%dm"
            memory: "%dMi"
---
apiVersion: v1
kind: Service
metadata:
  name: %s
spec:
  selector:
    app: %s
  ports:
  - port: 80
    targetPort: %d
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: web
spec:
  rules:
%s
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: %s
            port:
              number: 80
`, resourceName, resourceName, resourceName, imageName, port,
		int(cpuCores*1000), memoryMB,
		int(cpuCores*1000*2), memoryMB*2,
		resourceName, resourceName, port,
		resourceName, fmt.Sprintf("  - host: %s.%s", subdomain, baseDomain), resourceName)
}
