package keys

import (
	"github.com/charmbracelet/bubbles/key"
)

type KeyName int

const (
	KeyUp KeyName = iota
	KeyDown
	KeyEnter
	KeyNew
	KeyKill
	KeyQuit
	KeyReview    // Review current session (Shift+R)
	KeyNewReview // Open new review dialog (r)
	KeyPush
	KeySubmit

	KeyTab        // Tab is a special keybinding for switching between panes.
	KeySubmitName // SubmitName is a special keybinding for submitting the name of a new instance.

	KeyCheckout
	KeyResume
	KeyRestart    // Restart tmux session for instance (Ctrl+R)
	KeyPrompt     // New key for entering a prompt
	KeyHelp       // Key for showing help screen
	KeyOpenEditor    // Key for opening worktree in editor
	KeyMoveToProgress // Move session from Review back to In Progress (Shift+T)

	// Diff keybindings
	KeyShiftUp
	KeyShiftDown

	KeyJumpToInstance // Jump to instance by number (0-99)
)

// GlobalKeyStringsMap is a global, immutable map string to keybinding.
var GlobalKeyStringsMap = map[string]KeyName{
	"up":         KeyUp,
	"k":          KeyUp,
	"down":       KeyDown,
	"j":          KeyDown,
	"shift+up":   KeyShiftUp,
	"shift+down": KeyShiftDown,
	"N":          KeyPrompt,
	"enter":      KeyEnter,
	"o":          KeyEnter,
	"n":          KeyNew,
	"D":          KeyKill,
	"q":          KeyQuit,
	"tab":        KeyTab,
	"c":          KeyCheckout,
	"r":          KeyNewReview,
	"R":          KeyReview,
	"ctrl+r":     KeyRestart,
	"p":          KeySubmit,
	"?":          KeyHelp,
	"e":          KeyOpenEditor,
	"T":          KeyMoveToProgress,
	"0":          KeyJumpToInstance,
	"1":          KeyJumpToInstance,
	"2":          KeyJumpToInstance,
	"3":          KeyJumpToInstance,
	"4":          KeyJumpToInstance,
	"5":          KeyJumpToInstance,
	"6":          KeyJumpToInstance,
	"7":          KeyJumpToInstance,
	"8":          KeyJumpToInstance,
	"9":          KeyJumpToInstance,
}

// GlobalkeyBindings is a global, immutable map of KeyName tot keybinding.
var GlobalkeyBindings = map[KeyName]key.Binding{
	KeyUp: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	KeyDown: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	KeyShiftUp: key.NewBinding(
		key.WithKeys("shift+up"),
		key.WithHelp("shift+↑", "scroll"),
	),
	KeyShiftDown: key.NewBinding(
		key.WithKeys("shift+down"),
		key.WithHelp("shift+↓", "scroll"),
	),
	KeyEnter: key.NewBinding(
		key.WithKeys("enter", "o"),
		key.WithHelp("↵/o", "open"),
	),
	KeyNew: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new"),
	),
	KeyKill: key.NewBinding(
		key.WithKeys("D"),
		key.WithHelp("D", "kill"),
	),
	KeyHelp: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	KeyQuit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	KeySubmit: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "push branch"),
	),
	KeyPrompt: key.NewBinding(
		key.WithKeys("N"),
		key.WithHelp("N", "new with prompt"),
	),
	KeyCheckout: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "checkout"),
	),
	KeyTab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch tab"),
	),
	KeyResume: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "resume"),
	),
	KeyReview: key.NewBinding(
		key.WithKeys("R"),
		key.WithHelp("R", "review"),
	),
	KeyNewReview: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "new review"),
	),
	KeyRestart: key.NewBinding(
		key.WithKeys("ctrl+r"),
		key.WithHelp("^R", "restart"),
	),
	KeyOpenEditor: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "editor"),
	),
	KeyMoveToProgress: key.NewBinding(
		key.WithKeys("T"),
		key.WithHelp("T", "move to in-progress"),
	),

	// -- Special keybindings --

	KeySubmitName: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "submit name"),
	),
	KeyJumpToInstance: key.NewBinding(
		key.WithKeys("0", "1", "2", "3", "4", "5", "6", "7", "8", "9"),
		key.WithHelp("0-9", "jump"),
	),
}
