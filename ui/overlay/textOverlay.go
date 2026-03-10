package overlay

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TextOverlay represents a text screen overlay
type TextOverlay struct {
	// Whether the overlay has been dismissed
	Dismissed bool
	// Callback function to be called when the overlay is dismissed.
	// Returns a tea.Cmd that should be executed after dismissal.
	OnDismiss func() tea.Cmd
	// Content to display in the overlay
	content string
	// dismissCmd stores the tea.Cmd returned by OnDismiss for later retrieval.
	dismissCmd tea.Cmd

	width int
}

// NewTextOverlay creates a new text screen overlay with the given title and content
func NewTextOverlay(content string) *TextOverlay {
	return &TextOverlay{
		Dismissed: false,
		content:   content,
	}
}

// HandleKeyPress processes a key press and updates the state
// Returns true if the overlay should be closed
func (t *TextOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Close on any key
	t.Dismissed = true
	// Call the OnDismiss callback if it exists
	if t.OnDismiss != nil {
		t.dismissCmd = t.OnDismiss()
	}
	return true
}

// DismissCmd returns the tea.Cmd from the OnDismiss callback, if any.
func (t *TextOverlay) DismissCmd() tea.Cmd {
	return t.dismissCmd
}

// Render renders the text overlay
func (t *TextOverlay) Render(opts ...WhitespaceOption) string {
	// Create styles
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(t.width)

	// Apply the border style and return
	return style.Render(t.content)
}

func (t *TextOverlay) SetWidth(width int) {
	t.width = width
}
