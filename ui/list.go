package ui

import (
	"claude-squad/log"
	"claude-squad/session"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const readyIcon = "● "
const pausedIcon = "⏸ "

var readyStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#51bd73", Dark: "#51bd73"})

var addedLinesStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#51bd73", Dark: "#51bd73"})

var removedLinesStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#de613e"))

var pausedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#888888"})

var titleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"})

var listDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#A49FA5", Dark: "#777777"})

var selectedTitleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"})

var selectedDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"})

var mainTitle = lipgloss.NewStyle().
	Background(lipgloss.Color("62")).
	Foreground(lipgloss.Color("230"))

var autoYesStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.Color("#1a1a1a"))

var groupHeaderStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"})

// repoGroup represents a group of instances for a single repository
type repoGroup struct {
	repoName  string
	instances []*session.Instance
}

type List struct {
	items         []*session.Instance
	selectedIdx   int
	height, width int
	renderer      *InstanceRenderer
	autoyes       bool

	// map of repo name to number of instances using it. Used to display the repo name only if there are
	// multiple repos in play.
	repos map[string]int

	// Cache for grouped instances (performance optimization)
	cachedGroups   []repoGroup
	cachedOrder    []*session.Instance
	cachedIndexMap map[*session.Instance]int
	cacheValid     bool
}

func NewList(spinner *spinner.Model, autoYes bool) *List {
	return &List{
		items:    []*session.Instance{},
		renderer: &InstanceRenderer{spinner: spinner},
		repos:    make(map[string]int),
		autoyes:  autoYes,
	}
}

// invalidateCache marks the grouped instances cache as stale
func (l *List) invalidateCache() {
	l.cacheValid = false
}

// ensureCacheValid rebuilds the cache if it's stale
func (l *List) ensureCacheValid() {
	if l.cacheValid {
		return
	}
	l.cachedGroups = l.buildGroupedInstances()
	l.cachedOrder = l.buildDisplayOrder(l.cachedGroups)
	l.cachedIndexMap = make(map[*session.Instance]int, len(l.items))
	for i, inst := range l.items {
		l.cachedIndexMap[inst] = i
	}
	l.cacheValid = true
}

// SetSize sets the height and width of the list.
func (l *List) SetSize(width, height int) {
	l.width = width
	l.height = height
	l.renderer.setWidth(width)
}

// SetSessionPreviewSize sets the height and width for the tmux sessions. This makes the stdout line have the correct
// width and height.
func (l *List) SetSessionPreviewSize(width, height int) (err error) {
	for i, item := range l.items {
		if !item.Started() || item.Paused() {
			continue
		}

		if innerErr := item.SetPreviewSize(width, height); innerErr != nil {
			err = errors.Join(
				err, fmt.Errorf("could not set preview size for instance %d: %v", i, innerErr))
		}
	}
	return
}

func (l *List) NumInstances() int {
	return len(l.items)
}

// InstanceRenderer handles rendering of session.Instance objects
type InstanceRenderer struct {
	spinner *spinner.Model
	width   int
}

func (r *InstanceRenderer) setWidth(width int) {
	r.width = AdjustPreviewWidth(width)
}

// ɹ and ɻ are other options.
const branchIcon = "Ꮧ"

func (r *InstanceRenderer) Render(i *session.Instance, idx int, selected bool, hasMultipleRepos bool) string {
	prefix := fmt.Sprintf(" %d. ", idx)
	if idx >= 10 {
		prefix = prefix[:len(prefix)-1]
	}
	titleS := selectedTitleStyle
	descS := selectedDescStyle
	if !selected {
		titleS = titleStyle
		descS = listDescStyle
	}

	// add spinner next to title if it's running
	var join string
	switch i.Status {
	case session.Running:
		join = fmt.Sprintf("%s ", r.spinner.View())
	case session.Ready:
		join = readyStyle.Render(readyIcon)
	case session.Paused:
		join = pausedStyle.Render(pausedIcon)
	case session.Loading:
		join = fmt.Sprintf("%s ", r.spinner.View())
	default:
	}

	// Cut the title if it's too long
	titleText := i.Title
	widthAvail := r.width - 3 - runewidth.StringWidth(prefix) - 1
	if widthAvail > 0 && runewidth.StringWidth(titleText) > widthAvail {
		titleText = runewidth.Truncate(titleText, widthAvail-3, "...")
	}
	title := titleS.Render(lipgloss.JoinHorizontal(
		lipgloss.Left,
		lipgloss.Place(r.width-3, 1, lipgloss.Left, lipgloss.Center, fmt.Sprintf("%s %s", prefix, titleText)),
		" ",
		join,
	))

	stat := i.GetDiffStats()

	var diff string
	var addedDiff, removedDiff string
	if stat == nil || stat.Error != nil || stat.IsEmpty() {
		// Don't show diff stats if there's an error or if they don't exist
		addedDiff = ""
		removedDiff = ""
		diff = ""
	} else {
		addedDiff = fmt.Sprintf("+%d", stat.Added)
		removedDiff = fmt.Sprintf("-%d ", stat.Removed)
		diff = lipgloss.JoinHorizontal(
			lipgloss.Center,
			addedLinesStyle.Background(descS.GetBackground()).Render(addedDiff),
			lipgloss.Style{}.Background(descS.GetBackground()).Foreground(descS.GetForeground()).Render(","),
			removedLinesStyle.Background(descS.GetBackground()).Render(removedDiff),
		)
	}

	remainingWidth := r.width
	remainingWidth -= runewidth.StringWidth(prefix)
	remainingWidth -= runewidth.StringWidth(branchIcon)

	diffWidth := runewidth.StringWidth(addedDiff) + runewidth.StringWidth(removedDiff)
	if diffWidth > 0 {
		diffWidth += 1
	}

	// Use fixed width for diff stats to avoid layout issues
	remainingWidth -= diffWidth

	branch := i.Branch
	if i.Started() && hasMultipleRepos {
		repoName, err := i.RepoName()
		if err != nil {
			log.ErrorLog.Printf("could not get repo name in instance renderer: %v", err)
		} else {
			branch += fmt.Sprintf(" (%s)", repoName)
		}
	}
	// Don't show branch if there's no space for it. Or show ellipsis if it's too long.
	branchWidth := runewidth.StringWidth(branch)
	if remainingWidth < 0 {
		branch = ""
	} else if remainingWidth < branchWidth {
		if remainingWidth < 3 {
			branch = ""
		} else {
			// We know the remainingWidth is at least 4 and branch is longer than that, so this is safe.
			branch = runewidth.Truncate(branch, remainingWidth-3, "...")
		}
	}
	remainingWidth -= runewidth.StringWidth(branch)

	// Add spaces to fill the remaining width.
	spaces := ""
	if remainingWidth > 0 {
		spaces = strings.Repeat(" ", remainingWidth)
	}

	branchLine := fmt.Sprintf("%s %s-%s%s%s", strings.Repeat(" ", len(prefix)), branchIcon, branch, spaces, diff)

	// join title and subtitle
	text := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		descS.Render(branchLine),
	)

	return text
}

func (l *List) String() string {
	const titleText = " Instances "
	const autoYesText = " auto-yes "

	// Write the title.
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("\n")

	// Write title line
	// add padding of 2 because the border on list items adds some extra characters
	titleWidth := AdjustPreviewWidth(l.width) + 2
	if !l.autoyes {
		b.WriteString(lipgloss.Place(
			titleWidth, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleText)))
	} else {
		title := lipgloss.Place(
			titleWidth/2, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleText))
		autoYes := lipgloss.Place(
			titleWidth-(titleWidth/2), 1, lipgloss.Right, lipgloss.Bottom, autoYesStyle.Render(autoYesText))
		b.WriteString(lipgloss.JoinHorizontal(
			lipgloss.Top, title, autoYes))
	}

	b.WriteString("\n")
	b.WriteString("\n")

	// Build grouped view and render
	groups := l.getGroupedInstances()
	renderedIdx := 0

	for groupIdx, group := range groups {
		// Render group header
		b.WriteString(l.renderGroupHeader(group.repoName))
		b.WriteString("\n")

		// Render instances in this group
		for instIdx, inst := range group.instances {
			actualIdx := l.findInstanceIndex(inst)
			isSelected := actualIdx == l.selectedIdx

			b.WriteString(l.renderer.Render(inst, renderedIdx+1, isSelected, len(l.repos) > 1))
			renderedIdx++

			// Add spacing between instances within a group
			if instIdx < len(group.instances)-1 {
				b.WriteString("\n\n")
			}
		}

		// Add extra spacing between groups
		if groupIdx < len(groups)-1 {
			b.WriteString("\n\n")
		}
	}

	return lipgloss.Place(l.width, l.height, lipgloss.Left, lipgloss.Top, b.String())
}

// Down selects the next item in the list based on display order.
func (l *List) Down() {
	if len(l.items) == 0 {
		return
	}
	displayOrder := l.getDisplayOrder()
	currentInst := l.items[l.selectedIdx]

	// Find current position in display order
	for i, inst := range displayOrder {
		if inst == currentInst {
			if i < len(displayOrder)-1 {
				// Move to next in display order, update selectedIdx to point to it in l.items
				l.selectedIdx = l.findInstanceIndex(displayOrder[i+1])
			}
			return
		}
	}
}

// Kill removes the currently selected instance from the list.
func (l *List) Kill() {
	if len(l.items) == 0 {
		return
	}
	targetInstance := l.items[l.selectedIdx]

	// Kill the tmux session
	if err := targetInstance.Kill(); err != nil {
		log.ErrorLog.Printf("could not kill instance: %v", err)
	}

	// Unregister the reponame.
	repoName, err := targetInstance.RepoName()
	if err != nil {
		log.ErrorLog.Printf("could not get repo name: %v", err)
	} else {
		l.rmRepo(repoName)
	}

	// Remove the item from the list.
	l.items = append(l.items[:l.selectedIdx], l.items[l.selectedIdx+1:]...)

	// Adjust selectedIdx if we deleted the last item.
	if l.selectedIdx >= len(l.items) && l.selectedIdx > 0 {
		l.selectedIdx = len(l.items) - 1
	}

	l.invalidateCache()
}

func (l *List) Attach() (chan struct{}, error) {
	targetInstance := l.items[l.selectedIdx]
	return targetInstance.Attach()
}

// Up selects the prev item in the list based on display order.
func (l *List) Up() {
	if len(l.items) == 0 {
		return
	}
	displayOrder := l.getDisplayOrder()
	currentInst := l.items[l.selectedIdx]

	// Find current position in display order
	for i, inst := range displayOrder {
		if inst == currentInst {
			if i > 0 {
				// Move to prev in display order, update selectedIdx to point to it in l.items
				l.selectedIdx = l.findInstanceIndex(displayOrder[i-1])
			}
			return
		}
	}
}

// JumpToDisplayIndex jumps to the item at the given 1-indexed display position.
// Returns true if the jump was successful, false if the index is out of bounds.
func (l *List) JumpToDisplayIndex(idx int) bool {
	displayOrder := l.getDisplayOrder()
	if idx < 1 || idx > len(displayOrder) {
		return false
	}
	l.selectedIdx = l.findInstanceIndex(displayOrder[idx-1])
	return true
}

func (l *List) addRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		l.repos[repo] = 0
	}
	l.repos[repo]++
}

func (l *List) rmRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		log.ErrorLog.Printf("repo %s not found", repo)
		return
	}
	l.repos[repo]--
	if l.repos[repo] == 0 {
		delete(l.repos, repo)
	}
}

// AddInstance adds a new instance to the list. It returns a finalizer function that should be called when the instance
// is started. If the instance was restored from storage or is paused, you can call the finalizer immediately.
// When creating a new one and entering the name, you want to call the finalizer once the name is done.
func (l *List) AddInstance(instance *session.Instance) (finalize func()) {
	l.items = append(l.items, instance)
	l.invalidateCache()
	// The finalizer registers the repo name once the instance is started.
	return func() {
		repoName, err := instance.RepoName()
		if err != nil {
			log.ErrorLog.Printf("could not get repo name: %v", err)
			return
		}

		l.addRepo(repoName)
		l.invalidateCache() // Repo name now known, rebuild groups
	}
}

// GetSelectedInstance returns the currently selected instance
func (l *List) GetSelectedInstance() *session.Instance {
	if len(l.items) == 0 {
		return nil
	}
	return l.items[l.selectedIdx]
}

// SetSelectedInstance sets the selected index. Noop if the index is out of bounds.
func (l *List) SetSelectedInstance(idx int) {
	if idx >= len(l.items) {
		return
	}
	l.selectedIdx = idx
}

// GetInstances returns all instances in the list
func (l *List) GetInstances() []*session.Instance {
	return l.items
}

// getGroupedInstances returns cached instances grouped by repo.
// Use invalidateCache() when the list changes.
func (l *List) getGroupedInstances() []repoGroup {
	l.ensureCacheValid()
	return l.cachedGroups
}

// buildGroupedInstances builds the grouped instances (internal, no caching).
func (l *List) buildGroupedInstances() []repoGroup {
	// Build map of repo name -> instances
	repoMap := make(map[string][]*session.Instance)
	for _, inst := range l.items {
		var repoName string
		if inst.Started() {
			name, err := inst.RepoName()
			if err == nil {
				repoName = name
			}
		}
		if repoName == "" {
			repoName = "(unknown)"
		}
		repoMap[repoName] = append(repoMap[repoName], inst)
	}

	// Get sorted repo names
	repoNames := make([]string, 0, len(repoMap))
	for name := range repoMap {
		repoNames = append(repoNames, name)
	}
	sort.Strings(repoNames)

	// Build groups with sorted instances
	groups := make([]repoGroup, 0, len(repoNames))
	for _, name := range repoNames {
		instances := repoMap[name]
		// Sort by CreatedAt ascending (oldest first)
		sort.Slice(instances, func(i, j int) bool {
			return instances[i].CreatedAt.Before(instances[j].CreatedAt)
		})
		groups = append(groups, repoGroup{
			repoName:  name,
			instances: instances,
		})
	}

	return groups
}

// getDisplayOrder returns cached instances in display order.
func (l *List) getDisplayOrder() []*session.Instance {
	l.ensureCacheValid()
	return l.cachedOrder
}

// buildDisplayOrder builds the display order from groups (internal, no caching).
func (l *List) buildDisplayOrder(groups []repoGroup) []*session.Instance {
	result := make([]*session.Instance, 0, len(l.items))
	for _, group := range groups {
		result = append(result, group.instances...)
	}
	return result
}

// renderGroupHeader renders a visual separator header for a repository group
func (l *List) renderGroupHeader(repoName string) string {
	// Calculate available width for the header line
	width := l.renderer.width
	if width <= 0 {
		width = 40 // default fallback
	}

	// Build header: "───── repo-name ─────"
	nameLen := runewidth.StringWidth(repoName)
	totalDashes := width - nameLen - 2 // -2 for spaces around name
	leftDashes := totalDashes / 2
	rightDashes := totalDashes - leftDashes

	if leftDashes < 2 {
		leftDashes = 2
		rightDashes = 2
	}

	header := fmt.Sprintf("%s %s %s",
		strings.Repeat("─", leftDashes),
		repoName,
		strings.Repeat("─", rightDashes))

	return groupHeaderStyle.Render(header)
}

// findInstanceIndex returns the index of an instance in l.items, or -1 if not found.
// Uses O(1) map lookup from cache.
func (l *List) findInstanceIndex(target *session.Instance) int {
	l.ensureCacheValid()
	if idx, ok := l.cachedIndexMap[target]; ok {
		return idx
	}
	return -1
}

