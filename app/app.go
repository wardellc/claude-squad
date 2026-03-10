package app

import (
	"claude-squad/config"
	"claude-squad/keys"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/session/git"
	"claude-squad/ui"
	"claude-squad/ui/overlay"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const GlobalInstanceLimit = 30

// Run is the main entrypoint into the application.
func Run(ctx context.Context, program string, autoYes bool, repos []config.RepoInfo) error {
	p := tea.NewProgram(
		newHome(ctx, program, autoYes, repos),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Mouse scroll
	)
	_, err := p.Run()
	return err
}

type state int

const (
	stateDefault state = iota
	// stateNewForm is the state when the new instance form is displayed.
	stateNewForm
	// stateHelp is the state when a help screen is displayed.
	stateHelp
	// stateConfirm is the state when a confirmation modal is displayed.
	stateConfirm
)

type home struct {
	ctx context.Context

	// -- Storage and Configuration --

	program string
	autoYes bool

	// storage is the interface for saving/loading data to/from the app's state
	storage *session.Storage
	// appConfig stores persistent application configuration
	appConfig *config.Config
	// appState stores persistent application state like seen help screens
	appState config.AppState

	// -- State --

	// state is the current discrete state of the application
	state state
	// newInstanceFinalizer is called when an instance is created.
	// It registers the new instance in the list after the instance has been started.
	newInstanceFinalizer func()

	// keySent is used to manage underlining menu items
	keySent bool
	// pendingAction stores the action to execute on confirmation
	pendingAction tea.Cmd

	// digitBuffer accumulates digit keypresses for multi-digit jump (e.g. "11" for session 11)
	digitBuffer string
	// digitSeq is incremented each time a digit is pressed, used to ignore stale debounce timeouts
	digitSeq int

	// -- UI Components --

	// list displays the list of instances
	list *ui.List
	// menu displays the bottom menu
	menu *ui.Menu
	// tabbedWindow displays the tabbed window with preview and diff panes
	tabbedWindow *ui.TabbedWindow
	// errBox displays error messages
	errBox *ui.ErrBox
	// global spinner instance. we plumb this down to where it's needed
	spinner spinner.Model
	// textOverlay displays text information
	textOverlay *overlay.TextOverlay
	// confirmationOverlay displays confirmation modals
	confirmationOverlay *overlay.ConfirmationOverlay
	// instanceFormOverlay handles the unified new instance form
	instanceFormOverlay *overlay.InstanceFormOverlay

	// -- Layout --

	// windowWidth stores the last known window width for mouse event handling
	windowWidth int

	// -- Multi-repository support --

	// repos is the list of available repositories
	repos []config.RepoInfo
	// selectedRepoPath is the path of the currently selected repo for new instances
	selectedRepoPath string
}

func newHome(ctx context.Context, program string, autoYes bool, repos []config.RepoInfo) *home {
	// Load application config
	appConfig := config.LoadConfig()

	// Load application state
	appState := config.LoadState()

	// Initialize storage
	storage, err := session.NewStorage(appState)
	if err != nil {
		fmt.Printf("Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	h := &home{
		ctx:          ctx,
		spinner:      spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane()),
		errBox:       ui.NewErrBox(),
		storage:      storage,
		appConfig:    appConfig,
		program:      program,
		autoYes:      autoYes,
		state:        stateDefault,
		appState:     appState,
		repos:        repos,
	}
	h.list = ui.NewList(&h.spinner, autoYes)

	// Load saved instances
	instances, err := storage.LoadInstances()
	if err != nil {
		fmt.Printf("Failed to load instances: %v\n", err)
		os.Exit(1)
	}

	// Add loaded instances to the list
	for _, instance := range instances {
		// Call the finalizer immediately.
		h.list.AddInstance(instance)()
		if autoYes {
			instance.AutoYes = true
		}
	}

	// Select the first item in display order so the list starts at the top
	h.list.SelectFirstInDisplayOrder()

	return h
}

// updateHandleWindowSizeEvent sets the sizes of the components.
// The components will try to render inside their bounds.
func (m *home) listWidth() int {
	return int(float32(m.windowWidth) * 0.3)
}

func (m *home) updateHandleWindowSizeEvent(msg tea.WindowSizeMsg) {
	m.windowWidth = msg.Width
	// List takes 30% of width, preview takes 70%
	listWidth := int(float32(msg.Width) * 0.3)
	tabsWidth := msg.Width - listWidth

	// Menu takes 10% of height, list and window take 90%
	contentHeight := int(float32(msg.Height) * 0.9)
	menuHeight := msg.Height - contentHeight - 1     // minus 1 for error box
	m.errBox.SetSize(int(float32(msg.Width)*0.9), 1) // error box takes 1 row

	m.tabbedWindow.SetSize(tabsWidth, contentHeight)
	m.list.SetSize(listWidth, contentHeight)

	if m.textOverlay != nil {
		m.textOverlay.SetWidth(int(float32(msg.Width) * 0.6))
	}
	if m.instanceFormOverlay != nil {
		m.instanceFormOverlay.SetSize(int(float32(msg.Width)*0.5), int(float32(msg.Height)*0.6))
	}

	previewWidth, previewHeight := m.tabbedWindow.GetPreviewSize()
	if err := m.list.SetSessionPreviewSize(previewWidth, previewHeight); err != nil {
		log.ErrorLog.Print(err)
	}
	m.menu.SetSize(msg.Width, menuHeight)
}

func (m *home) Init() tea.Cmd {
	// Upon starting, we want to start the spinner. Whenever we get a spinner.TickMsg, we
	// update the spinner, which sends a new spinner.TickMsg. I think this lasts forever lol.
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			time.Sleep(100 * time.Millisecond)
			return previewTickMsg{}
		},
		tickUpdateMetadataCmd,
		// Do an immediate PR check for all loaded instances, then start the recurring timer
		func() tea.Msg {
			// Brief delay to let instances finish starting
			time.Sleep(2 * time.Second)
			return tickUpdatePRInfoMessage{}
		},
	)
}

func (m *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case hideErrMsg:
		m.errBox.Clear()
	case previewTickMsg:
		// Schedule next tick
		nextTick := func() tea.Msg {
			time.Sleep(100 * time.Millisecond)
			return previewTickMsg{}
		}
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nextTick
		}
		// Refresh preview asynchronously to avoid blocking UI
		refreshCmd := func() tea.Msg {
			if err := selected.RefreshPreview(); err != nil {
				log.WarningLog.Printf("failed to refresh preview: %v", err)
			}
			return previewRefreshedMsg{instance: selected}
		}
		return m, tea.Batch(nextTick, refreshCmd)
	case previewRefreshedMsg:
		// Only update preview if the instance is still selected (handles rapid navigation)
		if m.list.GetSelectedInstance() == msg.instance {
			if err := m.tabbedWindow.UpdatePreview(msg.instance); err != nil {
				return m, m.handleError(err)
			}
		}
		return m, nil
	case keyupMsg:
		m.menu.ClearKeydown()
		return m, nil
	case digitTimeoutMsg:
		// Ignore stale timeouts from previous digit sequences
		if msg.seq != m.digitSeq {
			return m, nil
		}
		idx, _ := strconv.Atoi(m.digitBuffer)
		m.digitBuffer = ""
		if m.list.JumpToDisplayIndex(idx) {
			return m, m.instanceChanged()
		}
		return m, nil
	case instanceStartedMsg:
		if msg.err != nil {
			// Instance failed to start - remove it from the list
			m.list.Kill()
			return m, m.handleError(msg.err)
		}
		// Instance started successfully - save and update UI
		if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
			return m, m.handleError(err)
		}
		// Increment repo usage count for sorting
		if m.selectedRepoPath != "" {
			if err := m.appConfig.IncrementRepoUsage(m.selectedRepoPath); err != nil {
				log.WarningLog.Printf("failed to increment repo usage: %v", err)
			}
			m.selectedRepoPath = "" // Clear after use
		}
		m.newInstanceFinalizer()
		if m.autoYes {
			msg.instance.AutoYes = true
		}
		// Trigger PR check for the newly started instance (it may already have a PR from a previous session)
		inst := msg.instance
		prCmd := func() tea.Msg {
			return triggerPRCheckMsg{instance: inst, force: true}
		}
		return m, tea.Batch(tea.WindowSize(), m.instanceChanged(), prCmd)
	case tickUpdateMetadataMessage:
		selected := m.list.GetSelectedInstance()
		var prCheckCmds []tea.Cmd
		for _, instance := range m.list.GetInstances() {
			// Skip instances that are not started, paused, or being deleted
			if !instance.Started() || instance.Paused() || instance.Status == session.Deleting {
				continue
			}
			prevStatus := instance.Status
			updated, prompt, bgTask := instance.HasUpdated()
			if updated || bgTask {
				instance.SetStatus(session.Running)
			} else {
				if prompt {
					instance.TapEnter()
				} else {
					instance.SetStatus(session.Ready)
				}
			}
			// When an instance transitions to Ready (agent finished work),
			// trigger a PR check - the agent may have pushed or created a PR
			if prevStatus == session.Running && instance.Status == session.Ready {
				inst := instance
				prCheckCmds = append(prCheckCmds, func() tea.Msg {
					return triggerPRCheckMsg{instance: inst, force: false}
				})
			}
		}
		// Update diff pane for selected instance (uses cached stats)
		if selected != nil && selected.Status != session.Deleting {
			m.tabbedWindow.UpdateDiff(selected)
		}
		// Run diff stats update asynchronously for selected instance only
		var diffCmd tea.Cmd
		if selected != nil && selected.Started() && !selected.Paused() && selected.Status != session.Deleting {
			inst := selected
			diffCmd = func() tea.Msg {
				worktree := inst.GetGitWorktreeUnsafe()
				if worktree == nil {
					return diffStatsUpdatedMsg{instance: inst, stats: nil, err: nil}
				}
				stats := worktree.Diff()
				return diffStatsUpdatedMsg{instance: inst, stats: stats, err: stats.Error}
			}
		}
		var batchCmds []tea.Cmd
		batchCmds = append(batchCmds, tickUpdateMetadataCmd)
		if diffCmd != nil {
			batchCmds = append(batchCmds, diffCmd)
		}
		batchCmds = append(batchCmds, prCheckCmds...)
		return m, tea.Batch(batchCmds...)
	case tickUpdatePRInfoMessage:
		// Fallback poll: trigger async PR checks for all running instances
		var cmds []tea.Cmd
		cmds = append(cmds, tickUpdatePRInfoCmd)
		for _, instance := range m.list.GetInstances() {
			if !instance.Started() || instance.Paused() {
				continue
			}
			inst := instance
			cmds = append(cmds, func() tea.Msg {
				if err := inst.UpdatePRInfo(false); err != nil {
					log.WarningLog.Printf("could not update PR info: %v", err)
				}
				return prInfoUpdatedMsg{instance: inst}
			})
		}
		return m, tea.Batch(cmds...)
	case triggerPRCheckMsg:
		// Event-driven PR check for a specific instance or all
		var cmds []tea.Cmd
		if msg.instance != nil {
			inst := msg.instance
			force := msg.force
			cmds = append(cmds, func() tea.Msg {
				if err := inst.UpdatePRInfo(force); err != nil {
					log.WarningLog.Printf("could not update PR info: %v", err)
				}
				return prInfoUpdatedMsg{instance: inst}
			})
		} else {
			for _, instance := range m.list.GetInstances() {
				if !instance.Started() || instance.Paused() {
					continue
				}
				inst := instance
				force := msg.force
				cmds = append(cmds, func() tea.Msg {
					if err := inst.UpdatePRInfo(force); err != nil {
						log.WarningLog.Printf("could not update PR info: %v", err)
					}
					return prInfoUpdatedMsg{instance: inst}
				})
			}
		}
		return m, tea.Batch(cmds...)
	case prInfoUpdatedMsg:
		// PR info fetch completed - no special action needed, the instance already has the data
		return m, nil
	case tea.MouseMsg:
		// Handle mouse wheel events for scrolling
		if msg.Action == tea.MouseActionPress {
			if msg.Button == tea.MouseButtonWheelDown || msg.Button == tea.MouseButtonWheelUp {
				// Determine if mouse is over the list panel (left 30% of width)
				listWidth := m.listWidth()
				if msg.X < listWidth {
					// Mouse is over the list panel - move selection up/down
					switch msg.Button {
					case tea.MouseButtonWheelUp:
						m.list.Up()
						return m, m.instanceChanged()
					case tea.MouseButtonWheelDown:
						m.list.Down()
						return m, m.instanceChanged()
					}
				} else {
					// Mouse is over the preview/diff panel
					selected := m.list.GetSelectedInstance()
					if selected == nil || selected.Status == session.Paused {
						return m, nil
					}
					switch msg.Button {
					case tea.MouseButtonWheelUp:
						m.tabbedWindow.ScrollUp()
					case tea.MouseButtonWheelDown:
						m.tabbedWindow.ScrollDown()
					}
				}
			}
		}
		return m, nil
	case diffStatsUpdatedMsg:
		// Only update if the instance is still selected and not being deleted
		selected := m.list.GetSelectedInstance()
		if selected == msg.instance && msg.instance.Status != session.Deleting {
			if msg.err == nil && msg.stats != nil {
				msg.instance.SetDiffStats(msg.stats)
				m.tabbedWindow.UpdateDiff(msg.instance)
			}
		}
		return m, nil
	case instanceDeletionStartedMsg:
		// Set status to Deleting and start async deletion
		msg.instance.SetStatus(session.Deleting)
		inst := msg.instance
		internalName := inst.InternalName
		deleteCmd := func() tea.Msg {
			err := inst.Kill()
			return instanceDeletionCompletedMsg{instance: inst, internalName: internalName, err: err}
		}
		return m, deleteCmd
	case instanceDeletionCompletedMsg:
		if msg.err != nil {
			// Deletion failed - revert status to Ready and show error
			msg.instance.SetStatus(session.Ready)
			return m, m.handleError(msg.err)
		}
		// Deletion succeeded - unregister repo and remove from list
		repoName, err := msg.instance.RepoName()
		if err != nil {
			log.WarningLog.Printf("could not get repo name: %v", err)
		}
		m.list.RemoveInstance(msg.instance)
		if repoName != "" {
			// Note: We need to unregister repo name properly
			// The list's repos map is private, so we rely on invalidateCache
		}
		// Save updated instances
		if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
			return m, m.handleError(err)
		}
		return m, m.instanceChanged()
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		m.updateHandleWindowSizeEvent(msg)
		return m, nil
	case error:
		// Handle errors from confirmation actions
		return m, m.handleError(msg)
	case instanceChangedMsg:
		// Handle instance changed after confirmation action
		return m, m.instanceChanged()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *home) handleQuit() (tea.Model, tea.Cmd) {
	if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
		return m, m.handleError(err)
	}
	return m, tea.Quit
}

func (m *home) handleMenuHighlighting(msg tea.KeyMsg) (cmd tea.Cmd, returnEarly bool) {
	// Handle menu highlighting when you press a button. We intercept it here and immediately return to
	// update the ui while re-sending the keypress. Then, on the next call to this, we actually handle the keypress.
	if m.keySent {
		m.keySent = false
		return nil, false
	}
	if m.state == stateHelp || m.state == stateConfirm || m.state == stateNewForm {
		return nil, false
	}
	// If it's in the global keymap, we should try to highlight it.
	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return nil, false
	}

	if selected := m.list.GetSelectedInstance(); selected != nil && selected.Paused() && name == keys.KeyEnter {
		return nil, false
	}
	if name == keys.KeyShiftDown || name == keys.KeyShiftUp {
		return nil, false
	}

	m.keySent = true
	return tea.Batch(
		func() tea.Msg { return msg },
		m.keydownCallback(name)), true
}

func (m *home) handleKeyPress(msg tea.KeyMsg) (mod tea.Model, cmd tea.Cmd) {
	cmd, returnEarly := m.handleMenuHighlighting(msg)
	if returnEarly {
		return m, cmd
	}

	if m.state == stateHelp {
		return m.handleHelpState(msg)
	}

	// Handle new instance form state
	if m.state == stateNewForm {
		shouldClose := m.instanceFormOverlay.HandleKeyPress(msg)
		if shouldClose {
			if m.instanceFormOverlay.IsSubmitted() {
				name := m.instanceFormOverlay.GetName()
				repo := m.instanceFormOverlay.GetSelectedRepo()
				prompt := m.instanceFormOverlay.GetPrompt()
				baseBranch := m.instanceFormOverlay.GetSelectedBranch()
				permissionMode := m.instanceFormOverlay.GetPermissionMode()

				m.instanceFormOverlay = nil
				return m.createInstanceFromForm(name, repo.Path, prompt, baseBranch, permissionMode)
			}
			// Canceled
			m.instanceFormOverlay = nil
			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			return m, nil
		}
		return m, nil
	}

	// Handle confirmation state
	if m.state == stateConfirm {
		shouldClose := m.confirmationOverlay.HandleKeyPress(msg)
		if shouldClose {
			confirmed := m.confirmationOverlay.IsConfirmed()
			action := m.pendingAction
			m.state = stateDefault
			m.confirmationOverlay = nil
			m.pendingAction = nil
			if confirmed && action != nil {
				return m, action
			}
			return m, nil
		}
		return m, nil
	}

	// Exit scrolling mode when ESC is pressed and preview pane is in scrolling mode
	// Check if Escape key was pressed and we're not in the diff tab (meaning we're in preview tab)
	// Always check for escape key first to ensure it doesn't get intercepted elsewhere
	if msg.Type == tea.KeyEsc {
		// If in preview tab and in scroll mode, exit scroll mode
		if !m.tabbedWindow.IsInDiffTab() && m.tabbedWindow.IsPreviewInScrollMode() {
			// Use the selected instance from the list
			selected := m.list.GetSelectedInstance()
			err := m.tabbedWindow.ResetPreviewToNormalMode(selected)
			if err != nil {
				return m, m.handleError(err)
			}
			return m, m.instanceChanged()
		}
	}

	// Handle quit commands first
	if msg.String() == "ctrl+c" || msg.String() == "q" {
		return m.handleQuit()
	}

	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return m, nil
	}

	switch name {
	case keys.KeyHelp:
		return m.showHelpScreen(helpTypeGeneral{}, nil)
	case keys.KeyPrompt, keys.KeyNew:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}

		// Get the default repo (most recently used or first available)
		var defaultRepo config.RepoInfo
		var repos []config.RepoInfo
		if len(m.repos) > 0 {
			repos = m.appConfig.GetReposSortedByUsage(m.repos)
			defaultRepo = m.appConfig.GetMostRecentRepo(repos)
		}

		// Show the new instance form
		m.instanceFormOverlay = overlay.NewInstanceFormOverlay(repos, defaultRepo)
		m.state = stateNewForm
		m.menu.SetState(ui.StateNewInstance)
		return m, tea.WindowSize()
	case keys.KeyUp:
		m.list.Up()
		return m, m.instanceChanged()
	case keys.KeyDown:
		m.list.Down()
		return m, m.instanceChanged()
	case keys.KeyJumpToInstance:
		m.digitBuffer += msg.String()
		m.digitSeq++
		seq := m.digitSeq
		return m, tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
			return digitTimeoutMsg{seq: seq}
		})
	case keys.KeyShiftUp:
		m.tabbedWindow.ScrollUp()
		return m, m.instanceChanged()
	case keys.KeyShiftDown:
		m.tabbedWindow.ScrollDown()
		return m, m.instanceChanged()
	case keys.KeyTab:
		m.tabbedWindow.Toggle()
		m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
		return m, m.instanceChanged()
	case keys.KeyOpenEditor:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		if m.appConfig.Editor == "" {
			return m, m.handleError(fmt.Errorf("no editor configured in ~/.claude-squad/config.json"))
		}
		if err := selected.OpenInEditor(m.appConfig.Editor); err != nil {
			return m, m.handleError(err)
		}
		return m, nil
	case keys.KeyKill:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Don't allow deleting an instance that is already being deleted
		if selected.Status == session.Deleting {
			return m, nil
		}

		// Create the kill action as a tea.Cmd that triggers async deletion
		inst := selected
		killAction := func() tea.Msg {
			// Check if branch is checked out before starting deletion
			worktree, err := inst.GetGitWorktree()
			if err != nil {
				return err
			}

			checkedOut, err := worktree.IsBranchCheckedOut()
			if err != nil {
				return err
			}

			if checkedOut {
				return fmt.Errorf("instance %s is currently checked out", inst.Title)
			}

			// Return message to start async deletion
			return instanceDeletionStartedMsg{instance: inst}
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Kill session '%s'?", selected.Title)
		return m, m.confirmAction(message, killAction)
	case keys.KeySubmit:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the push action as a tea.Cmd
		inst := selected
		pushAction := func() tea.Msg {
			// Default commit message with timestamp
			commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s", inst.Title, time.Now().Format(time.RFC822))
			worktree, err := inst.GetGitWorktree()
			if err != nil {
				return err
			}
			if err = worktree.PushChanges(commitMsg, true); err != nil {
				return err
			}
			// After successful push, trigger a PR check
			return triggerPRCheckMsg{instance: inst, force: true}
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Push changes from session '%s'?", selected.Title)
		return m, m.confirmAction(message, pushAction)
	case keys.KeyCheckout:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Show help screen before pausing
		return m.showHelpScreen(helpTypeInstanceCheckout{}, func() tea.Cmd {
			if err := selected.Pause(); err != nil {
				return m.handleError(err)
			}
			return m.instanceChanged()
		})
	case keys.KeyResume:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		if err := selected.Resume(); err != nil {
			return m, m.handleError(err)
		}
		return m, tea.WindowSize()
	case keys.KeyRestart:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		if err := selected.Restart(); err != nil {
			return m, m.handleError(err)
		}
		return m, tea.WindowSize()
	case keys.KeyEnter:
		if m.list.NumInstances() == 0 {
			return m, nil
		}
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Paused() || !selected.TmuxAlive() {
			return m, nil
		}
		// Show help screen before attaching
		return m.showHelpScreen(helpTypeInstanceAttach{}, func() tea.Cmd {
			ch, err := m.list.Attach()
			if err != nil {
				return m.handleError(err)
			}
			<-ch
			m.state = stateDefault
			return nil
		})
	default:
		return m, nil
	}
}

// instanceChanged updates the preview pane, menu, and diff pane based on the selected instance. It returns an error
// Cmd if there was any error.
func (m *home) instanceChanged() tea.Cmd {
	// selected may be nil
	selected := m.list.GetSelectedInstance()

	// Only do fast in-memory updates here - preview and diff are updated asynchronously
	// via previewRefreshedMsg (100ms ticker) and tickUpdateMetadataMessage (500ms ticker)
	m.tabbedWindow.SetInstance(selected)
	m.menu.SetInstance(selected)
	return nil
}

type keyupMsg struct{}

// digitTimeoutMsg is sent after the digit debounce period expires.
type digitTimeoutMsg struct {
	seq int // sequence number to ignore stale timeouts
}

// keydownCallback clears the menu option highlighting after 500ms.
func (m *home) keydownCallback(name keys.KeyName) tea.Cmd {
	m.menu.Keydown(name)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}

		return keyupMsg{}
	}
}

// hideErrMsg implements tea.Msg and clears the error text from the screen.
type hideErrMsg struct{}

// previewTickMsg implements tea.Msg and triggers a preview update
type previewTickMsg struct{}

// previewRefreshedMsg is sent when async preview refresh completes
type previewRefreshedMsg struct {
	instance *session.Instance
}

type tickUpdateMetadataMessage struct{}

type instanceChangedMsg struct{}

// instanceStartedMsg is sent when an async instance start completes
type instanceStartedMsg struct {
	instance *session.Instance
	err      error
}

// instanceDeletionStartedMsg is sent when deletion starts (to set Deleting status)
type instanceDeletionStartedMsg struct {
	instance *session.Instance
}

// instanceDeletionCompletedMsg is sent when async deletion completes
type instanceDeletionCompletedMsg struct {
	instance     *session.Instance
	internalName string
	err          error
}

// diffStatsUpdatedMsg is sent when async diff stats update completes
type diffStatsUpdatedMsg struct {
	instance *session.Instance
	stats    *git.DiffStats
	err      error
}

// prInfoUpdatedMsg is sent when async PR info fetch completes
type prInfoUpdatedMsg struct {
	instance *session.Instance
}

// triggerPRCheckMsg requests a PR check for a specific instance (or all if nil)
type triggerPRCheckMsg struct {
	instance *session.Instance
	force    bool
}

// tickUpdateMetadataCmd is the callback to update the metadata of the instances.
// Runs every 5 seconds so status icons stay responsive.
var tickUpdateMetadataCmd = func() tea.Msg {
	time.Sleep(5 * time.Second)
	return tickUpdateMetadataMessage{}
}

type tickUpdatePRInfoMessage struct{}

// tickUpdatePRInfoCmd updates PR info every 3 minutes as a fallback poll.
// Most PR updates are triggered by events (push, agent finishing work).
var tickUpdatePRInfoCmd = func() tea.Msg {
	time.Sleep(3 * time.Minute)
	return tickUpdatePRInfoMessage{}
}

// handleError handles all errors which get bubbled up to the app. sets the error message. We return a callback tea.Cmd that returns a hideErrMsg message
// which clears the error message after 3 seconds.
func (m *home) handleError(err error) tea.Cmd {
	log.ErrorLog.Printf("%v", err)
	m.errBox.SetError(err)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(3 * time.Second):
		}

		return hideErrMsg{}
	}
}

// createInstanceFromForm creates a new instance from the form values
func (m *home) createInstanceFromForm(name, repoPath, prompt, baseBranch, permissionMode string) (tea.Model, tea.Cmd) {
	// Use current directory if no repo path
	if repoPath == "" {
		repoPath = "."
	}

	instance, err := session.NewInstance(session.InstanceOptions{
		Title:          name,
		Path:           repoPath,
		Program:        m.program,
		PermissionMode: permissionMode,
		BaseBranch:     baseBranch,
	})
	if err != nil {
		return m, m.handleError(err)
	}

	m.selectedRepoPath = repoPath
	instance.Prompt = prompt

	// Set InternalName for uniqueness: "{repoName}_{title}"
	repoName := filepath.Base(repoPath)
	instance.InternalName = fmt.Sprintf("%s_%s", repoName, name)

	m.newInstanceFinalizer = m.list.AddInstance(instance)
	m.list.SetSelectedInstance(m.list.NumInstances() - 1)

	// Set status to Loading while worktree is being created
	instance.SetStatus(session.Loading)

	// Update last used repo
	if repoPath != "" && repoPath != "." {
		if err := m.appConfig.SetLastUsedRepo(repoPath); err != nil {
			log.WarningLog.Printf("failed to set last used repo: %v", err)
		}
	}

	m.state = stateDefault
	m.menu.SetState(ui.StateDefault)

	// Start the instance asynchronously
	startCmd := func() tea.Msg {
		err := instance.Start(true)
		return instanceStartedMsg{instance: instance, err: err}
	}

	return m, tea.Batch(tea.WindowSize(), startCmd)
}

// confirmAction shows a confirmation modal and stores the action to execute on confirm
func (m *home) confirmAction(message string, action tea.Cmd) tea.Cmd {
	m.state = stateConfirm
	m.pendingAction = action

	// Create and show the confirmation overlay using ConfirmationOverlay
	m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	// Set a fixed width for consistent appearance
	m.confirmationOverlay.SetWidth(50)

	return nil
}

func (m *home) View() string {
	listWithPadding := lipgloss.NewStyle().PaddingTop(1).Render(m.list.String())
	previewWithPadding := lipgloss.NewStyle().PaddingTop(1).Render(m.tabbedWindow.String())
	listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top, listWithPadding, previewWithPadding)

	mainView := lipgloss.JoinVertical(
		lipgloss.Center,
		listAndPreview,
		m.menu.String(),
		m.errBox.String(),
	)

	if m.state == stateNewForm {
		if m.instanceFormOverlay == nil {
			log.ErrorLog.Printf("instance form overlay is nil")
			return mainView
		}
		return overlay.PlaceOverlay(0, 0, m.instanceFormOverlay.Render(), mainView, true, true)
	} else if m.state == stateHelp {
		if m.textOverlay == nil {
			log.ErrorLog.Printf("text overlay is nil")
			return mainView
		}
		return overlay.PlaceOverlay(0, 0, m.textOverlay.Render(), mainView, true, true)
	} else if m.state == stateConfirm {
		if m.confirmationOverlay == nil {
			log.ErrorLog.Printf("confirmation overlay is nil")
			return mainView
		}
		return overlay.PlaceOverlay(0, 0, m.confirmationOverlay.Render(), mainView, true, true)
	}

	return mainView
}
