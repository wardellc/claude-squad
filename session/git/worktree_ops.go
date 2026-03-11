package git

import (
	"claude-squad/log"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrBranchAlreadyCheckedOut is returned when a branch is already checked out
// in another worktree. ExistingPath contains the path to that worktree.
type ErrBranchAlreadyCheckedOut struct {
	Branch       string
	ExistingPath string
}

func (e *ErrBranchAlreadyCheckedOut) Error() string {
	return fmt.Sprintf("branch %q is already checked out at %s", e.Branch, e.ExistingPath)
}

// Setup creates a new worktree for the session
func (g *GitWorktree) Setup() error {
	// Fetch the base branch from origin to ensure we have the latest state
	// Extract branch name from base branch (e.g., "origin/main" -> "main")
	baseBranchName := g.baseBranch
	if strings.HasPrefix(baseBranchName, "origin/") {
		baseBranchName = strings.TrimPrefix(baseBranchName, "origin/")
		if _, err := g.runGitCommand(g.repoPath, "fetch", "origin", baseBranchName); err != nil {
			// Log warning but continue - fetch failure shouldn't block worktree creation
			log.WarningLog.Printf("failed to fetch %s from origin: %v", baseBranchName, err)
		}
	}

	// Ensure worktrees directory exists early (can be done in parallel with branch check)
	worktreesDir, err := getWorktreeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	// Create directory and check branch existence in parallel
	errChan := make(chan error, 1)
	branchExistsCh := make(chan bool, 1)

	// Goroutine for directory creation
	go func() {
		errChan <- os.MkdirAll(worktreesDir, 0755)
	}()

	// Goroutine for branch check - use native git for speed
	go func() {
		// Use git rev-parse which is faster than opening repo with go-git
		cmd := exec.Command("git", "-C", g.repoPath, "rev-parse", "--verify", "--quiet", "refs/heads/"+g.branchName)
		branchExistsCh <- cmd.Run() == nil
	}()

	// Wait for both operations
	if err := <-errChan; err != nil {
		return err
	}
	branchExists := <-branchExistsCh

	if branchExists {
		return g.setupFromExistingBranch()
	}
	return g.setupNewWorktree()
}

// setupFromExistingBranch creates a worktree from an existing branch
func (g *GitWorktree) setupFromExistingBranch() error {
	// Directory already created in Setup(), skip duplicate creation

	// Clean up any existing worktree first
	_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath) // Ignore error if worktree doesn't exist

	// Create a new worktree from the existing branch
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", g.worktreePath, g.branchName); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "is already used by worktree at") ||
			strings.Contains(errMsg, "is already checked out at") {
			// Extract existing worktree path from error message
			existingPath := ""
			for _, prefix := range []string{"is already used by worktree at '", "is already checked out at '"} {
				if idx := strings.Index(errMsg, prefix); idx >= 0 {
					start := idx + len(prefix)
					if end := strings.Index(errMsg[start:], "'"); end >= 0 {
						existingPath = errMsg[start : start+end]
					}
				}
			}
			return &ErrBranchAlreadyCheckedOut{
				Branch:       g.branchName,
				ExistingPath: existingPath,
			}
		}
		return fmt.Errorf("failed to create worktree from branch %s: %w", g.branchName, err)
	}

	return nil
}

// ReuseExistingWorktree removes the existing worktree for this branch, then
// re-creates a fresh worktree. Used when the user confirms they want to
// replace an in-use worktree for a review.
func (g *GitWorktree) ReuseExistingWorktree(existingPath string) error {
	// Remove the existing worktree
	if existingPath != "" {
		_, _ = g.runGitCommand(g.repoPath, "worktree", "remove", "-f", existingPath)
	}
	// Prune stale worktree references
	_ = exec.Command("git", "-C", g.repoPath, "worktree", "prune").Run()

	// Now retry creating the worktree
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", g.worktreePath, g.branchName); err != nil {
		return fmt.Errorf("failed to create worktree from branch %s after cleanup: %w", g.branchName, err)
	}
	return nil
}

// PullLatest fetches the branch from origin and fast-forwards the worktree
// to match the remote. Should be called after the worktree is set up.
func (g *GitWorktree) PullLatest() error {
	// Fetch the branch from origin
	if _, err := g.runGitCommand(g.repoPath, "fetch", "origin", g.branchName); err != nil {
		log.WarningLog.Printf("failed to fetch branch %s from origin: %v", g.branchName, err)
		// Not fatal — branch may not exist on remote yet
		return nil
	}

	// Check if the worktree path exists
	if _, err := os.Stat(g.worktreePath); err != nil {
		return nil
	}

	// Pull (ff-only) inside the worktree to pick up remote changes
	if _, err := g.runGitCommand(g.worktreePath, "merge", "--ff-only", "origin/"+g.branchName); err != nil {
		// If ff-only fails, the local branch has diverged — log but don't fail
		log.WarningLog.Printf("could not fast-forward %s to origin: %v", g.branchName, err)
	}
	return nil
}

// setupNewWorktree creates a new worktree from the configured base branch
func (g *GitWorktree) setupNewWorktree() error {
	// Get base branch commit to branch from
	output, err := g.runGitCommand(g.repoPath, "rev-parse", g.baseBranch)
	if err != nil {
		if strings.Contains(err.Error(), "unknown revision") ||
			strings.Contains(err.Error(), "fatal: ambiguous argument") ||
			strings.Contains(err.Error(), "fatal: not a valid object name") {
			return fmt.Errorf("could not find %s: ensure the branch exists", g.baseBranch)
		}
		return fmt.Errorf("failed to get %s commit hash: %w", g.baseBranch, err)
	}
	baseCommit := strings.TrimSpace(string(output))
	g.baseCommitSHA = baseCommit

	// Create a new worktree from the base branch
	// The -b flag creates the branch, no need for separate cleanup since branch doesn't exist
	// (we already checked in Setup())
	if _, err := g.runGitCommand(g.repoPath, "worktree", "add", "-b", g.branchName, g.worktreePath, baseCommit); err != nil {
		// If it fails due to existing worktree/branch, try cleanup and retry once
		if strings.Contains(err.Error(), "already exists") {
			g.cleanupForRetry()
			if _, err := g.runGitCommand(g.repoPath, "worktree", "add", "-b", g.branchName, g.worktreePath, baseCommit); err != nil {
				return fmt.Errorf("failed to create worktree from commit %s: %w", baseCommit, err)
			}
			return nil
		}
		return fmt.Errorf("failed to create worktree from commit %s: %w", baseCommit, err)
	}

	return nil
}

// cleanupForRetry does minimal cleanup needed to retry worktree creation
func (g *GitWorktree) cleanupForRetry() {
	// Remove existing worktree if present
	_ = exec.Command("git", "-C", g.repoPath, "worktree", "remove", "-f", g.worktreePath).Run()
	// Delete branch if it exists
	_ = exec.Command("git", "-C", g.repoPath, "branch", "-D", g.branchName).Run()
	// Prune stale worktrees
	_ = exec.Command("git", "-C", g.repoPath, "worktree", "prune").Run()
}

// Cleanup removes the worktree and associated branch
func (g *GitWorktree) Cleanup() error {
	var errs []error

	// Check if worktree path exists before attempting removal
	if _, err := os.Stat(g.worktreePath); err == nil {
		// Remove the worktree using git command
		if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
			errs = append(errs, err)
		}
	} else if !os.IsNotExist(err) {
		// Only append error if it's not a "not exists" error
		errs = append(errs, fmt.Errorf("failed to check worktree path: %w", err))
	}

	// Delete branch using native git command (faster than go-git)
	if err := exec.Command("git", "-C", g.repoPath, "branch", "-D", g.branchName).Run(); err != nil {
		// Ignore error if branch doesn't exist
		if !strings.Contains(err.Error(), "not found") {
			log.WarningLog.Printf("failed to delete branch %s: %v", g.branchName, err)
		}
	}

	// Prune the worktree to clean up any remaining references
	if err := g.Prune(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return g.combineErrors(errs)
	}

	return nil
}

// Remove removes the worktree but keeps the branch
func (g *GitWorktree) Remove() error {
	// Remove the worktree using git command
	if _, err := g.runGitCommand(g.repoPath, "worktree", "remove", "-f", g.worktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	return nil
}

// Prune removes all working tree administrative files and directories
func (g *GitWorktree) Prune() error {
	if _, err := g.runGitCommand(g.repoPath, "worktree", "prune"); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}
	return nil
}

// CleanupWorktrees removes all worktrees and their associated branches
func CleanupWorktrees() error {
	worktreesDir, err := getWorktreeDirectory()
	if err != nil {
		return fmt.Errorf("failed to get worktree directory: %w", err)
	}

	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return fmt.Errorf("failed to read worktree directory: %w", err)
	}

	// Get a list of all branches associated with worktrees
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Parse the output to extract branch names
	worktreeBranches := make(map[string]string)
	currentWorktree := ""
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			currentWorktree = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			branchPath := strings.TrimPrefix(line, "branch ")
			// Extract branch name from refs/heads/branch-name
			branchName := strings.TrimPrefix(branchPath, "refs/heads/")
			if currentWorktree != "" {
				worktreeBranches[currentWorktree] = branchName
			}
		}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			worktreePath := filepath.Join(worktreesDir, entry.Name())

			// Delete the branch associated with this worktree if found
			for path, branch := range worktreeBranches {
				if strings.Contains(path, entry.Name()) {
					// Delete the branch
					deleteCmd := exec.Command("git", "branch", "-D", branch)
					if err := deleteCmd.Run(); err != nil {
						// Log the error but continue with other worktrees
						log.ErrorLog.Printf("failed to delete branch %s: %v", branch, err)
					}
					break
				}
			}

			// Remove the worktree directory
			os.RemoveAll(worktreePath)
		}
	}

	// You have to prune the cleaned up worktrees.
	cmd = exec.Command("git", "worktree", "prune")
	_, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}

	return nil
}
