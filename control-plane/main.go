package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DeployRequest struct {
	Repo      string `json:"repo" binding:"required"`
	Port      int    `json:"port"`
	Subdomain string `json:"subdomain"`
}

func main() {

	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "LaptopCloud running",
		})
	})

	router.POST("/deploy", deployHandler)

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

	result, err := deployRepo(repoURL, deploymentID, subdomain, port)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

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
	})
}
