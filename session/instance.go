package session

import (
	"claude-squad/log"
	"claude-squad/session/git"
	"claude-squad/session/tmux"
	"os/exec"
	"path/filepath"

	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
)

type Status int

const (
	// Running is the status when the instance is running and claude is working.
	Running Status = iota
	// Ready is if the claude instance is ready to be interacted with (waiting for user input).
	Ready
	// Loading is if the instance is loading (if we are starting it up or something).
	Loading
	// Paused is if the instance is paused (worktree removed but branch preserved).
	Paused
	// Deleting is if the instance is being deleted (async deletion in progress).
	Deleting
)

// Instance is a running instance of claude code.
type Instance struct {
	// Title is the display name of the instance (user-provided).
	Title string
	// InternalName is the unique identifier combining repo and title: "{repoName}_{title}"
	// This allows same display name across different repos.
	InternalName string
	// Path is the path to the workspace.
	Path string
	// Branch is the branch of the instance.
	Branch string
	// Status is the status of the instance.
	Status Status
	// Program is the program to run in the instance.
	Program string
	// Height is the height of the instance.
	Height int
	// Width is the width of the instance.
	Width int
	// CreatedAt is the time the instance was created.
	CreatedAt time.Time
	// UpdatedAt is the time the instance was last updated.
	UpdatedAt time.Time
	// AutoYes is true if the instance should automatically press enter when prompted.
	AutoYes bool
	// Prompt is the initial prompt to pass to the instance on startup
	Prompt string
	// PermissionMode is the Claude permission mode ("plan" or "bypass")
	PermissionMode string
	// BaseBranch is the branch to create the worktree from
	BaseBranch string
	// IsReview marks this instance as a review session (shown in Review section)
	IsReview bool

	// DiffStats stores the current git diff statistics
	diffStats *git.DiffStats
	// PRInfo stores the current PR information
	prInfo *git.PRInfo

	// The below fields are initialized upon calling Start().

	started bool
	// worktreeReady is true when Setup() has already been called externally
	worktreeReady bool
	// tmuxSession is the tmux session for the instance.
	tmuxSession *tmux.TmuxSession
	// gitWorktree is the git worktree for the instance.
	gitWorktree *git.GitWorktree

	// Cached fields for performance (avoid repeated I/O on render)
	cachedRepoName string // Cached repo name
	cachedPreview  string // Cached tmux pane content
}

// ToInstanceData converts an Instance to its serializable form
func (i *Instance) ToInstanceData() InstanceData {
	data := InstanceData{
		Title:          i.Title,
		InternalName:   i.InternalName,
		Path:           i.Path,
		Branch:         i.Branch,
		Status:         i.Status,
		Height:         i.Height,
		Width:          i.Width,
		CreatedAt:      i.CreatedAt,
		UpdatedAt:      time.Now(),
		Program:        i.Program,
		AutoYes:        i.AutoYes,
		PermissionMode: i.PermissionMode,
		IsReview:       i.IsReview,
	}

	// Only include worktree data if gitWorktree is initialized
	if i.gitWorktree != nil {
		data.Worktree = GitWorktreeData{
			RepoPath:      i.gitWorktree.GetRepoPath(),
			WorktreePath:  i.gitWorktree.GetWorktreePath(),
			SessionName:   i.Title,
			BranchName:    i.gitWorktree.GetBranchName(),
			BaseCommitSHA: i.gitWorktree.GetBaseCommitSHA(),
			BaseBranch:    i.gitWorktree.GetBaseBranch(),
		}
	}

	// Only include diff stats if they exist
	if i.diffStats != nil {
		data.DiffStats = DiffStatsData{
			Added:   i.diffStats.Added,
			Removed: i.diffStats.Removed,
			Content: i.diffStats.Content,
		}
	}

	// Only include PR info if it exists
	if i.prInfo != nil {
		data.PRInfo = PRInfoData{
			Number:            i.prInfo.Number,
			State:             string(i.prInfo.State),
			HasReviewRequired: i.prInfo.HasReviewRequired,
			HasAssignee:       i.prInfo.HasAssignee,
			IsApproved:        i.prInfo.IsApproved,
		}
	}

	return data
}

// FromInstanceData creates a new Instance from serialized data
func FromInstanceData(data InstanceData) (*Instance, error) {
	// Backward compatibility: if InternalName is empty, use Title
	internalName := data.InternalName
	if internalName == "" {
		internalName = data.Title
	}

	// Backward compatibility: convert old DangerouslySkipPermissions to PermissionMode
	permissionMode := data.PermissionMode
	if permissionMode == "" {
		if data.DangerouslySkipPermissions {
			permissionMode = "bypass"
		} else {
			permissionMode = "bypass" // Default to bypass to maintain current behavior
		}
	}

	instance := &Instance{
		Title:          data.Title,
		InternalName:   internalName,
		Path:           data.Path,
		Branch:         data.Branch,
		Status:         data.Status,
		Height:         data.Height,
		Width:          data.Width,
		CreatedAt:      data.CreatedAt,
		UpdatedAt:      data.UpdatedAt,
		Program:        data.Program,
		PermissionMode: permissionMode,
		IsReview:       data.IsReview,
		gitWorktree: git.NewGitWorktreeFromStorage(
			data.Worktree.RepoPath,
			data.Worktree.WorktreePath,
			data.Worktree.SessionName,
			data.Worktree.BranchName,
			data.Worktree.BaseCommitSHA,
			data.Worktree.BaseBranch,
		),
		diffStats: &git.DiffStats{
			Added:   data.DiffStats.Added,
			Removed: data.DiffStats.Removed,
			Content: data.DiffStats.Content,
		},
		prInfo: &git.PRInfo{
			Number:            data.PRInfo.Number,
			State:             git.PRState(data.PRInfo.State),
			HasReviewRequired: data.PRInfo.HasReviewRequired,
			HasAssignee:       data.PRInfo.HasAssignee,
			IsApproved:        data.PRInfo.IsApproved,
		},
	}

	if instance.Paused() {
		instance.started = true
		instance.tmuxSession = tmux.NewTmuxSession(instance.InternalName, instance.Program, instance.PermissionMode)
	} else {
		if err := instance.Start(false); err != nil {
			return nil, err
		}
	}

	return instance, nil
}

// Options for creating a new instance
type InstanceOptions struct {
	// Title is the title of the instance.
	Title string
	// Path is the path to the workspace.
	Path string
	// Program is the program to run in the instance (e.g. "claude", "aider --model ollama_chat/gemma3:1b")
	Program string
	// If AutoYes is true, then
	AutoYes bool
	// PermissionMode is the Claude permission mode ("plan" or "bypass")
	PermissionMode string
	// BaseBranch is the branch to create the worktree from (defaults to "origin/main")
	BaseBranch string
}

func NewInstance(opts InstanceOptions) (*Instance, error) {
	t := time.Now()

	// Convert path to absolute
	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	return &Instance{
		Title:          opts.Title,
		Status:         Ready,
		Path:           absPath,
		Program:        opts.Program,
		Height:         0,
		Width:          0,
		CreatedAt:      t,
		UpdatedAt:      t,
		AutoYes:        false,
		PermissionMode: opts.PermissionMode,
		BaseBranch:     opts.BaseBranch,
	}, nil
}

func (i *Instance) RepoName() (string, error) {
	if !i.started {
		return "", fmt.Errorf("cannot get repo name for instance that has not been started")
	}
	if i.cachedRepoName == "" {
		i.cachedRepoName = i.gitWorktree.GetRepoName()
	}
	return i.cachedRepoName, nil
}

func (i *Instance) SetStatus(status Status) {
	i.Status = status
}

// firstTimeSetup is true if this is a new instance. Otherwise, it's one loaded from storage.
func (i *Instance) Start(firstTimeSetup bool) error {
	if i.Title == "" {
		return fmt.Errorf("instance title cannot be empty")
	}

	var tmuxSession *tmux.TmuxSession
	if i.tmuxSession != nil {
		// Use existing tmux session (useful for testing)
		tmuxSession = i.tmuxSession
	} else {
		// Create new tmux session using InternalName for uniqueness
		tmuxSession = tmux.NewTmuxSession(i.InternalName, i.Program, i.PermissionMode)
	}
	i.tmuxSession = tmuxSession

	if firstTimeSetup {
		// If gitWorktree is already set (e.g., for review instances), skip creation
		if i.gitWorktree == nil {
			gitWorktree, branchName, err := git.NewGitWorktree(i.Path, i.Title, i.BaseBranch)
			if err != nil {
				return fmt.Errorf("failed to create git worktree: %w", err)
			}
			i.gitWorktree = gitWorktree
			i.Branch = branchName
		}
	}

	// Setup error handler to cleanup resources on any error
	var setupErr error
	defer func() {
		if setupErr != nil {
			if cleanupErr := i.Kill(); cleanupErr != nil {
				setupErr = fmt.Errorf("%v (cleanup error: %v)", setupErr, cleanupErr)
			}
		} else {
			i.started = true
		}
	}()

	if !firstTimeSetup {
		// Check if tmux session still exists (may not after computer restart)
		if tmuxSession.DoesSessionExist() {
			// Session exists, just restore PTY connection
			if err := tmuxSession.Restore(); err != nil {
				setupErr = fmt.Errorf("failed to restore existing session: %w", err)
				return setupErr
			}
		} else {
			// Session was lost (e.g., computer restart), but worktree may still exist
			worktreePath := i.gitWorktree.GetWorktreePath()
			if _, statErr := os.Stat(worktreePath); statErr == nil {
				// Worktree exists - create new tmux session in it
				if err := i.tmuxSession.Start(worktreePath); err != nil {
					setupErr = fmt.Errorf("failed to start new session for existing worktree: %w", err)
					return setupErr
				}
				// Handle trust screen asynchronously for new session
				_ = i.tmuxSession.HandleTrustScreenAsync()
			} else {
				// Neither tmux session nor worktree exists - mark as paused
				i.SetStatus(Paused)
				return nil
			}
		}
	} else {
		// Setup git worktree (skip if already set up externally)
		if !i.worktreeReady {
			if err := i.gitWorktree.Setup(); err != nil {
				setupErr = fmt.Errorf("failed to setup git worktree: %w", err)
				return setupErr
			}
		}

		// Create new session
		if err := i.tmuxSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
			// Cleanup git worktree if tmux session creation fails
			if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
			}
			setupErr = fmt.Errorf("failed to start new session: %w", err)
			return setupErr
		}

		// Handle trust screen asynchronously (don't block startup)
		trustDone := i.tmuxSession.HandleTrustScreenAsync()

		// If there's a prompt to send, do it in background after trust screen
		if i.Prompt != "" {
			prompt := i.Prompt
			i.Prompt = "" // Clear immediately
			go func() {
				<-trustDone // Wait for trust screen to be handled
				// Brief delay to ensure the program is ready for input
				time.Sleep(500 * time.Millisecond)
				if err := i.tmuxSession.SendKeys(prompt); err != nil {
					log.ErrorLog.Printf("failed to send initial prompt: %v", err)
					return
				}
				// Brief pause to prevent carriage return from being interpreted as newline
				time.Sleep(100 * time.Millisecond)
				if err := i.tmuxSession.TapEnter(); err != nil {
					log.ErrorLog.Printf("failed to tap enter after initial prompt: %v", err)
				}
			}()
		}
	}

	i.SetStatus(Running)
	return nil
}

// Kill terminates the instance and cleans up all resources
func (i *Instance) Kill() error {
	if !i.started {
		// If instance was never started, just return success
		return nil
	}

	var errs []error

	// Always try to cleanup both resources, even if one fails
	// Clean up tmux session first since it's using the git worktree
	if i.tmuxSession != nil {
		if err := i.tmuxSession.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close tmux session: %w", err))
		}
	}

	// Then clean up git worktree
	if i.gitWorktree != nil {
		if err := i.gitWorktree.Cleanup(); err != nil {
			errs = append(errs, fmt.Errorf("failed to cleanup git worktree: %w", err))
		}
	}

	return i.combineErrors(errs)
}

// combineErrors combines multiple errors into a single error
func (i *Instance) combineErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	errMsg := "multiple cleanup errors occurred:"
	for _, err := range errs {
		errMsg += "\n  - " + err.Error()
	}
	return fmt.Errorf("%s", errMsg)
}

// Preview returns cached tmux pane content instantly (non-blocking).
// Use RefreshPreview() to update the cache.
func (i *Instance) Preview() (string, error) {
	if !i.started || i.Status == Paused {
		return "", nil
	}
	return i.cachedPreview, nil
}

// RefreshPreview updates the cached preview content by capturing tmux pane.
// This is the blocking operation that should be called from a ticker, not on key press.
func (i *Instance) RefreshPreview() error {
	if !i.started || i.Status == Paused {
		return nil
	}
	content, err := i.tmuxSession.CapturePaneContent()
	if err != nil {
		return err
	}
	i.cachedPreview = content
	return nil
}

func (i *Instance) HasUpdated() (updated bool, hasPrompt bool) {
	if !i.started {
		return false, false
	}
	return i.tmuxSession.HasUpdated()
}

// TapEnter sends an enter key press to the tmux session if AutoYes is enabled.
func (i *Instance) TapEnter() {
	if !i.started || !i.AutoYes {
		return
	}
	if err := i.tmuxSession.TapEnter(); err != nil {
		log.ErrorLog.Printf("error tapping enter: %v", err)
	}
}

func (i *Instance) Attach() (chan struct{}, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot attach instance that has not been started")
	}
	return i.tmuxSession.Attach()
}

func (i *Instance) SetPreviewSize(width, height int) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot set preview size for instance that has not been started or " +
			"is paused")
	}
	return i.tmuxSession.SetDetachedSize(width, height)
}

// GetGitWorktree returns the git worktree for the instance
func (i *Instance) GetGitWorktree() (*git.GitWorktree, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot get git worktree for instance that has not been started")
	}
	return i.gitWorktree, nil
}

func (i *Instance) Started() bool {
	return i.started
}

// SetTitle sets the title of the instance. Returns an error if the instance has started.
// We cant change the title once it's been used for a tmux session etc.
func (i *Instance) SetTitle(title string) error {
	if i.started {
		return fmt.Errorf("cannot change title of a started instance")
	}
	i.Title = title
	return nil
}

func (i *Instance) Paused() bool {
	return i.Status == Paused
}

// TmuxAlive returns true if the tmux session is alive. This is a sanity check before attaching.
func (i *Instance) TmuxAlive() bool {
	return i.tmuxSession.DoesSessionExist()
}

// Pause stops the tmux session and removes the worktree, preserving the branch
func (i *Instance) Pause() error {
	if !i.started {
		return fmt.Errorf("cannot pause instance that has not been started")
	}
	if i.Status == Paused {
		return fmt.Errorf("instance is already paused")
	}

	var errs []error

	// Check if there are any changes to commit
	if dirty, err := i.gitWorktree.IsDirty(); err != nil {
		errs = append(errs, fmt.Errorf("failed to check if worktree is dirty: %w", err))
		log.ErrorLog.Print(err)
	} else if dirty {
		// Commit changes locally (without pushing to GitHub)
		commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s (paused)", i.Title, time.Now().Format(time.RFC822))
		if err := i.gitWorktree.CommitChanges(commitMsg); err != nil {
			errs = append(errs, fmt.Errorf("failed to commit changes: %w", err))
			log.ErrorLog.Print(err)
			// Return early if we can't commit changes to avoid corrupted state
			return i.combineErrors(errs)
		}
	}

	// Detach from tmux session instead of closing to preserve session output
	if err := i.tmuxSession.DetachSafely(); err != nil {
		errs = append(errs, fmt.Errorf("failed to detach tmux session: %w", err))
		log.ErrorLog.Print(err)
		// Continue with pause process even if detach fails
	}

	// Check if worktree exists before trying to remove it
	if _, err := os.Stat(i.gitWorktree.GetWorktreePath()); err == nil {
		// Remove worktree but keep branch
		if err := i.gitWorktree.Remove(); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove git worktree: %w", err))
			log.ErrorLog.Print(err)
			return i.combineErrors(errs)
		}

		// Only prune if remove was successful
		if err := i.gitWorktree.Prune(); err != nil {
			errs = append(errs, fmt.Errorf("failed to prune git worktrees: %w", err))
			log.ErrorLog.Print(err)
			return i.combineErrors(errs)
		}
	}

	if err := i.combineErrors(errs); err != nil {
		log.ErrorLog.Print(err)
		return err
	}

	i.SetStatus(Paused)
	_ = clipboard.WriteAll(i.gitWorktree.GetBranchName())
	return nil
}

// Resume recreates the worktree and restarts the tmux session
func (i *Instance) Resume() error {
	if !i.started {
		return fmt.Errorf("cannot resume instance that has not been started")
	}
	if i.Status != Paused {
		return fmt.Errorf("can only resume paused instances")
	}

	// Check if branch is checked out
	if checked, err := i.gitWorktree.IsBranchCheckedOut(); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to check if branch is checked out: %w", err)
	} else if checked {
		return fmt.Errorf("cannot resume: branch is checked out, please switch to a different branch")
	}

	// Setup git worktree
	if err := i.gitWorktree.Setup(); err != nil {
		log.ErrorLog.Print(err)
		return fmt.Errorf("failed to setup git worktree: %w", err)
	}

	// Check if tmux session still exists from pause, otherwise create new one
	if i.tmuxSession.DoesSessionExist() {
		// Session exists, just restore PTY connection to it
		if err := i.tmuxSession.Restore(); err != nil {
			log.ErrorLog.Print(err)
			// If restore fails, fall back to creating new session
			if err := i.tmuxSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
				log.ErrorLog.Print(err)
				// Cleanup git worktree if tmux session creation fails
				if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
					err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
					log.ErrorLog.Print(err)
				}
				return fmt.Errorf("failed to start new session: %w", err)
			}
			// Handle trust screen asynchronously for new session
			_ = i.tmuxSession.HandleTrustScreenAsync()
		}
	} else {
		// Create new tmux session
		if err := i.tmuxSession.Start(i.gitWorktree.GetWorktreePath()); err != nil {
			log.ErrorLog.Print(err)
			// Cleanup git worktree if tmux session creation fails
			if cleanupErr := i.gitWorktree.Cleanup(); cleanupErr != nil {
				err = fmt.Errorf("%v (cleanup error: %v)", err, cleanupErr)
				log.ErrorLog.Print(err)
			}
			return fmt.Errorf("failed to start new session: %w", err)
		}
		// Handle trust screen asynchronously
		_ = i.tmuxSession.HandleTrustScreenAsync()
	}

	i.SetStatus(Running)
	return nil
}

// Restart closes the existing tmux session (if any) and creates a new one in the worktree.
// This is useful when the tmux session is in a bad state or needs to be refreshed.
func (i *Instance) Restart() error {
	if !i.started {
		return fmt.Errorf("cannot restart: instance not started")
	}

	if i.Status == Paused {
		return fmt.Errorf("cannot restart: instance is paused, use resume instead")
	}

	// Get worktree path before closing session
	worktreePath := i.gitWorktree.GetWorktreePath()

	// Check if worktree exists
	if _, err := os.Stat(worktreePath); err != nil {
		return fmt.Errorf("cannot restart: worktree does not exist at %s", worktreePath)
	}

	// Close existing tmux session if it exists
	if i.tmuxSession != nil && i.tmuxSession.DoesSessionExist() {
		if err := i.tmuxSession.Close(); err != nil {
			log.ErrorLog.Printf("failed to close existing tmux session: %v", err)
			// Continue anyway - we want to create a new session
		}
	}

	// Create new tmux session in the worktree
	if err := i.tmuxSession.Start(worktreePath); err != nil {
		return fmt.Errorf("failed to start new session: %w", err)
	}

	// Handle trust screen asynchronously
	_ = i.tmuxSession.HandleTrustScreenAsync()

	i.SetStatus(Running)
	return nil
}

// UpdateDiffStats updates the git diff statistics for this instance
func (i *Instance) UpdateDiffStats() error {
	if !i.started {
		i.diffStats = nil
		return nil
	}

	if i.Status == Paused {
		// Keep the previous diff stats if the instance is paused
		return nil
	}

	stats := i.gitWorktree.Diff()
	if stats.Error != nil {
		if strings.Contains(stats.Error.Error(), "base commit SHA not set") {
			// Worktree is not fully set up yet, not an error
			i.diffStats = nil
			return nil
		}
		return fmt.Errorf("failed to get diff stats: %w", stats.Error)
	}

	i.diffStats = stats
	return nil
}

// GetDiffStats returns the current git diff statistics
func (i *Instance) GetDiffStats() *git.DiffStats {
	return i.diffStats
}

// SetDiffStats sets the git diff statistics (used for async updates)
func (i *Instance) SetDiffStats(stats *git.DiffStats) {
	i.diffStats = stats
}

// GetGitWorktreeUnsafe returns the git worktree without checking if started
// This is used for async operations that need direct access
func (i *Instance) GetGitWorktreeUnsafe() *git.GitWorktree {
	return i.gitWorktree
}

// UpdatePRInfo updates the PR information for this instance
func (i *Instance) UpdatePRInfo() error {
	if !i.started {
		i.prInfo = nil
		return nil
	}

	// Keep the previous PR info if the instance is paused
	if i.Status == Paused {
		return nil
	}

	info := git.FetchPRInfo(i.gitWorktree.GetRepoPath(), i.gitWorktree.GetBranchName())
	if info.Error != nil {
		// Don't return error - just silently skip if gh CLI is unavailable
		return nil
	}

	i.prInfo = info
	return nil
}

// GetPRInfo returns the current PR information
func (i *Instance) GetPRInfo() *git.PRInfo {
	return i.prInfo
}

// SendPrompt sends a prompt to the tmux session
func (i *Instance) SendPrompt(prompt string) error {
	if !i.started {
		return fmt.Errorf("instance not started")
	}
	if i.tmuxSession == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	if err := i.tmuxSession.SendKeys(prompt); err != nil {
		return fmt.Errorf("error sending keys to tmux session: %w", err)
	}

	// Brief pause to prevent carriage return from being interpreted as newline
	time.Sleep(100 * time.Millisecond)
	if err := i.tmuxSession.TapEnter(); err != nil {
		return fmt.Errorf("error tapping enter: %w", err)
	}

	return nil
}

// SendPromptCommand sends a prompt to the tmux session using tmux send-keys command.
// This is more reliable than PTY-based SendPrompt for injecting input from outside.
func (i *Instance) SendPromptCommand(prompt string) error {
	if !i.started {
		return fmt.Errorf("instance not started")
	}
	if i.tmuxSession == nil {
		return fmt.Errorf("tmux session not initialized")
	}
	return i.tmuxSession.SendKeysCommand(prompt)
}

// PreviewFullHistory captures the entire tmux pane output including full scrollback history
func (i *Instance) PreviewFullHistory() (string, error) {
	if !i.started || i.Status == Paused {
		return "", nil
	}
	return i.tmuxSession.CapturePaneContentWithOptions("-", "-")
}

// SetGitWorktree sets the git worktree (used for review instances with pre-created worktrees)
func (i *Instance) SetGitWorktree(worktree *git.GitWorktree) {
	i.gitWorktree = worktree
}

// SetWorktreeReady marks the worktree as already set up (Setup() called externally).
func (i *Instance) SetWorktreeReady() {
	i.worktreeReady = true
}

// SetTmuxSession sets the tmux session for testing purposes
func (i *Instance) SetTmuxSession(session *tmux.TmuxSession) {
	i.tmuxSession = session
}

// SendKeys sends keys to the tmux session
func (i *Instance) SendKeys(keys string) error {
	if !i.started || i.Status == Paused {
		return fmt.Errorf("cannot send keys to instance that has not been started or is paused")
	}
	return i.tmuxSession.SendKeys(keys)
}

// OpenInEditor opens the worktree directory in the specified editor
func (i *Instance) OpenInEditor(editor string) error {
	if !i.started {
		return fmt.Errorf("cannot open editor: instance has not been started")
	}
	if i.Status == Paused {
		return fmt.Errorf("cannot open editor: instance is paused (no worktree)")
	}
	if editor == "" {
		return fmt.Errorf("no editor configured")
	}

	worktreePath := i.gitWorktree.GetWorktreePath()

	var cmd *exec.Cmd
	editorLower := strings.ToLower(editor)

	// Use 'open -a' on macOS for GUI apps (Cursor, VS Code, etc.)
	if strings.Contains(editorLower, "cursor") {
		cmd = exec.Command("open", "-a", "Cursor", worktreePath)
	} else if strings.Contains(editorLower, "code") || strings.Contains(editorLower, "visual studio") {
		cmd = exec.Command("open", "-a", "Visual Studio Code", worktreePath)
	} else {
		// For other editors, try direct execution
		cmd = exec.Command(editor, worktreePath)
	}

	// Start in background - don't wait for editor to close
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open editor '%s': %w", editor, err)
	}

	return nil
}
