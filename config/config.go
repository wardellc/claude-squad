package config

import (
	"claude-squad/log"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ConfigFileName = "config.json"
	defaultProgram = "claude"
)

// GetConfigDir returns the path to the application's configuration directory
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config home directory: %w", err)
	}
	return filepath.Join(homeDir, ".claude-squad"), nil
}

// RepoInfo represents a git repository with usage statistics
type RepoInfo struct {
	Name  string // Directory name for display
	Path  string // Absolute path to repo
	Count int    // Usage count for sorting
}

// Config represents the application configuration
type Config struct {
	// DefaultProgram is the default program to run in new instances
	DefaultProgram string `json:"default_program"`
	// AutoYes is a flag to automatically accept all prompts.
	AutoYes bool `json:"auto_yes"`
	// DaemonPollInterval is the interval (ms) at which the daemon polls sessions for autoyes mode.
	DaemonPollInterval int `json:"daemon_poll_interval"`
	// BranchPrefix is the prefix used for git branches created by the application.
	BranchPrefix string `json:"branch_prefix"`
	// Editor is the command to open worktrees in an editor (e.g., "code", "cursor")
	Editor string `json:"editor,omitempty"`
	// Repos is a list of configured repository paths
	Repos []string `json:"repos,omitempty"`
	// RepoUsageCounts tracks how many worktrees have been created per repo (for sorting)
	RepoUsageCounts map[string]int `json:"repo_usage_counts,omitempty"`
	// LastUsedRepoPath tracks the most recently used repository path
	LastUsedRepoPath string `json:"last_used_repo_path,omitempty"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	program, err := GetClaudeCommand()
	if err != nil {
		log.ErrorLog.Printf("failed to get claude command: %v", err)
		program = defaultProgram
	}

	return &Config{
		DefaultProgram:     program,
		AutoYes:            false,
		DaemonPollInterval: 1000,
		BranchPrefix:       "",
	}
}

// GetClaudeCommand attempts to find the "claude" command in the user's shell
// It checks in the following order:
// 1. Shell alias resolution: using "which" command
// 2. PATH lookup
//
// If both fail, it returns an error.
func GetClaudeCommand() (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash" // Default to bash if SHELL is not set
	}

	// Force the shell to load the user's profile and then run the command
	// For zsh, source .zshrc; for bash, source .bashrc
	var shellCmd string
	if strings.Contains(shell, "zsh") {
		shellCmd = "source ~/.zshrc &>/dev/null || true; which claude"
	} else if strings.Contains(shell, "bash") {
		shellCmd = "source ~/.bashrc &>/dev/null || true; which claude"
	} else {
		shellCmd = "which claude"
	}

	cmd := exec.Command(shell, "-c", shellCmd)
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		path := strings.TrimSpace(string(output))
		if path != "" {
			// Check if the output is an alias definition and extract the actual path
			// Handle formats like "claude: aliased to /path/to/claude" or other shell-specific formats
			aliasRegex := regexp.MustCompile(`(?:aliased to|->|=)\s*([^\s]+)`)
			matches := aliasRegex.FindStringSubmatch(path)
			if len(matches) > 1 {
				path = matches[1]
			}
			return path, nil
		}
	}

	// Otherwise, try to find in PATH directly
	claudePath, err := exec.LookPath("claude")
	if err == nil {
		return claudePath, nil
	}

	return "", fmt.Errorf("claude command not found in aliases or PATH")
}

func LoadConfig() *Config {
	configDir, err := GetConfigDir()
	if err != nil {
		log.ErrorLog.Printf("failed to get config directory: %v", err)
		return DefaultConfig()
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create and save default config if file doesn't exist
			defaultCfg := DefaultConfig()
			if saveErr := saveConfig(defaultCfg); saveErr != nil {
				log.WarningLog.Printf("failed to save default config: %v", saveErr)
			}
			return defaultCfg
		}

		log.WarningLog.Printf("failed to get config file: %v", err)
		return DefaultConfig()
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		log.ErrorLog.Printf("failed to parse config file: %v", err)
		return DefaultConfig()
	}

	return &config
}

// saveConfig saves the configuration to disk
func saveConfig(config *Config) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, ConfigFileName)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

// SaveConfig exports the saveConfig function for use by other packages
func SaveConfig(config *Config) error {
	return saveConfig(config)
}

// IncrementRepoUsage increments the usage count for a repository and saves the config
func (c *Config) IncrementRepoUsage(repoPath string) error {
	if c.RepoUsageCounts == nil {
		c.RepoUsageCounts = make(map[string]int)
	}
	c.RepoUsageCounts[repoPath]++
	return saveConfig(c)
}

// GetRepoUsageCount returns the usage count for a repository
func (c *Config) GetRepoUsageCount(repoPath string) int {
	if c.RepoUsageCounts == nil {
		return 0
	}
	return c.RepoUsageCounts[repoPath]
}

// GetReposSortedByUsage returns RepoInfo slice sorted by usage count (descending)
func (c *Config) GetReposSortedByUsage(repos []RepoInfo) []RepoInfo {
	// Create a copy to avoid modifying the original
	sorted := make([]RepoInfo, len(repos))
	copy(sorted, repos)

	// Update counts from config
	for i := range sorted {
		sorted[i].Count = c.GetRepoUsageCount(sorted[i].Path)
	}

	// Sort by count descending
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Count > sorted[i].Count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// SetLastUsedRepo sets the most recently used repository path and saves the config
func (c *Config) SetLastUsedRepo(repoPath string) error {
	c.LastUsedRepoPath = repoPath
	return saveConfig(c)
}

// GetLastUsedRepoPath returns the most recently used repository path
func (c *Config) GetLastUsedRepoPath() string {
	return c.LastUsedRepoPath
}

// GetMostRecentRepo returns the most recently used repo, or the first repo if none used
func (c *Config) GetMostRecentRepo(repos []RepoInfo) RepoInfo {
	if c.LastUsedRepoPath != "" {
		for _, repo := range repos {
			if repo.Path == c.LastUsedRepoPath {
				return repo
			}
		}
	}
	// Fallback to first repo or empty
	if len(repos) > 0 {
		return repos[0]
	}
	return RepoInfo{}
}

// DiscoverRepos discovers git repositories in a directory
func DiscoverRepos(dir string) ([]RepoInfo, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var repos []RepoInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		repoPath := filepath.Join(absDir, entry.Name())
		gitDir := filepath.Join(repoPath, ".git")

		// Check if .git exists (file or directory - supports worktrees)
		if _, err := os.Stat(gitDir); err == nil {
			repos = append(repos, RepoInfo{
				Name: entry.Name(),
				Path: repoPath,
			})
		}
	}

	return repos, nil
}

// IsGitRepo checks if a directory is a git repository
func IsGitRepo(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	gitDir := filepath.Join(absPath, ".git")
	_, err = os.Stat(gitDir)
	return err == nil
}

// GetRepoName returns the name of a repository from its path
func GetRepoName(repoPath string) string {
	return filepath.Base(repoPath)
}
