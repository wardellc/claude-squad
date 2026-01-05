package overlay

import (
	"claude-squad/config"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RepoSelectorOverlay represents a repository selection overlay
type RepoSelectorOverlay struct {
	repos         []config.RepoInfo
	selectedIdx   int
	Submitted     bool
	Canceled      bool
	width, height int
}

// NewRepoSelectorOverlay creates a new repository selector overlay
func NewRepoSelectorOverlay(repos []config.RepoInfo) *RepoSelectorOverlay {
	return &RepoSelectorOverlay{
		repos:       repos,
		selectedIdx: 0,
		Submitted:   false,
		Canceled:    false,
	}
}

// SetSize sets the overlay dimensions
func (r *RepoSelectorOverlay) SetSize(width, height int) {
	r.width = width
	r.height = height
}

// HandleKeyPress processes a key press and updates the state accordingly.
// Returns true if the overlay should be closed.
func (r *RepoSelectorOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyUp:
		if r.selectedIdx > 0 {
			r.selectedIdx--
		}
		return false
	case tea.KeyDown:
		if r.selectedIdx < len(r.repos)-1 {
			r.selectedIdx++
		}
		return false
	case tea.KeyEnter:
		r.Submitted = true
		return true
	case tea.KeyEsc:
		r.Canceled = true
		return true
	default:
		// Handle vim-style navigation
		if msg.Type == tea.KeyRunes {
			switch string(msg.Runes) {
			case "j":
				if r.selectedIdx < len(r.repos)-1 {
					r.selectedIdx++
				}
				return false
			case "k":
				if r.selectedIdx > 0 {
					r.selectedIdx--
				}
				return false
			}
		}
		return false
	}
}

// GetSelectedRepo returns the currently selected repository
func (r *RepoSelectorOverlay) GetSelectedRepo() config.RepoInfo {
	if r.selectedIdx >= 0 && r.selectedIdx < len(r.repos) {
		return r.repos[r.selectedIdx]
	}
	return config.RepoInfo{}
}

// IsSubmitted returns whether a selection was made
func (r *RepoSelectorOverlay) IsSubmitted() bool {
	return r.Submitted
}

// IsCanceled returns whether the selection was canceled
func (r *RepoSelectorOverlay) IsCanceled() bool {
	return r.Canceled
}

// Render renders the repository selector overlay
func (r *RepoSelectorOverlay) Render() string {
	// Create styles
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true).
		MarginBottom(1)

	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("0")).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("7"))

	countStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		MarginTop(1)

	// Build the content
	var content strings.Builder
	content.WriteString(titleStyle.Render("Select Repository"))
	content.WriteString("\n\n")

	// Render each repository
	for i, repo := range r.repos {
		// Format: "1. repo-name         (12 instances)"
		prefix := fmt.Sprintf("%d. ", i+1)
		name := repo.Name

		// Count suffix
		var countSuffix string
		if repo.Count == 1 {
			countSuffix = "(1 instance)"
		} else if repo.Count > 1 {
			countSuffix = fmt.Sprintf("(%d instances)", repo.Count)
		}

		// Calculate padding for alignment
		maxNameLen := 25
		if len(name) > maxNameLen {
			name = name[:maxNameLen-3] + "..."
		}
		padding := maxNameLen - len(name)
		if padding < 1 {
			padding = 1
		}

		line := prefix + name + strings.Repeat(" ", padding) + countSuffix

		if i == r.selectedIdx {
			content.WriteString(selectedStyle.Render(" " + line + " "))
		} else {
			styledCount := ""
			if countSuffix != "" {
				styledCount = countStyle.Render(countSuffix)
				line = prefix + name + strings.Repeat(" ", padding)
				content.WriteString(normalStyle.Render(" "+line) + styledCount)
			} else {
				content.WriteString(normalStyle.Render(" " + line + " "))
			}
		}
		content.WriteString("\n")
	}

	// Help text
	content.WriteString(helpStyle.Render("↑/↓ navigate  Enter select  Esc cancel"))

	return style.Render(content.String())
}

// View returns the rendered view (alias for Render for consistency)
func (r *RepoSelectorOverlay) View() string {
	return r.Render()
}
