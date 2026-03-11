package overlay

import (
	"claude-squad/config"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ReviewFormField represents which field is focused in the review form
type ReviewFormField int

const (
	ReviewFieldTarget ReviewFormField = iota
	ReviewFieldRepo
)

// ReviewFormOverlay represents the review creation form
type ReviewFormOverlay struct {
	// Field values
	targetInput  textinput.Model // PR number or branch name
	selectedRepo config.RepoInfo

	// Available repos
	repos []config.RepoInfo

	// Focus management
	focusedField ReviewFormField

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

// NewReviewFormOverlay creates a new review form overlay
func NewReviewFormOverlay(repos []config.RepoInfo, defaultRepo config.RepoInfo) *ReviewFormOverlay {
	targetInput := textinput.New()
	targetInput.Placeholder = "PR number or branch name..."
	targetInput.CharLimit = 128
	targetInput.Prompt = ""
	targetInput.Focus()

	return &ReviewFormOverlay{
		targetInput:   targetInput,
		selectedRepo:  defaultRepo,
		repos:         repos,
		focusedField:  ReviewFieldTarget,
		searchMode:    false,
		filteredRepos: repos,
		searchIndex:   0,
		submitted:     false,
		canceled:      false,
	}
}

// SetSize sets the overlay dimensions
func (f *ReviewFormOverlay) SetSize(width, height int) {
	f.width = width
	f.height = height
	f.targetInput.Width = width - 10
}

// HandleKeyPress processes a key press and returns true if the overlay should close
func (f *ReviewFormOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyCtrlC {
		f.canceled = true
		return true
	}

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

	if f.searchMode {
		return f.handleSearchModeKey(msg)
	}

	// Handle Up/Down for field navigation
	if msg.Type == tea.KeyUp {
		f.prevField()
		return false
	}
	if msg.Type == tea.KeyDown {
		f.nextField()
		return false
	}

	switch f.focusedField {
	case ReviewFieldTarget:
		return f.handleTargetFieldKey(msg)
	case ReviewFieldRepo:
		return f.handleRepoFieldKey(msg)
	}

	return false
}

func (f *ReviewFormOverlay) nextField() {
	f.focusedField = (f.focusedField + 1) % 2
	f.updateFieldFocus()
}

func (f *ReviewFormOverlay) prevField() {
	f.focusedField = (f.focusedField + 1) % 2
	f.updateFieldFocus()
}

func (f *ReviewFormOverlay) updateFieldFocus() {
	switch f.focusedField {
	case ReviewFieldTarget:
		f.targetInput.Focus()
	case ReviewFieldRepo:
		f.targetInput.Blur()
	}
}

func (f *ReviewFormOverlay) handleTargetFieldKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyEnter {
		if f.validate() {
			f.submitted = true
			return true
		}
		return false
	}
	f.targetInput, _ = f.targetInput.Update(msg)
	return false
}

func (f *ReviewFormOverlay) handleRepoFieldKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyTab {
		if len(f.repos) > 0 {
			f.searchMode = true
			f.searchQuery = ""
			f.filteredRepos = f.repos
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

	if msg.Type == tea.KeyEnter {
		if f.validate() {
			f.submitted = true
			return true
		}
		return false
	}

	return false
}

func (f *ReviewFormOverlay) handleSearchModeKey(msg tea.KeyMsg) bool {
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

func (f *ReviewFormOverlay) filterRepos() {
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
	f.searchIndex = 0
}

func (f *ReviewFormOverlay) validate() bool {
	target := strings.TrimSpace(f.targetInput.Value())
	if target == "" {
		return false
	}
	if len(f.repos) > 0 && f.selectedRepo.Path == "" {
		return false
	}
	return true
}

// GetTarget returns the entered PR number or branch name
func (f *ReviewFormOverlay) GetTarget() string {
	return strings.TrimSpace(f.targetInput.Value())
}

// GetSelectedRepo returns the selected repository
func (f *ReviewFormOverlay) GetSelectedRepo() config.RepoInfo {
	return f.selectedRepo
}

// IsSubmitted returns whether the form was submitted
func (f *ReviewFormOverlay) IsSubmitted() bool {
	return f.submitted
}

// IsCanceled returns whether the form was canceled
func (f *ReviewFormOverlay) IsCanceled() bool {
	return f.canceled
}

// IsPRNumber returns true if the target looks like a PR number (all digits)
func (f *ReviewFormOverlay) IsPRNumber() bool {
	target := f.GetTarget()
	if target == "" {
		return false
	}
	for _, r := range target {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Render renders the review form overlay
func (f *ReviewFormOverlay) Render() string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	if f.width > 0 {
		style = style.Width(f.width - 4)
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

	var content strings.Builder
	content.WriteString(titleStyle.Render("Create Review"))
	content.WriteString("\n\n")

	// Target field
	if f.focusedField == ReviewFieldTarget {
		content.WriteString(focusedLabelStyle.Render("PR / Branch: "))
	} else {
		content.WriteString(labelStyle.Render("PR / Branch: "))
	}
	content.WriteString(f.targetInput.View())
	content.WriteString("\n\n")

	// Repository field
	if f.focusedField == ReviewFieldRepo {
		content.WriteString(focusedLabelStyle.Render("Repository: "))
	} else {
		content.WriteString(labelStyle.Render("Repository: "))
	}

	if f.searchMode {
		searchInput := f.searchQuery
		if searchInput == "" {
			searchInput = dimStyle.Render("Type to search...")
		}
		content.WriteString(searchInput)
		content.WriteString("\n")

		maxVisible := 5
		if len(f.filteredRepos) < maxVisible {
			maxVisible = len(f.filteredRepos)
		}

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
		if f.selectedRepo.Name != "" {
			content.WriteString(f.selectedRepo.Name)
			if f.focusedField == ReviewFieldRepo {
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

	// Help text
	var helpText string
	if f.searchMode {
		helpText = "Type to filter | ↑/↓ navigate | Enter select | Esc cancel"
	} else {
		helpText = "↑/↓ navigate | Enter: submit | Esc: cancel"
	}
	content.WriteString(helpStyle.Render(helpText))

	// Validation hint
	if f.GetTarget() == "" && f.focusedField != ReviewFieldTarget {
		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("PR number or branch name is required"))
	}

	return style.Render(content.String())
}
