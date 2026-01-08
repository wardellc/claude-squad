package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5"
)

// sanitizeBranchName transforms an arbitrary string into a Git branch name friendly string.
// Note: Git branch names have several rules, so this function uses a simple approach
// by allowing only a safe subset of characters.
func sanitizeBranchName(s string) string {
	// Convert to lower-case
	s = strings.ToLower(s)

	// Replace spaces with a dash
	s = strings.ReplaceAll(s, " ", "-")

	// Remove any characters not allowed in our safe subset.
	// Here we allow: letters, digits, dash, underscore, slash, and dot.
	re := regexp.MustCompile(`[^a-z0-9\-_/.]+`)
	s = re.ReplaceAllString(s, "")

	// Replace multiple dashes with a single dash (optional cleanup)
	reDash := regexp.MustCompile(`-+`)
	s = reDash.ReplaceAllString(s, "-")

	// Trim leading and trailing dashes or slashes to avoid issues
	s = strings.Trim(s, "-/")

	return s
}

// checkGHCLI checks if GitHub CLI is installed and configured
func checkGHCLI() error {
	// Check if gh is installed
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) is not installed. Please install it first")
	}

	// Check if gh is authenticated
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("GitHub CLI is not configured. Please run 'gh auth login' first")
	}

	return nil
}

// IsGitRepo checks if the given path is within a git repository
func IsGitRepo(path string) bool {
	for {
		_, err := git.PlainOpen(path)
		if err == nil {
			return true
		}

		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
}

func findGitRepoRoot(path string) (string, error) {
	currentPath := path
	for {
		_, err := git.PlainOpen(currentPath)
		if err == nil {
			// Found the repository root
			return currentPath, nil
		}

		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			// Reached the filesystem root without finding a repository
			return "", fmt.Errorf("failed to find Git repository root from path: %s", path)
		}
		currentPath = parent
	}
}

// BranchInfo represents a git branch with metadata
type BranchInfo struct {
	Name     string // Full branch name (e.g., "origin/main", "feature/xyz")
	IsRemote bool   // True if remote branch
}

// ListBranches returns all local and remote branches for a repository
// Branches are sorted: remote main first, then other remotes, then locals
func ListBranches(repoPath string) ([]BranchInfo, error) {
	cmd := exec.Command("git", "-C", repoPath, "branch", "-a", "--format=%(refname:short)")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	var remoteMain, otherRemotes, locals []BranchInfo
	seen := make(map[string]bool)

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] || strings.Contains(line, "HEAD") {
			continue
		}
		seen[line] = true

		isRemote := strings.HasPrefix(line, "origin/")
		isMain := strings.HasSuffix(line, "/main") || line == "main"

		branch := BranchInfo{Name: line, IsRemote: isRemote}

		if isRemote && isMain {
			remoteMain = append(remoteMain, branch)
		} else if isRemote {
			otherRemotes = append(otherRemotes, branch)
		} else {
			locals = append(locals, branch)
		}
	}

	// Combine: remote main first, then other remotes, then locals
	result := append(remoteMain, otherRemotes...)
	return append(result, locals...), nil
}
