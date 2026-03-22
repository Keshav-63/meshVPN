package executor

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"

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
	log.Printf("Cloning repository: %s", repo)
	if err := e.cloneRepo(repo, deploymentID); err != nil {
		return fmt.Errorf("clone repo: %w", err)
	}

	// Step 2: Build Docker image
	imageName := fmt.Sprintf("%s/%s:latest", e.runtime.ImagePrefix, deploymentID)
	log.Printf("Building image: %s", imageName)
	if err := e.buildImage(deploymentID, imageName); err != nil {
		return fmt.Errorf("build image: %w", err)
	}

	// Step 3: Push image to registry
	log.Printf("Pushing image: %s", imageName)
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
	cmd := exec.Command("git", "clone", repo, fmt.Sprintf("/tmp/%s", deploymentID))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %s", string(output))
	}
	return nil
}

func (e *JobExecutor) buildImage(deploymentID, imageName string) error {
	cmd := exec.Command("docker", "build", "-t", imageName, fmt.Sprintf("/tmp/%s", deploymentID))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker build failed: %s", string(output))
	}
	return nil
}

func (e *JobExecutor) pushImage(imageName string) error {
	cmd := exec.Command("docker", "push", imageName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker push failed: %s", string(output))
	}
	return nil
}

func (e *JobExecutor) createDeployment(deploymentID, imageName, subdomain string, port int, job map[string]interface{}) error {
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
	manifest := e.generateManifest(deploymentID, imageName, subdomain, port, cpuCores, memoryMB)

	// Apply manifest using kubectl
	kubectlBin := e.runtime.KubectlBin
	if kubectlBin == "" {
		kubectlBin = "kubectl"
	}

	cmd := exec.Command(kubectlBin, "apply", "-f", "-")
	if e.runtime.Kubeconfig != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("KUBECONFIG=%s", e.runtime.Kubeconfig))
	}
	if e.runtime.Namespace != "" {
		cmd.Args = append(cmd.Args, "-n", e.runtime.Namespace)
	}

	cmd.Stdin = strings.NewReader(manifest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply failed: %s", string(output))
	}

	return nil
}

func (e *JobExecutor) generateManifest(deploymentID, imageName, subdomain string, port int, cpuCores float64, memoryMB int) string {
	// Prefix deployment ID with "app-" to ensure K8s naming compliance
	// K8s resource names must start with a letter
	resourceName := fmt.Sprintf("app-%s", deploymentID)

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
  - host: %s.keshavstack.tech
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
		resourceName, subdomain, resourceName)
}
