package httpapi

import (
	"net/http"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/store"

	"github.com/gin-gonic/gin"
)

type WorkerHandler struct {
	workerRepo store.WorkerRepository
	jobRepo    store.JobRepository
}

func NewWorkerHandler(workerRepo store.WorkerRepository, jobRepo store.JobRepository) *WorkerHandler {
	return &WorkerHandler{
		workerRepo: workerRepo,
		jobRepo:    jobRepo,
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
		Status      string `json:"status"`       // idle, busy
		CurrentJobs int    `json:"current_jobs"`
	}
	c.BindJSON(&req)

	// Update heartbeat timestamp
	if err := h.workerRepo.UpdateHeartbeat(c.Request.Context(), workerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "heartbeat update failed"})
		return
	}

	// Update status and job count if provided
	if req.Status != "" {
		worker, err := h.workerRepo.Get(c.Request.Context(), workerID)
		if err == nil {
			worker.Status = req.Status
			worker.CurrentJobs = req.CurrentJobs
			h.workerRepo.Update(c.Request.Context(), worker)
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
		JobID string `json:"job_id" binding:"required"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id required"})
		return
	}

	// Mark job as done
	h.jobRepo.MarkDone(c.Request.Context(), req.JobID)

	// Decrement worker job count
	h.workerRepo.DecrementJobCount(c.Request.Context(), workerID)

	logs.Infof("workers", "job completed worker_id=%s job_id=%s", workerID, req.JobID)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// POST /api/workers/:id/job-failed
func (h *WorkerHandler) JobFailed(c *gin.Context) {
	workerID := c.Param("id")

	var req struct {
		JobID string `json:"job_id" binding:"required"`
		Error string `json:"error"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id required"})
		return
	}

	// Mark job as failed
	h.jobRepo.MarkFailed(c.Request.Context(), req.JobID, req.Error)

	// Decrement worker job count
	h.workerRepo.DecrementJobCount(c.Request.Context(), workerID)

	logs.Errorf("workers", "job failed worker_id=%s job_id=%s err=%s", workerID, req.JobID, req.Error)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GET /api/workers
func (h *WorkerHandler) List(c *gin.Context) {
	workers, err := h.workerRepo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list workers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"workers": workers})
}
