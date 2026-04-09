package httpapi

import (
	"net/http"
	"os"
	"strings"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/store"

	"github.com/gin-gonic/gin"
)

type WorkerHandler struct {
	workerRepo store.WorkerRepository
	jobRepo    store.JobRepository
	deployRepo store.DeploymentRepository
}

func NewWorkerHandler(workerRepo store.WorkerRepository, jobRepo store.JobRepository, deployRepo store.DeploymentRepository) *WorkerHandler {
	return &WorkerHandler{
		workerRepo: workerRepo,
		jobRepo:    jobRepo,
		deployRepo: deployRepo,
	}
}

// POST /api/workers/register
func (h *WorkerHandler) Register(c *gin.Context) {
	var req struct {
		WorkerID     string                    `json:"worker_id" binding:"required"`
		Name         string                    `json:"name" binding:"required"`
		TailscaleIP  string                    `json:"tailscale_ip" binding:"required"`
		Hostname     string                    `json:"hostname"`
		Capabilities domain.WorkerCapabilities `json:"capabilities"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	worker := domain.Worker{
		WorkerID:          req.WorkerID,
		Name:              req.Name,
		TailscaleIP:       req.TailscaleIP,
		Hostname:          req.Hostname,
		Status:            string(domain.WorkerStatusIdle),
		Capabilities:      req.Capabilities,
		MaxConcurrentJobs: req.Capabilities.MaxConcurrentJobs,
		CurrentJobs:       0,
	}

	if err := h.workerRepo.Register(c.Request.Context(), worker); err != nil {
		logs.Errorf("workers", "registration failed worker_id=%s err=%v", worker.WorkerID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register worker"})
		return
	}

	logs.Infof("workers", "worker registered worker_id=%s name=%s ip=%s",
		worker.WorkerID, worker.Name, worker.TailscaleIP)

	c.JSON(http.StatusOK, gin.H{
		"status": "registered",
		"worker": worker,
	})
}

// POST /api/workers/:id/heartbeat
func (h *WorkerHandler) Heartbeat(c *gin.Context) {
	workerID := c.Param("id")

	var req struct {
		Status      string `json:"status"` // idle, busy
		CurrentJobs int    `json:"current_jobs"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			logs.Errorf("workers", "invalid heartbeat payload worker_id=%s err=%v", workerID, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid heartbeat payload"})
			return
		}
	}

	// Update heartbeat timestamp
	if err := h.workerRepo.UpdateHeartbeat(c.Request.Context(), workerID); err != nil {
		logs.Errorf("workers", "heartbeat update failed worker_id=%s err=%v", workerID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "heartbeat update failed"})
		return
	}

	// Update status and job count if provided
	if req.Status != "" {
		worker, err := h.workerRepo.Get(c.Request.Context(), workerID)
		if err == nil {
			worker.Status = req.Status
			worker.CurrentJobs = req.CurrentJobs
			if err := h.workerRepo.Update(c.Request.Context(), worker); err != nil {
				logs.Errorf("workers", "heartbeat status update failed worker_id=%s err=%v", workerID, err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "heartbeat status update failed"})
				return
			}
		} else {
			logs.Errorf("workers", "heartbeat worker lookup failed worker_id=%s err=%v", workerID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "heartbeat worker lookup failed"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GET /api/workers/:id/claim-job
func (h *WorkerHandler) ClaimJob(c *gin.Context) {
	workerID := c.Param("id")

	job, err := h.jobRepo.ClaimForWorker(c.Request.Context(), workerID)
	if err == store.ErrNoQueuedJobs {
		c.JSON(http.StatusOK, gin.H{"job": nil})
		return
	}
	if err != nil {
		logs.Errorf("workers", "claim job failed worker_id=%s err=%v", workerID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to claim job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"job": job})
}

// POST /api/workers/:id/job-complete
func (h *WorkerHandler) JobComplete(c *gin.Context) {
	workerID := c.Param("id")

	var req struct {
		JobID        string `json:"job_id" binding:"required"`
		DeploymentID string `json:"deployment_id"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id required"})
		return
	}

	// Mark job as done
	if err := h.jobRepo.MarkDone(c.Request.Context(), req.JobID); err != nil {
		logs.Errorf("workers", "mark job done failed worker_id=%s job_id=%s err=%v", workerID, req.JobID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark job done"})
		return
	}

	// Decrement worker job count
	if err := h.workerRepo.DecrementJobCount(c.Request.Context(), workerID); err != nil {
		logs.Errorf("workers", "decrement worker job count failed worker_id=%s job_id=%s err=%v", workerID, req.JobID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decrement worker job count"})
		return
	}

	if h.deployRepo != nil && req.DeploymentID != "" {
		rec, err := h.deployRepo.Get(req.DeploymentID)
		if err == nil {
			n := rec
			n.Status = "running"
			n.OwnerWorkerID = workerID
			if strings.TrimSpace(n.Container) == "" {
				n.Container = "app-" + req.DeploymentID
			}
			if strings.TrimSpace(n.URL) == "" {
				n.URL = buildWorkerDeploymentURL(n.Subdomain)
			}
			n.Error = ""
			n.FinishedAt = nil
			n.BuildLogs = n.BuildLogs + "\n=== worker ===\nremote worker reported job complete\n"
			h.deployRepo.Update(n)
		} else {
			logs.Errorf("workers", "deployment lookup failed for job complete worker_id=%s deployment_id=%s err=%v", workerID, req.DeploymentID, err)
		}
	}

	logs.Infof("workers", "job completed worker_id=%s job_id=%s", workerID, req.JobID)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// POST /api/workers/:id/job-failed
func (h *WorkerHandler) JobFailed(c *gin.Context) {
	workerID := c.Param("id")

	var req struct {
		JobID        string `json:"job_id" binding:"required"`
		DeploymentID string `json:"deployment_id"`
		Error        string `json:"error"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id required"})
		return
	}

	// Mark job as failed
	if err := h.jobRepo.MarkFailed(c.Request.Context(), req.JobID, req.Error); err != nil {
		logs.Errorf("workers", "mark job failed failed worker_id=%s job_id=%s err=%v", workerID, req.JobID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark job failed"})
		return
	}

	// Decrement worker job count
	if err := h.workerRepo.DecrementJobCount(c.Request.Context(), workerID); err != nil {
		logs.Errorf("workers", "decrement worker job count failed worker_id=%s job_id=%s err=%v", workerID, req.JobID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decrement worker job count"})
		return
	}

	if h.deployRepo != nil && req.DeploymentID != "" {
		rec, err := h.deployRepo.Get(req.DeploymentID)
		if err == nil {
			finishedAt := time.Now().UTC()
			n := rec
			n.Status = "failed"
			n.OwnerWorkerID = workerID
			if strings.TrimSpace(n.Container) == "" {
				n.Container = "app-" + req.DeploymentID
			}
			if strings.TrimSpace(n.URL) == "" {
				n.URL = buildWorkerDeploymentURL(n.Subdomain)
			}
			n.Error = req.Error
			n.FinishedAt = &finishedAt
			n.BuildLogs = n.BuildLogs + "\n=== worker error ===\n" + req.Error + "\n"
			h.deployRepo.Update(n)
		} else {
			logs.Errorf("workers", "deployment lookup failed for job failed worker_id=%s deployment_id=%s err=%v", workerID, req.DeploymentID, err)
		}
	}

	logs.Errorf("workers", "job failed worker_id=%s job_id=%s err=%s", workerID, req.JobID, req.Error)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GET /api/workers
// @Summary      List workers
// @Description  Get list of all worker nodes
// @Tags         Platform
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "List of workers"
// @Failure      401  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Security     BearerAuth
// @Router       /workers [get]
func (h *WorkerHandler) List(c *gin.Context) {
	workers, err := h.workerRepo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list workers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"workers": workers})
}

func buildWorkerDeploymentURL(subdomain string) string {
	baseDomain := strings.Trim(strings.ToLower(strings.TrimSpace(os.Getenv("APP_BASE_DOMAIN"))), ".")
	if baseDomain == "" {
		baseDomain = "keshavstack.tech"
	}

	return "https://" + subdomain + "." + baseDomain
}
