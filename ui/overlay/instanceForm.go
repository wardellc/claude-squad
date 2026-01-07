package overlay

import (
	"claude-squad/config"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FormField represents which field is focused
type FormField int

const (
	FieldName FormField = iota
	FieldRepo
	FieldPrompt
	FieldDangerouslySkipPermissions
)

// InstanceFormOverlay represents the unified instance creation form
type InstanceFormOverlay struct {
	// Field values
	nameInput                  textinput.Model
	selectedRepo               config.RepoInfo
	promptInput                textinput.Model
	dangerouslySkipPermissions bool

	// Available repos
	repos []config.RepoInfo

	// Focus management
	focusedField FormField

	// Repo search mode
	searchMode    bool
	searchQuery   string
	filteredRepos []config.RepoInfo
	searchIndex   int

	// Form state
	submitted bool
	canceled  bool

	// Dimensions
	width, height int
}

// NewInstanceFormOverlay creates a new instance form overlay
func NewInstanceFormOverlay(repos []config.RepoInfo, defaultRepo config.RepoInfo) *InstanceFormOverlay {
	// Initialize name input
	nameInput := textinput.New()
	nameInput.Placeholder = "Enter instance name..."
	nameInput.CharLimit = 32
	nameInput.Prompt = "" // Remove the default chevron
	nameInput.Focus()

	// Initialize prompt input (single line)
	promptInput := textinput.New()
	promptInput.Placeholder = "Optional: Enter initial prompt..."
	promptInput.Prompt = "" // Remove the default chevron
	promptInput.CharLimit = 0
	promptInput.Blur()

	return &InstanceFormOverlay{
		nameInput:                  nameInput,
		selectedRepo:               defaultRepo,
		promptInput:                promptInput,
		dangerouslySkipPermissions: true, // Default to true
		repos:                      repos,
		focusedField:               FieldName,
		searchMode:                 false,
		filteredRepos:              repos,
		searchIndex:                0,
		submitted:                  false,
		canceled:                   false,
	}
}

// SetSize sets the overlay dimensions
func (f *InstanceFormOverlay) SetSize(width, height int) {
	f.width = width
	f.height = height
	f.nameInput.Width = width - 10
	f.promptInput.Width = width - 10
}

// HandleKeyPress processes a key press and updates the state accordingly.
// Returns true if the overlay should be closed.
func (f *InstanceFormOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Handle Ctrl+C globally
	if msg.Type == tea.KeyCtrlC {
		f.canceled = true
		return true
	}

	// Handle Escape - context sensitive
	if msg.Type == tea.KeyEsc {
		if f.searchMode {
			f.searchMode = false
			f.searchQuery = ""
			f.filteredRepos = f.repos
			f.searchIndex = 0
			return false
		}
		f.canceled = true
		return true
	}

	// Handle Ctrl+Enter to submit (check multiple ways it can be sent)
	if !f.searchMode {
		// Ctrl+Enter can be sent as Ctrl+J, or as a special key
		if msg.Type == tea.KeyCtrlJ || msg.String() == "ctrl+enter" {
			if f.validate() {
				f.submitted = true
				return true
			}
			return false
		}
	}

	// In search mode, handle search-specific keys
	if f.searchMode {
		return f.handleSearchModeKey(msg)
	}

	// Handle Up/Down arrow keys for field navigation
	if msg.Type == tea.KeyUp {
		f.prevField()
		return false
	}
	if msg.Type == tea.KeyDown {
		f.nextField()
		return false
	}

	// Delegate to focused field handler
	switch f.focusedField {
	case FieldName:
		return f.handleNameFieldKey(msg)
	case FieldRepo:
		return f.handleRepoFieldKey(msg)
	case FieldPrompt:
		return f.handlePromptFieldKey(msg)
	case FieldDangerouslySkipPermissions:
		return f.handleDangerouslySkipPermissionsKey(msg)
	}

	return false
}

func (f *InstanceFormOverlay) nextField() {
	f.focusedField = (f.focusedField + 1) % 4
	f.updateFieldFocus()
}

func (f *InstanceFormOverlay) prevField() {
	f.focusedField = (f.focusedField + 3) % 4 // +3 is same as -1 mod 4
	f.updateFieldFocus()
}

func (f *InstanceFormOverlay) updateFieldFocus() {
	switch f.focusedField {
	case FieldName:
		f.nameInput.Focus()
		f.promptInput.Blur()
	case FieldRepo:
		f.nameInput.Blur()
		f.promptInput.Blur()
	case FieldPrompt:
		f.nameInput.Blur()
		f.promptInput.Focus()
	case FieldDangerouslySkipPermissions:
		f.nameInput.Blur()
		f.promptInput.Blur()
	}
}

func (f *InstanceFormOverlay) handleNameFieldKey(msg tea.KeyMsg) bool {
	// Enter submits the form if valid
	if msg.Type == tea.KeyEnter {
		if f.validate() {
			f.submitted = true
			return true
		}
		return false
	}

	// Pass other keys to the text input (except arrow keys which are handled above)
	f.nameInput, _ = f.nameInput.Update(msg)
	return false
}

func (f *InstanceFormOverlay) handleRepoFieldKey(msg tea.KeyMsg) bool {
	// Tab opens search mode (instead of moving to next field)
	if msg.Type == tea.KeyTab {
		if len(f.repos) > 0 {
			f.searchMode = true
			f.searchQuery = ""
			f.filteredRepos = f.repos
			// Try to find current selection in filtered list
			f.searchIndex = 0
			for i, repo := range f.filteredRepos {
				if repo.Path == f.selectedRepo.Path {
					f.searchIndex = i
					break
				}
			}
		}
		return false
	}

	// Enter submits the form if valid
	if msg.Type == tea.KeyEnter {
		if f.validate() {
			f.submitted = true
			return true
		}
		return false
	}

	return false
}

func (f *InstanceFormOverlay) handleSearchModeKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyUp:
		if f.searchIndex > 0 {
			f.searchIndex--
		}
		return false
	case tea.KeyDown:
		if f.searchIndex < len(f.filteredRepos)-1 {
			f.searchIndex++
		}
		return false
	case tea.KeyEnter:
		// Select current repo and exit search mode
		if f.searchIndex >= 0 && f.searchIndex < len(f.filteredRepos) {
			f.selectedRepo = f.filteredRepos[f.searchIndex]
		}
		f.searchMode = false
		f.searchQuery = ""
		f.filteredRepos = f.repos
		return false
	case tea.KeyBackspace:
		if len(f.searchQuery) > 0 {
			f.searchQuery = f.searchQuery[:len(f.searchQuery)-1]
			f.filterRepos()
		}
		return false
	case tea.KeyRunes:
		// Handle vim-style navigation in search mode
		if len(msg.Runes) == 1 {
			switch string(msg.Runes) {
			case "j":
				if f.searchIndex < len(f.filteredRepos)-1 {
					f.searchIndex++
				}
				return false
			case "k":
				if f.searchIndex > 0 {
					f.searchIndex--
				}
				return false
			}
		}
		// Add character to search query
		f.searchQuery += string(msg.Runes)
		f.filterRepos()
		return false
	case tea.KeySpace:
		f.searchQuery += " "
		f.filterRepos()
		return false
	}

	return false
}

func (f *InstanceFormOverlay) handlePromptFieldKey(msg tea.KeyMsg) bool {
	// Enter on prompt field submits (if valid)
	if msg.Type == tea.KeyEnter {
		if f.validate() {
			f.submitted = true
			return true
		}
		return false
	}

	// Pass other keys to the text input
	f.promptInput, _ = f.promptInput.Update(msg)
	return false
}

func (f *InstanceFormOverlay) handleDangerouslySkipPermissionsKey(msg tea.KeyMsg) bool {
	// Tab toggles the value
	if msg.Type == tea.KeyTab {
		f.dangerouslySkipPermissions = !f.dangerouslySkipPermissions
		return false
	}

	// Enter submits the form if valid
	if msg.Type == tea.KeyEnter {
		if f.validate() {
			f.submitted = true
			return true
		}
		return false
	}

	return false
}

func (f *InstanceFormOverlay) filterRepos() {
	if f.searchQuery == "" {
		f.filteredRepos = f.repos
	} else {
		query := strings.ToLower(f.searchQuery)
		f.filteredRepos = nil
		for _, repo := range f.repos {
			if strings.Contains(strings.ToLower(repo.Name), query) ||
				strings.Contains(strings.ToLower(repo.Path), query) {
				f.filteredRepos = append(f.filteredRepos, repo)
			}
		}
	}

	// Reset selection index if out of bounds
	if f.searchIndex >= len(f.filteredRepos) {
		if len(f.filteredRepos) > 0 {
			f.searchIndex = len(f.filteredRepos) - 1
		} else {
			f.searchIndex = 0
		}
	}
}

func (f *InstanceFormOverlay) validate() bool {
	// Name is required
	name := strings.TrimSpace(f.nameInput.Value())
	if name == "" {
		return false
	}

	// If repos are configured, one must be selected
	if len(f.repos) > 0 && f.selectedRepo.Path == "" {
		return false
	}

	return true
}

// GetName returns the entered instance name
func (f *InstanceFormOverlay) GetName() string {
	return strings.TrimSpace(f.nameInput.Value())
}

// GetSelectedRepo returns the selected repository
func (f *InstanceFormOverlay) GetSelectedRepo() config.RepoInfo {
	return f.selectedRepo
}

// GetPrompt returns the entered prompt
func (f *InstanceFormOverlay) GetPrompt() string {
	return f.promptInput.Value()
}

// GetDangerouslySkipPermissions returns whether to skip permissions
func (f *InstanceFormOverlay) GetDangerouslySkipPermissions() bool {
	return f.dangerouslySkipPermissions
}

// IsSubmitted returns whether the form was submitted
func (f *InstanceFormOverlay) IsSubmitted() bool {
	return f.submitted
}

// IsCanceled returns whether the form was canceled
func (f *InstanceFormOverlay) IsCanceled() bool {
	return f.canceled
}

// Render renders the instance form overlay
func (f *InstanceFormOverlay) Render() string {
	// Create styles
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	// Set width on the style before rendering (not after) to avoid border wrapping issues
	if f.width > 0 {
		style = style.Width(f.width - 4) // Account for border and padding
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("7"))

	focusedLabelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))

	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("0")).
		Bold(true)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		MarginTop(1)

	// Build content
	var content strings.Builder
	content.WriteString(titleStyle.Render("Create New Instance"))
	content.WriteString("\n\n")

	// Name field
	if f.focusedField == FieldName {
		content.WriteString(focusedLabelStyle.Render("Name: "))
	} else {
		content.WriteString(labelStyle.Render("Name: "))
	}
	content.WriteString(f.nameInput.View())
	content.WriteString("\n\n")

	// Repository field
	if f.focusedField == FieldRepo {
		content.WriteString(focusedLabelStyle.Render("Repository: "))
	} else {
		content.WriteString(labelStyle.Render("Repository: "))
	}

	if f.searchMode {
		// Show search input and filtered list
		searchInput := f.searchQuery
		if searchInput == "" {
			searchInput = dimStyle.Render("Type to search...")
		}
		content.WriteString(searchInput)
		content.WriteString("\n")

		// Show filtered repos (max 5)
		maxVisible := 5
		if len(f.filteredRepos) < maxVisible {
			maxVisible = len(f.filteredRepos)
		}

		// Calculate visible window around selection
		startIdx := 0
		if f.searchIndex >= maxVisible {
			startIdx = f.searchIndex - maxVisible + 1
		}
		endIdx := startIdx + maxVisible
		if endIdx > len(f.filteredRepos) {
			endIdx = len(f.filteredRepos)
			startIdx = endIdx - maxVisible
			if startIdx < 0 {
				startIdx = 0
			}
		}

		for i := startIdx; i < endIdx; i++ {
			repo := f.filteredRepos[i]
			name := repo.Name
			if len(name) > 30 {
				name = name[:27] + "..."
			}

			if i == f.searchIndex {
				content.WriteString(selectedStyle.Render(" > " + name + " "))
			} else {
				content.WriteString(dimStyle.Render("   " + name))
			}
			content.WriteString("\n")
		}

		if len(f.filteredRepos) == 0 {
			content.WriteString(dimStyle.Render("   No matching repositories"))
			content.WriteString("\n")
		}
	} else {
		// Show selected repo
		if f.selectedRepo.Name != "" {
			content.WriteString(f.selectedRepo.Name)
			if f.focusedField == FieldRepo {
				content.WriteString(dimStyle.Render("  [Tab to change]"))
			}
		} else if len(f.repos) > 0 {
			content.WriteString(dimStyle.Render("Press Tab to select"))
		} else {
			content.WriteString(dimStyle.Render("(current directory)"))
		}
		content.WriteString("\n")
	}
	content.WriteString("\n")

	// Prompt field
	if f.focusedField == FieldPrompt {
		content.WriteString(focusedLabelStyle.Render("Prompt: "))
	} else {
		content.WriteString(labelStyle.Render("Prompt: "))
	}
	content.WriteString(f.promptInput.View())
	content.WriteString("\n\n")

	// Skip Permissions field
	if f.focusedField == FieldDangerouslySkipPermissions {
		content.WriteString(focusedLabelStyle.Render("Skip Permissions: "))
	} else {
		content.WriteString(labelStyle.Render("Skip Permissions: "))
	}
	if f.dangerouslySkipPermissions {
		content.WriteString("[Yes]")
	} else {
		content.WriteString("[No]")
	}
	if f.focusedField == FieldDangerouslySkipPermissions {
		content.WriteString(dimStyle.Render("  [Tab to toggle]"))
	}
	content.WriteString("\n")

	// Help text
	var helpText string
	if f.searchMode {
		helpText = "Type to filter | ↑/↓ navigate | Enter select | Esc cancel"
	} else {
		helpText = "↑/↓ navigate | Enter: submit | Esc: cancel"
	}
	content.WriteString(helpStyle.Render(helpText))

	// Validation hint
	if f.GetName() == "" && f.focusedField != FieldName {
		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Name is required"))
	}

	return style.Render(content.String())
}

// View returns the rendered view (alias for Render)
func (f *InstanceFormOverlay) View() string {
	return f.Render()
}

// GetRepos returns the list of available repositories
func (f *InstanceFormOverlay) GetRepos() []config.RepoInfo {
	return f.repos
}

// SetSelectedRepoByPath sets the selected repo by its path
func (f *InstanceFormOverlay) SetSelectedRepoByPath(path string) {
	for _, repo := range f.repos {
		if repo.Path == path {
			f.selectedRepo = repo
			return
		}
	}
}

// HasRepos returns true if there are repos configured
func (f *InstanceFormOverlay) HasRepos() bool {
	return len(f.repos) > 0
}

// GetRepoPath returns the selected repo path or empty string
func (f *InstanceFormOverlay) GetRepoPath() string {
	return f.selectedRepo.Path
}
