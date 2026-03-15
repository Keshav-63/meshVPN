package main

import (
	"errors"
	"sync"
	"time"
)

type DeploymentRecord struct {
	DeploymentID string            `json:"deployment_id"`
	Repo         string            `json:"repo"`
	Subdomain    string            `json:"subdomain"`
	Port         int               `json:"port"`
	Container    string            `json:"container,omitempty"`
	Image        string            `json:"image,omitempty"`
	URL          string            `json:"url,omitempty"`
	Status       string            `json:"status"`
	Error        string            `json:"error,omitempty"`
	BuildLogs    string            `json:"build_logs,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	BuildArgs    map[string]string `json:"build_args,omitempty"`
	StartedAt    time.Time         `json:"started_at"`
	FinishedAt   *time.Time        `json:"finished_at,omitempty"`
}

type DeploymentStore struct {
	mu      sync.RWMutex
	records map[string]DeploymentRecord
	order   []string
}

func NewDeploymentStore() *DeploymentStore {
	return &DeploymentStore{
		records: make(map[string]DeploymentRecord),
		order:   make([]string, 0),
	}
}

func (s *DeploymentStore) Start(rec DeploymentRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.records[rec.DeploymentID]; !exists {
		s.order = append([]string{rec.DeploymentID}, s.order...)
	}

	s.records[rec.DeploymentID] = rec
}

func (s *DeploymentStore) Update(rec DeploymentRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[rec.DeploymentID] = rec
}

func (s *DeploymentStore) Get(id string) (DeploymentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.records[id]
	if !ok {
		return DeploymentRecord{}, errors.New("deployment not found")
	}

	return rec, nil
}

func (s *DeploymentStore) List() []DeploymentRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]DeploymentRecord, 0, len(s.order))
	for _, id := range s.order {
		rec, ok := s.records[id]
		if ok {
			result = append(result, rec)
		}
	}

	return result
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	copyMap := make(map[string]string, len(values))
	for k, v := range values {
		copyMap[k] = v
	}

	return copyMap
}
