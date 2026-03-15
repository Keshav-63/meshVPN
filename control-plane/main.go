package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DeployRequest struct {
	Repo      string            `json:"repo" binding:"required"`
	Port      int               `json:"port"`
	Subdomain string            `json:"subdomain"`
	Env       map[string]string `json:"env"`
	BuildArgs map[string]string `json:"build_args"`
}

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var deploymentStore = NewDeploymentStore()

func main() {

	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "LaptopCloud running",
		})
	})

	router.POST("/deploy", deployHandler)
	router.GET("/deployments", listDeploymentsHandler)
	router.GET("/deployments/:id/build-logs", deploymentBuildLogsHandler)
	router.GET("/deployments/:id/app-logs", deploymentAppLogsHandler)

	router.Run(":8080")
}

func deployHandler(c *gin.Context) {

	var req DeployRequest

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "repo is required",
		})
		return
	}

	repoURL := strings.TrimSpace(req.Repo)
	if repoURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "repo is required",
		})
		return
	}

	port := req.Port
	if port == 0 {
		port = 3000
	}

	deploymentID := strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	subdomain := strings.TrimSpace(req.Subdomain)
	if subdomain == "" {
		subdomain = "app-" + deploymentID
	}

	runtimeEnv, err := sanitizeEnvMap(req.Env)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	buildArgs, err := sanitizeEnvMap(req.BuildArgs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid build_args: " + err.Error(),
		})
		return
	}

	start := time.Now().UTC()
	deploymentStore.Start(DeploymentRecord{
		DeploymentID: deploymentID,
		Repo:         repoURL,
		Subdomain:    subdomain,
		Port:         port,
		Status:       "deploying",
		Env:          cloneStringMap(runtimeEnv),
		BuildArgs:    cloneStringMap(buildArgs),
		StartedAt:    start,
	})

	result, buildLogs, err := deployRepo(repoURL, deploymentID, subdomain, port, runtimeEnv, buildArgs)
	if err != nil {
		finished := time.Now().UTC()
		deploymentStore.Update(DeploymentRecord{
			DeploymentID: deploymentID,
			Repo:         repoURL,
			Subdomain:    subdomain,
			Port:         port,
			Status:       "failed",
			Error:        err.Error(),
			BuildLogs:    buildLogs,
			Env:          cloneStringMap(runtimeEnv),
			BuildArgs:    cloneStringMap(buildArgs),
			StartedAt:    start,
			FinishedAt:   &finished,
		})
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":         err.Error(),
			"deployment_id": deploymentID,
			"build_logs":    buildLogs,
		})
		return
	}

	finished := time.Now().UTC()
	deploymentStore.Update(DeploymentRecord{
		DeploymentID: result.DeploymentID,
		Repo:         result.Repo,
		Subdomain:    result.Subdomain,
		Port:         result.Port,
		Container:    result.Container,
		Image:        result.Image,
		URL:          result.URL,
		Status:       "running",
		BuildLogs:    buildLogs,
		Env:          cloneStringMap(runtimeEnv),
		BuildArgs:    cloneStringMap(buildArgs),
		StartedAt:    start,
		FinishedAt:   &finished,
	})

	c.JSON(http.StatusOK, gin.H{
		"message":        "deployment completed",
		"deployment_id":  result.DeploymentID,
		"repo":           result.Repo,
		"repo_path":      result.RepoPath,
		"image":          result.Image,
		"container":      result.Container,
		"subdomain":      result.Subdomain,
		"url":            result.URL,
		"container_port": result.Port,
		"build_logs":     buildLogs,
	})
}

func listDeploymentsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"deployments": deploymentStore.List(),
	})
}

func deploymentBuildLogsHandler(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	rec, err := deploymentStore.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"deployment_id": rec.DeploymentID,
		"status":        rec.Status,
		"build_logs":    rec.BuildLogs,
	})
}

func deploymentAppLogsHandler(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	rec, err := deploymentStore.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	if strings.TrimSpace(rec.Container) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deployment has no running container"})
		return
	}

	tail := 200
	if raw := strings.TrimSpace(c.Query("tail")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tail must be a positive integer"})
			return
		}
		if parsed > 5000 {
			parsed = 5000
		}
		tail = parsed
	}

	logs, logErr := containerLogs(rec.Container, tail)
	if logErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":            logErr.Error(),
			"container":        rec.Container,
			"application_logs": logs,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"deployment_id":    rec.DeploymentID,
		"container":        rec.Container,
		"tail":             tail,
		"application_logs": logs,
	})
}

func sanitizeEnvMap(values map[string]string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	sanitized := make(map[string]string, len(values))
	for key, value := range values {
		trimmedKey := strings.TrimSpace(key)
		if !envKeyPattern.MatchString(trimmedKey) {
			return nil, fmt.Errorf("invalid env var name: %s", key)
		}
		sanitized[trimmedKey] = value
	}

	return sanitized, nil
}
