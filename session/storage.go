package session

import (
	"claude-squad/config"
	"encoding/json"
	"fmt"
	"time"
)

// InstanceData represents the serializable data of an Instance
type InstanceData struct {
	Title        string    `json:"title"`
	InternalName string    `json:"internal_name,omitempty"`
	Path         string    `json:"path"`
	Branch       string    `json:"branch"`
	Status       Status    `json:"status"`
	Height       int       `json:"height"`
	Width        int       `json:"width"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	AutoYes      bool      `json:"auto_yes"`

	Program        string          `json:"program"`
	PermissionMode string          `json:"permission_mode"`
	IsReview       bool            `json:"is_review,omitempty"`
	Worktree       GitWorktreeData `json:"worktree"`
	DiffStats      DiffStatsData   `json:"diff_stats"`
	PRInfo         PRInfoData      `json:"pr_info,omitempty"`

	// Deprecated: kept for backward compatibility when loading old data
	DangerouslySkipPermissions bool `json:"dangerously_skip_permissions,omitempty"`
}

// GitWorktreeData represents the serializable data of a GitWorktree
type GitWorktreeData struct {
	RepoPath      string `json:"repo_path"`
	WorktreePath  string `json:"worktree_path"`
	SessionName   string `json:"session_name"`
	BranchName    string `json:"branch_name"`
	BaseCommitSHA string `json:"base_commit_sha"`
	BaseBranch    string `json:"base_branch,omitempty"`
}

// DiffStatsData represents the serializable data of a DiffStats
type DiffStatsData struct {
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Content string `json:"content"`
}

// PRInfoData represents the serializable data of a PRInfo
type PRInfoData struct {
	Number            int    `json:"number"`
	State             string `json:"state"`
	HasReviewRequired bool   `json:"has_review_required"`
	HasAssignee       bool   `json:"has_assignee"`
	IsApproved        bool   `json:"is_approved"`
}

// Storage handles saving and loading instances using the state interface
type Storage struct {
	state config.InstanceStorage
}

// NewStorage creates a new storage instance
func NewStorage(state config.InstanceStorage) (*Storage, error) {
	return &Storage{
		state: state,
	}, nil
}

// SaveInstances saves the list of instances to disk
func (s *Storage) SaveInstances(instances []*Instance) error {
	// Convert instances to InstanceData
	data := make([]InstanceData, 0)
	for _, instance := range instances {
		if instance.Started() {
			data = append(data, instance.ToInstanceData())
		}
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal instances: %w", err)
	}

	return s.state.SaveInstances(jsonData)
}

// LoadInstances loads the list of instances from disk
func (s *Storage) LoadInstances() ([]*Instance, error) {
	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	instances := make([]*Instance, len(instancesData))
	for i, data := range instancesData {
		instance, err := FromInstanceData(data)
		if err != nil {
			return nil, fmt.Errorf("failed to create instance %s: %w", data.Title, err)
		}
		instances[i] = instance
	}

	return instances, nil
}

// DeleteInstance removes an instance from storage by InternalName
func (s *Storage) DeleteInstance(internalName string) error {
	instances, err := s.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	found := false
	newInstances := make([]*Instance, 0)
	for _, instance := range instances {
		if instance.InternalName != internalName {
			newInstances = append(newInstances, instance)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", internalName)
	}

	return s.SaveInstances(newInstances)
}

// UpdateInstance updates an existing instance in storage by InternalName
func (s *Storage) UpdateInstance(instance *Instance) error {
	instances, err := s.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	found := false
	for i, existing := range instances {
		if existing.InternalName == instance.InternalName {
			instances[i] = instance
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", instance.InternalName)
	}

	return s.SaveInstances(instances)
}

// DeleteAllInstances removes all stored instances
func (s *Storage) DeleteAllInstances() error {
	return s.state.DeleteAllInstances()
}
