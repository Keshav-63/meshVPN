package store

import (
	"errors"
	"sync"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
)

type DeploymentRepository interface {
	Start(rec domain.DeploymentRecord)
	Update(rec domain.DeploymentRecord)
	Get(id string) (domain.DeploymentRecord, error)
	List() []domain.DeploymentRecord
}

type InMemoryDeploymentRepository struct {
	mu      sync.RWMutex
	records map[string]domain.DeploymentRecord
	order   []string
}

func NewInMemoryDeploymentRepository() *InMemoryDeploymentRepository {
	return &InMemoryDeploymentRepository{
		records: make(map[string]domain.DeploymentRecord),
		order:   make([]string, 0),
	}
}

func (s *InMemoryDeploymentRepository) Start(rec domain.DeploymentRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	logs.Debugf("store-memory", "start deployment deployment_id=%s status=%s", rec.DeploymentID, rec.Status)

	if _, exists := s.records[rec.DeploymentID]; !exists {
		s.order = append([]string{rec.DeploymentID}, s.order...)
	}

	s.records[rec.DeploymentID] = rec
}

func (s *InMemoryDeploymentRepository) Update(rec domain.DeploymentRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	logs.Debugf("store-memory", "update deployment deployment_id=%s status=%s", rec.DeploymentID, rec.Status)
	s.records[rec.DeploymentID] = rec
}

func (s *InMemoryDeploymentRepository) Get(id string) (domain.DeploymentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.records[id]
	if !ok {
		return domain.DeploymentRecord{}, errors.New("deployment not found")
	}

	return rec, nil
}

func (s *InMemoryDeploymentRepository) List() []domain.DeploymentRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]domain.DeploymentRecord, 0, len(s.order))
	for _, id := range s.order {
		rec, ok := s.records[id]
		if ok {
			result = append(result, rec)
		}
	}

	return result
}
