package diffviewer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/git"
	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/mode/shared"
	"github.com/zjrosen/perles/internal/ui/shared/overlay"
	"github.com/zjrosen/perles/internal/ui/shared/panes"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// Layout constants
const (
	fileListMinWidth   = 30   // Minimum width for file list panel
	fileListMaxWidth   = 60   // Maximum width for file list panel
	fileListRatio      = 0.30 // File list takes 30% of width
	boxMaxWidth        = 240  // Maximum box width
	boxMinWidth        = 40   // Minimum box width
	boxPaddingX        = 2    // Horizontal padding for box
	boxPaddingY        = 2    // Vertical padding for box
	footerPaneHeight   = 3    // Footer pane: 1 line content + 2 border lines
	scrollLines        = 3    // Number of lines to scroll with page up/down
	commitHistoryLimit = 50   // Maximum commits to load in history

	// Virtual scrolling threshold - use virtual scrolling when total lines exceed this
	virtualScrollingThreshold = 500

	// minSideBySideWidth is the minimum terminal width for side-by-side view.
	// Below this width, only unified view is available.
	minSideBySideWidth = 100
)

// focusPane indicates which pane has keyboard focus.
type focusPane int

const (
	// focusFileList indicates the file list panel has focus.
	focusFileList focusPane = iota
	// focusCommitPicker indicates the commit picker panel has focus.
	focusCommitPicker
	// focusDiffPane indicates the diff content panel has focus.
	focusDiffPane
)

// commitPaneMode indicates the current mode of the commits pane.
type commitPaneMode int

const (
	// commitPaneModeList shows the list of commits.
	commitPaneModeList commitPaneMode = iota
	// commitPaneModeFiles shows files changed in the selected commit.
	commitPaneModeFiles
)

// commitsTabIndex indicates which tab is active in the commits pane.
type commitsTabIndex int

const (
	// commitsTabCommits shows the commit history list (default).
	commitsTabCommits commitsTabIndex = iota
	// commitsTabBranches shows available branches for selection.
	commitsTabBranches
	// commitsTabWorktrees shows available worktrees for selection.
	commitsTabWorktrees
)

// ShowDiffViewerMsg requests showing the diff viewer.
type ShowDiffViewerMsg struct{}

// HideDiffViewerMsg requests hiding the diff viewer.
type HideDiffViewerMsg struct{}

// CommitsLoadedMsg carries parsed commit history back to the model.
type CommitsLoadedMsg struct {
	Commits []git.CommitInfo
	Branch  string
	Err     error
}

// CommitFilesLoadedMsg carries parsed files for a specific commit.
type CommitFilesLoadedMsg struct {
	Hash  string // Commit hash this load was for
	Files []DiffFile
	Err   error
}

// WorkingDirDiffLoadedMsg carries parsed working directory diff.
type WorkingDirDiffLoadedMsg struct {
	Files []DiffFile
	Err   error
}

// CommitPreviewLoadedMsg carries parsed diff for commit preview in ListMode.
type CommitPreviewLoadedMsg struct {
	Hash  string     // Commit hash this preview is for
	Files []DiffFile // All files changed in the commit
	Err   error
}

// WorktreesLoadedMsg carries the list of available worktrees.
type WorktreesLoadedMsg struct {
	Worktrees []git.WorktreeInfo
	Err       error
}

// BranchesLoadedMsg carries the list of available branches.
type BranchesLoadedMsg struct {
	Branches []git.BranchInfo
	Err      error
}

// CommitsForBranchLoadedMsg carries commits loaded for a specific branch.
type CommitsForBranchLoadedMsg struct {
	Commits []git.CommitInfo
	Branch  string // Branch name the commits are from
	Err     error
}

// HunkCopiedMsg is returned when a hunk has been copied to the clipboard.
type HunkCopiedMsg struct {
	LineCount int   // Number of lines copied
	Err       error // Error if copy failed
}

// ViewModeConstrainedMsg is returned when user tries to switch to side-by-side
// view but the terminal is too narrow. The app should show a toast message.
type ViewModeConstrainedMsg struct {
	RequestedMode ViewMode // The mode the user tried to switch to
	MinWidth      int      // Minimum width required
	CurrentWidth  int      // Current terminal width
}

// Model is the diff viewer component state.
type Model struct {
	visible       bool
	width, height int

	// View mode (unified vs side-by-side)
	viewMode          ViewMode // Current effective view mode (may be constrained by width)
	preferredViewMode ViewMode // User's preferred mode (stored even when width constrains it)

	// Working directory files (always shows uncommitted changes)
	workingDirFiles         []DiffFile // Parsed diff files for working directory
	workingDirTree          *FileTree  // Tree structure for working directory files
	selectedWorkingDirNode  int        // Currently selected node index in visible tree
	workingDirTreeScrollTop int        // First visible node for scrolling

	err error

	// Commit picker state
	commits                  []git.CommitInfo // Loaded commit history
	selectedCommit           int              // Index into commits slice
	commitScrollTop          int              // First visible commit for scrolling
	currentBranch            string           // Branch name for header display (e.g., "main")
	lastLeftFocus            focusPane        // Track last focused left pane for h key restoration
	commitPaneMode           commitPaneMode   // Current mode: list of commits or files in a commit
	commitFiles              []DiffFile       // Files changed in the selected commit (when in FilesMode)
	commitFilesTree          *FileTree        // Tree structure for commit files
	selectedCommitFileNode   int              // Currently selected node index in visible tree
	commitFilesTreeScrollTop int              // First visible node for scrolling in commit files tree
	inspectedCommit          *git.CommitInfo  // The commit being inspected (when in FilesMode)

	// Commit preview state (for showing full commit diff when commit is highlighted in ListMode)
	previewCommitFiles   []DiffFile // All files changed in the highlighted commit
	previewCommitHash    string     // Hash of the commit being previewed
	previewCommitLoading bool       // Loading state for preview diff

	// UI Components
	diffViewport viewport.Model

	// Virtual scrolling for large diffs
	virtualContent      *VirtualContent  // Legacy: used for line data
	virtualViewport     *VirtualViewport // New: efficient rendering without padding
	useVirtualScrolling bool             // Whether to use virtual scrolling (enabled for large diffs)
	lastViewportYOffset int              // Track scroll position for virtual content updates

	// Scroll position preservation per file
	scrollPositions map[string]int // Map of file path -> viewport Y offset

	// Focus management
	focus focusPane

	// Dependencies (passed in)
	gitExecutor git.GitExecutor
	clock       shared.Clock     // Clock for relative timestamp rendering
	clipboard   shared.Clipboard // Clipboard for copy operations

	// Git context switching state
	gitExecutorFactory    func(string) git.GitExecutor // Factory for creating executors
	originalWorkDir       string                       // Original working directory path
	currentWorktreePath   string                       // Path of the currently selected worktree (empty = original)
	currentWorktreeBranch string                       // Branch in the current worktree
	viewingBranch         string                       // Branch whose commits are displayed (empty = HEAD)

	// Modal visibility flags
	showHelpOverlay bool

	// Modal components
	helpOverlay helpModel

	// Tab navigation state for commits pane
	activeCommitTab commitsTabIndex // Currently active tab (0=Commits, 1=Branches, 2=Worktrees)

	// Branch list state (for Branches tab display)
	branchList       []git.BranchInfo // Loaded branches for tab display
	selectedBranch   int              // Index into branchList slice
	branchScrollTop  int              // First visible branch for scrolling
	branchListLoaded bool             // Whether branches have been loaded

	// Worktree list state (for Worktrees tab display)
	worktreeList       []git.WorktreeInfo // Loaded worktrees for tab display
	selectedWorktree   int                // Index into worktreeList slice
	worktreeScrollTop  int                // First visible worktree for scrolling
	worktreeListLoaded bool               // Whether worktrees have been loaded

	// Word-level diff cache (file path -> word diff results)
	wordDiffCache map[string]*fileWordDiff

	// Scrollbar state
	showScrollbar bool // true when totalLines > viewportHeight
}

// New creates a new diff viewer model.
func New() Model {
	return Model{
		visible:           false,
		focus:             focusFileList,
		clock:             shared.RealClock{},
		viewMode:          ViewModeUnified,
		preferredViewMode: ViewModeUnified,
		scrollPositions:   make(map[string]int),
		wordDiffCache:     make(map[string]*fileWordDiff),
		// Tab state initialization
		activeCommitTab:    commitsTabCommits,
		selectedBranch:     0,
		branchScrollTop:    0,
		branchListLoaded:   false,
		selectedWorktree:   0,
		worktreeScrollTop:  0,
		worktreeListLoaded: false,
	}
}

// NewWithGitExecutor creates a new diff viewer model with a git executor.
func NewWithGitExecutor(ge git.GitExecutor) Model {
	return Model{
		visible:           false,
		focus:             focusFileList,
		gitExecutor:       ge,
		clock:             shared.RealClock{},
		viewMode:          ViewModeUnified,
		preferredViewMode: ViewModeUnified,
		scrollPositions:   make(map[string]int),
		wordDiffCache:     make(map[string]*fileWordDiff),
		// Tab state initialization
		activeCommitTab:    commitsTabCommits,
		selectedBranch:     0,
		branchScrollTop:    0,
		branchListLoaded:   false,
		selectedWorktree:   0,
		worktreeScrollTop:  0,
		worktreeListLoaded: false,
	}
}

// NewWithGitExecutorFactory creates a diff viewer with a factory for git executors.
// This enables worktree switching by creating new executors for different paths.
// The factory function is called to create new git executors when switching worktrees.
// If factory is non-nil and initialPath is non-empty, creates an initial executor.
func NewWithGitExecutorFactory(factory func(string) git.GitExecutor, initialPath string) Model {
	m := New()
	m.gitExecutorFactory = factory
	m.originalWorkDir = initialPath
	if factory != nil && initialPath != "" {
		m.gitExecutor = factory(initialPath)
		m.currentWorktreePath = initialPath
	}
	return m
}

// SetClock sets the clock for timestamp rendering (useful for testing).
func (m Model) SetClock(clock shared.Clock) Model {
	m.clock = clock
	return m
}

// SetClipboard sets the clipboard for copy operations.
func (m Model) SetClipboard(clipboard shared.Clipboard) Model {
	m.clipboard = clipboard
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages for the diff viewer.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.MouseMsg:
		return (&m).handleMouseMsg(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.refreshViewport()
		return m, nil

	case WorkingDirDiffLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
		} else {
			m.workingDirFiles = msg.Files
			m.workingDirTree = NewFileTree(msg.Files)
			m.selectedWorkingDirNode = 0
			m.workingDirTreeScrollTop = 0
		}
		m.refreshViewport()
		return m, nil

	case CommitFilesLoadedMsg:
		// Only update if this is still the commit we're inspecting
		if m.inspectedCommit != nil && msg.Hash == m.inspectedCommit.Hash {
			if msg.Err != nil {
				m.err = msg.Err
				return m, nil
			}
			m.commitFiles = msg.Files
			m.commitFilesTree = NewFileTree(msg.Files)
			m.selectedCommitFileNode = 0
			m.commitFilesTreeScrollTop = 0
			// Now switch to files mode - data is ready, no flash
			m.commitPaneMode = commitPaneModeFiles
			m.refreshViewport()
		}
		return m, nil

	case CommitsLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.commits = msg.Commits
		m.currentBranch = msg.Branch
		m.selectedCommit = 0
		m.commitScrollTop = 0
		// Load preview for first commit if available
		if len(m.commits) > 0 {
			m.previewCommitHash = m.commits[0].Hash
			m.previewCommitLoading = true
			return m, m.LoadCommitPreview(m.commits[0].Hash)
		}
		return m, nil

	case CommitPreviewLoadedMsg:
		// Only update if this is still the commit we're previewing
		if msg.Hash == m.previewCommitHash {
			m.previewCommitLoading = false
			if msg.Err == nil {
				m.previewCommitFiles = msg.Files
			}
			m.refreshViewport()
		}
		return m, nil

	// Worktree selector messages
	case WorktreesLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.worktreeList = msg.Worktrees
		m.worktreeListLoaded = true
		return m, nil

	// Branch selector messages
	case BranchesLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.branchList = msg.Branches
		m.branchListLoaded = true
		return m, nil

	case CommitsForBranchLoadedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.commits = msg.Commits
		m.viewingBranch = msg.Branch
		m.selectedCommit = 0
		m.commitScrollTop = 0
		// Load preview for first commit if available
		if len(m.commits) > 0 {
			m.previewCommitHash = m.commits[0].Hash
			m.previewCommitLoading = true
			return m, m.LoadCommitPreview(m.commits[0].Hash)
		}
		m.refreshViewport()
		return m, nil
	}

	return m, nil
}

// handleKeyMsg processes keyboard input.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Handle help overlay keys when showing
	if m.showHelpOverlay {
		// Any key closes help (? or Esc typical, but any key works for discoverability)
		if key.Matches(msg, keys.DiffViewer.Help) || key.Matches(msg, keys.DiffViewer.Close) {
			m.showHelpOverlay = false
			return m, nil
		}
		// Ignore other keys while help is shown
		return m, nil
	}

	// Handle ] and [ keys - behavior depends on focus and mode
	keyStr := msg.String()

	// Tab navigation only when commits pane is focused AND in list mode (not viewing a specific commit's files)
	if m.focus == focusCommitPicker && m.commitPaneMode == commitPaneModeList {
		if keyStr == "]" {
			return m.nextCommitTab()
		}
		if keyStr == "[" {
			return m.prevCommitTab()
		}
	}

	// Hunk navigation (only reached when NOT on commits pane)
	if keyStr == "]" {
		return (&m).executeCommand(cmdNextHunk)
	}
	if keyStr == "[" {
		return (&m).executeCommand(cmdPrevHunk)
	}

	// Route all standard key bindings through the command dispatch system.
	// This eliminates duplicate logic between handleKeyMsg and executeCommand.
	if cmdID := keyToCommand(msg); cmdID != "" {
		return (&m).executeCommand(cmdID)
	}

	return m, nil
}

// handleMouseMsg processes mouse input for scrolling.
func (m *Model) handleMouseMsg(msg tea.MouseMsg) (Model, tea.Cmd) {
	// Only handle scroll wheel events
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return *m, nil
	}

	// Calculate overlay position (centered on screen)
	boxWidth := m.boxWidth()
	boxHeight := m.boxHeight()
	overlayX := (m.width - boxWidth) / 2
	overlayY := (m.height - boxHeight) / 2

	// Convert screen coordinates to overlay-relative coordinates
	relX := msg.X - overlayX
	relY := msg.Y - overlayY

	// Check if mouse is within the overlay bounds
	if relX < 0 || relX >= boxWidth || relY < 0 || relY >= boxHeight {
		return *m, nil
	}

	// Calculate pane boundaries
	fileListWidth := m.fileListWidth(boxWidth)

	// Mouse position relative to panes
	innerX := relX

	scrollUp := msg.Button == tea.MouseButtonWheelUp

	if innerX > fileListWidth {
		// Mouse is over diff pane - scroll the diff viewport
		if scrollUp {
			m.scrollDiffUp(scrollLines)
		} else {
			m.scrollDiffDown(scrollLines)
		}
	}
	// Note: Could add scrolling for file list and commit picker panes here if needed

	return *m, nil
}

// handleFileListDown handles j/k down navigation in the file list pane.
func (m Model) handleFileListDown() (Model, tea.Cmd) {
	if m.workingDirTree == nil {
		return m, nil
	}
	nodes := m.workingDirTree.VisibleNodes()
	if len(nodes) == 0 {
		return m, nil
	}
	newIdx := min(m.selectedWorkingDirNode+1, len(nodes)-1)
	if newIdx != m.selectedWorkingDirNode {
		// Save scroll position before switching files
		m.saveScrollPosition()
		m.selectedWorkingDirNode = newIdx
	}
	m.ensureWorkingDirNodeVisible()
	m.refreshViewport()
	return m, nil
}

// handleFileListUp handles j/k up navigation in the file list pane.
func (m Model) handleFileListUp() (Model, tea.Cmd) {
	if m.workingDirTree == nil {
		return m, nil
	}
	nodes := m.workingDirTree.VisibleNodes()
	if len(nodes) == 0 {
		return m, nil
	}
	newIdx := max(m.selectedWorkingDirNode-1, 0)
	if newIdx != m.selectedWorkingDirNode {
		// Save scroll position before switching files
		m.saveScrollPosition()
		m.selectedWorkingDirNode = newIdx
	}
	m.ensureWorkingDirNodeVisible()
	m.refreshViewport()
	return m, nil
}

// handleFileListToggle handles Enter/Space to toggle directory expand/collapse.
func (m Model) handleFileListToggle() (Model, tea.Cmd) {
	if m.workingDirTree == nil {
		return m, nil
	}
	nodes := m.workingDirTree.VisibleNodes()
	if m.selectedWorkingDirNode >= len(nodes) {
		return m, nil
	}
	node := nodes[m.selectedWorkingDirNode]
	if node.IsDir {
		m.workingDirTree.Toggle(node)
		// Clamp selection to new visible range
		newNodes := m.workingDirTree.VisibleNodes()
		if m.selectedWorkingDirNode >= len(newNodes) {
			m.selectedWorkingDirNode = max(len(newNodes)-1, 0)
		}
	}
	m.refreshViewport()
	return m, nil
}

// handleCommitFilesToggle handles Enter key on a directory in commit files tree.
func (m Model) handleCommitFilesToggle() (Model, tea.Cmd) {
	if m.commitFilesTree == nil {
		return m, nil
	}
	nodes := m.commitFilesTree.VisibleNodes()
	if m.selectedCommitFileNode >= len(nodes) {
		return m, nil
	}
	node := nodes[m.selectedCommitFileNode]
	if node.IsDir {
		m.commitFilesTree.Toggle(node)
		// Clamp selection to new visible range
		newNodes := m.commitFilesTree.VisibleNodes()
		if m.selectedCommitFileNode >= len(newNodes) {
			m.selectedCommitFileNode = max(len(newNodes)-1, 0)
		}
	}
	m.refreshViewport()
	return m, nil
}

// ensureItemVisible adjusts scroll position to keep the selected item visible.
// It modifies scrollTop in-place to ensure selected is within the visible window.
func ensureItemVisible(selected int, scrollTop *int, visibleHeight int) {
	if visibleHeight < 1 {
		return
	}
	// Scroll down if selection is below visible area
	if selected >= *scrollTop+visibleHeight {
		*scrollTop = selected - visibleHeight + 1
	}
	// Scroll up if selection is above visible area
	if selected < *scrollTop {
		*scrollTop = selected
	}
}

// ensureWorkingDirNodeVisible adjusts scroll position to keep selected node visible.
func (m *Model) ensureWorkingDirNodeVisible() {
	ensureItemVisible(m.selectedWorkingDirNode, &m.workingDirTreeScrollTop, m.fileListVisibleHeight())
}

// getSelectedNodeFromTree returns the selected node from a tree at the given index.
// Returns nil if tree is nil or index is out of bounds.
func getSelectedNodeFromTree(tree *FileTree, index int) *FileTreeNode {
	if tree == nil {
		return nil
	}
	nodes := tree.VisibleNodes()
	if index >= len(nodes) {
		return nil
	}
	return nodes[index]
}

// getSelectedWorkingDirFile returns the currently selected file from the tree.
// Returns nil if a directory is selected or no selection.
func (m Model) getSelectedWorkingDirFile() *DiffFile {
	node := m.getSelectedWorkingDirNode()
	if node == nil || node.IsDir {
		return nil
	}
	return node.File
}

// getSelectedWorkingDirNode returns the currently selected node from the tree.
// Returns nil if no selection.
func (m Model) getSelectedWorkingDirNode() *FileTreeNode {
	return getSelectedNodeFromTree(m.workingDirTree, m.selectedWorkingDirNode)
}

// getSelectedCommitFile returns the currently selected file from the commit files tree.
// Returns nil if a directory is selected or no selection.
func (m Model) getSelectedCommitFile() *DiffFile {
	node := m.getSelectedCommitFileNode()
	if node == nil || node.IsDir {
		return nil
	}
	return node.File
}

// getSelectedCommitFileNode returns the currently selected node from the commit files tree.
// Returns nil if no selection.
func (m Model) getSelectedCommitFileNode() *FileTreeNode {
	return getSelectedNodeFromTree(m.commitFilesTree, m.selectedCommitFileNode)
}

// fileListVisibleHeight returns the number of visible rows in the file list pane.
func (m Model) fileListVisibleHeight() int {
	contentHeight := m.contentHeight()
	fileListHeight := contentHeight / 2
	innerHeight := fileListHeight - 2 // Account for border
	return max(innerHeight, 1)
}

// handleCommitPaneDown handles j/k down navigation in the commit pane.
// Behavior depends on active tab and mode: ListMode navigates commits/branches/worktrees, FilesMode navigates files.
func (m Model) handleCommitPaneDown() (Model, tea.Cmd) {
	// Tab-aware navigation
	switch m.activeCommitTab {
	case commitsTabCommits:
		switch m.commitPaneMode {
		case commitPaneModeList:
			if len(m.commits) > 0 {
				newIdx := min(m.selectedCommit+1, len(m.commits)-1)
				if newIdx != m.selectedCommit {
					m.selectedCommit = newIdx
					m.ensureCommitVisible()
					m.previewCommitHash = m.commits[newIdx].Hash
					m.previewCommitLoading = true
					return m, m.LoadCommitPreview(m.commits[newIdx].Hash)
				}
			}
		case commitPaneModeFiles:
			if m.commitFilesTree != nil {
				nodes := m.commitFilesTree.VisibleNodes()
				if len(nodes) > 0 {
					newIdx := min(m.selectedCommitFileNode+1, len(nodes)-1)
					if newIdx != m.selectedCommitFileNode {
						// Save scroll position before switching files
						m.saveScrollPosition()
						m.selectedCommitFileNode = newIdx
					}
					m.ensureCommitFileNodeVisible()
					m.refreshViewport()
				}
			}
		}
	case commitsTabBranches:
		if len(m.branchList) > 0 {
			newIdx := min(m.selectedBranch+1, len(m.branchList)-1)
			if newIdx != m.selectedBranch {
				m.selectedBranch = newIdx
				m.ensureBranchVisible()
			}
		}
	case commitsTabWorktrees:
		if len(m.worktreeList) > 0 {
			newIdx := min(m.selectedWorktree+1, len(m.worktreeList)-1)
			if newIdx != m.selectedWorktree {
				m.selectedWorktree = newIdx
				m.ensureWorktreeVisible()
			}
		}
	}
	return m, nil
}

// handleCommitPaneUp handles j/k up navigation in the commit pane.
// Behavior depends on active tab and mode: ListMode navigates commits/branches/worktrees, FilesMode navigates files.
func (m Model) handleCommitPaneUp() (Model, tea.Cmd) {
	// Tab-aware navigation
	switch m.activeCommitTab {
	case commitsTabCommits:
		switch m.commitPaneMode {
		case commitPaneModeList:
			if len(m.commits) > 0 {
				newIdx := max(m.selectedCommit-1, 0)
				if newIdx != m.selectedCommit {
					m.selectedCommit = newIdx
					m.ensureCommitVisible()
					m.previewCommitHash = m.commits[newIdx].Hash
					m.previewCommitLoading = true
					return m, m.LoadCommitPreview(m.commits[newIdx].Hash)
				}
			}
		case commitPaneModeFiles:
			if m.commitFilesTree != nil {
				nodes := m.commitFilesTree.VisibleNodes()
				if len(nodes) > 0 {
					newIdx := max(m.selectedCommitFileNode-1, 0)
					if newIdx != m.selectedCommitFileNode {
						// Save scroll position before switching files
						m.saveScrollPosition()
						m.selectedCommitFileNode = newIdx
					}
					m.ensureCommitFileNodeVisible()
					m.refreshViewport()
				}
			}
		}
	case commitsTabBranches:
		if len(m.branchList) > 0 {
			newIdx := max(m.selectedBranch-1, 0)
			if newIdx != m.selectedBranch {
				m.selectedBranch = newIdx
				m.ensureBranchVisible()
			}
		}
	case commitsTabWorktrees:
		if len(m.worktreeList) > 0 {
			newIdx := max(m.selectedWorktree-1, 0)
			if newIdx != m.selectedWorktree {
				m.selectedWorktree = newIdx
				m.ensureWorktreeVisible()
			}
		}
	}
	return m, nil
}

// ensureCommitVisible adjusts scroll position to keep selected commit visible.
func (m *Model) ensureCommitVisible() {
	ensureItemVisible(m.selectedCommit, &m.commitScrollTop, m.commitPickerVisibleHeight())
}

// ensureCommitFileNodeVisible adjusts scroll position to keep selected commit file node visible.
func (m *Model) ensureCommitFileNodeVisible() {
	ensureItemVisible(m.selectedCommitFileNode, &m.commitFilesTreeScrollTop, m.commitPickerVisibleHeight())
}

// ensureBranchVisible adjusts scroll position to keep selected branch visible.
func (m *Model) ensureBranchVisible() {
	ensureItemVisible(m.selectedBranch, &m.branchScrollTop, m.commitPickerVisibleHeight())
}

// ensureWorktreeVisible adjusts scroll position to keep selected worktree visible.
func (m *Model) ensureWorktreeVisible() {
	ensureItemVisible(m.selectedWorktree, &m.worktreeScrollTop, m.commitPickerVisibleHeight())
}

// commitPickerVisibleHeight returns the number of visible rows in the commit picker pane.
func (m Model) commitPickerVisibleHeight() int {
	contentHeight := m.contentHeight()
	fileListHeight := contentHeight / 2
	commitPickerHeight := contentHeight - fileListHeight
	innerHeight := commitPickerHeight - 2 // Account for border
	return max(innerHeight, 1)
}

// drillIntoCommit loads files for the selected commit.
// The mode switch to commitPaneModeFiles happens when files are loaded (in CommitFilesLoadedMsg handler).
func (m Model) drillIntoCommit() (Model, tea.Cmd) {
	if len(m.commits) == 0 || m.selectedCommit >= len(m.commits) {
		return m, nil
	}
	commit := m.commits[m.selectedCommit]
	// Store which commit we're loading, but don't switch mode yet
	m.inspectedCommit = &commit
	m.commitFiles = nil
	m.commitFilesTree = nil
	m.selectedCommitFileNode = 0
	m.commitFilesTreeScrollTop = 0
	return m, m.LoadCommitFiles(commit.Hash)
}

// View renders the diff viewer content.
func (m Model) View() string {
	if !m.visible {
		return ""
	}

	boxWidth := m.boxWidth()
	contentHeight := m.contentHeight()

	// Build main content (file list + diff pane)
	var content string
	if m.err != nil {
		content = m.renderError(boxWidth, contentHeight)
	} else {
		// Always render split view - individual panes handle their own empty/loading states
		content = m.renderSplitView(boxWidth, contentHeight)
	}

	// Build footer pane (full width)
	footer := m.renderFooterPane(boxWidth)

	// Stack content and footer vertically
	baseView := lipgloss.JoinVertical(lipgloss.Left, content, footer)

	// Handle modal overlays (order matters - most recent on top)
	if m.showHelpOverlay {
		return m.helpOverlay.Overlay(baseView)
	}

	return baseView
}

// renderFooterPane builds the footer as a bordered pane with keyboard hints.
func (m Model) renderFooterPane(width int) string {
	hintStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	// Build hints based on focus and mode
	var hints []string
	hints = append(hints, hintStyle.Render("[j/k] Navigate"))
	hints = append(hints, hintStyle.Render("[h/l] Switch pane"))

	// Context-dependent hints for commit pane
	if m.focus == focusCommitPicker {
		if m.commitPaneMode == commitPaneModeList {
			hints = append(hints, hintStyle.Render("[Enter] View files"))
		} else {
			hints = append(hints, hintStyle.Render("[Esc] Back to commits"))
		}
	} else {
		hints = append(hints, hintStyle.Render("[^d/^u] Scroll"))
	}

	// Add help hint
	hints = append(hints, hintStyle.Render("[?] Help"))

	footerContent := " " + strings.Join(hints, "  ") // Add leading space for padding

	return panes.BorderedPane(panes.BorderConfig{
		Content:     footerContent,
		Width:       width,
		Height:      footerPaneHeight,
		BorderColor: styles.OverlayBorderColor,
	})
}

// Header pane height (1 line content + 2 border lines)
const headerPaneHeight = 3

// renderSplitView renders the multi-pane layout: left column (file list + commit picker) + right column (header + diff).
// The left column is vertically split 50/50 between file list and commit picker.
// The right column has a header pane showing breadcrumb/stats, then the diff pane below (connected, no gap).
func (m *Model) renderSplitView(width, height int) string {
	// Calculate panel widths
	fileListWidth := m.fileListWidth(width)
	diffWidth := width - fileListWidth

	// Calculate heights for left column panes (50/50 split)
	fileListHeight := height / 2
	commitPickerHeight := height - fileListHeight

	// Render file list pane with border
	fileListPane := m.renderFileListPane(fileListWidth, fileListHeight)

	// Render commit picker pane with border
	commitPickerPane := m.renderCommitPickerPane(fileListWidth, commitPickerHeight)

	// Stack left column vertically
	leftColumn := lipgloss.JoinVertical(lipgloss.Left, fileListPane, commitPickerPane)

	// Calculate right column heights
	diffHeight := height - headerPaneHeight

	// Render header pane (branch + commit + stats) - no bottom border
	headerPane := m.renderHeaderPane(diffWidth, headerPaneHeight)

	// Render diff pane - no top border (connects seamlessly with header)
	diffPane := m.renderDiffPane(diffWidth, diffHeight)

	// Stack right column vertically (header above diff, connected)
	rightColumn := lipgloss.JoinVertical(lipgloss.Left, headerPane, diffPane)

	// Join horizontally - left column has borders, right column has connected panes
	return lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, rightColumn)
}

// renderFileListPane renders the file list panel with a BorderedPane wrapper.
// The height parameter is the total height including the border.
func (m Model) renderFileListPane(width, height int) string {
	// Calculate inner dimensions (BorderedPane reserves 2 chars for borders on each axis)
	innerWidth := width - 2
	innerHeight := height - 2

	if innerWidth < 1 || innerHeight < 1 {
		return ""
	}

	// Render file list content
	content := m.renderFileListContent(innerWidth, innerHeight)

	// Build title with file count
	title := "Files Changed"
	var countStr string
	if len(m.workingDirFiles) == 0 {
		countStr = "No files"
	} else if len(m.workingDirFiles) == 1 {
		countStr = "1 file"
	} else {
		countStr = fmt.Sprintf("%d files", len(m.workingDirFiles))
	}

	return panes.BorderedPane(panes.BorderConfig{
		Content:            content,
		Width:              width,
		Height:             height,
		TopLeft:            title,
		TopRight:           countStr,
		Focused:            m.focus == focusFileList,
		TitleColor:         styles.OverlayTitleColor,
		BorderColor:        styles.OverlayBorderColor,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
}

// renderFileListContent renders the inner content of the file list (no borders).
// Always shows working directory (uncommitted) files as a tree with scrolling support.
func (m Model) renderFileListContent(width, height int) string {
	if m.workingDirTree == nil || len(m.workingDirFiles) == 0 {
		return renderNoFilesPlaceholder(width, height)
	}

	focused := m.focus == focusFileList
	return renderFileTree(m.workingDirTree, m.selectedWorkingDirNode, m.workingDirTreeScrollTop, width, height, focused)
}

// renderNoFilesPlaceholder renders a placeholder for zero files changed.
func renderNoFilesPlaceholder(width, height int) string {
	return renderCenteredPlaceholder(width, height, "No files changed", styles.TextMutedColor)
}

// renderCommitPickerPane renders the commit picker panel with a BorderedPane wrapper.
// In ListMode, shows tabs for Commits/Branches/Worktrees.
// In FilesMode, shows files changed in selected commit (no tabs).
// The height parameter is the total height including the border.
func (m Model) renderCommitPickerPane(width, height int) string {
	// Calculate inner dimensions (BorderedPane reserves 2 chars for borders on each axis)
	innerWidth := width - 2
	innerHeight := height - 2

	if innerWidth < 1 || innerHeight < 1 {
		return ""
	}

	switch m.commitPaneMode {
	case commitPaneModeList:
		// In list mode, use tabs for Commits/Branches/Worktrees
		focused := m.focus == focusCommitPicker

		// Build tab content for each tab
		commitsContent := renderCommitList(m.commits, m.selectedCommit, m.commitScrollTop, innerWidth, innerHeight, focused && m.activeCommitTab == commitsTabCommits)
		branchesContent := renderBranchList(m.branchList, m.selectedBranch, m.branchScrollTop, innerWidth, innerHeight, focused && m.activeCommitTab == commitsTabBranches)
		worktreesContent := renderWorktreeList(m.worktreeList, m.selectedWorktree, m.worktreeScrollTop, innerWidth, innerHeight, focused && m.activeCommitTab == commitsTabWorktrees)

		tabs := []panes.Tab{
			{Label: "Commits", Content: commitsContent},
			{Label: "Branches", Content: branchesContent},
			{Label: "Worktrees", Content: worktreesContent},
		}

		return panes.BorderedPane(panes.BorderConfig{
			Width:              width,
			Height:             height,
			Tabs:               tabs,
			ActiveTab:          int(m.activeCommitTab),
			Focused:            focused,
			BorderColor:        styles.OverlayBorderColor,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		})

	case commitPaneModeFiles:
		// In files mode, no tabs - show commit files without tabs
		var content string
		if m.commitFilesTree == nil || len(m.commitFiles) == 0 {
			content = renderNoFilesPlaceholder(innerWidth, innerHeight)
		} else {
			focused := m.focus == focusCommitPicker && m.commitPaneMode == commitPaneModeFiles
			content = renderFileTree(m.commitFilesTree, m.selectedCommitFileNode, m.commitFilesTreeScrollTop, innerWidth, innerHeight, focused)
		}

		// Build title showing which commit we're inspecting
		var title, countStr string
		if m.inspectedCommit != nil {
			title = m.inspectedCommit.ShortHash
		} else {
			title = "Commit Files"
		}
		if len(m.commitFiles) == 0 {
			countStr = "No files"
		} else if len(m.commitFiles) == 1 {
			countStr = "1 file"
		} else {
			countStr = fmt.Sprintf("%d files", len(m.commitFiles))
		}

		return panes.BorderedPane(panes.BorderConfig{
			Content:            content,
			Width:              width,
			Height:             height,
			TopLeft:            title,
			TopRight:           countStr,
			Focused:            m.focus == focusCommitPicker,
			TitleColor:         styles.OverlayTitleColor,
			BorderColor:        styles.OverlayBorderColor,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		})
	}

	return ""
}

// renderHeaderPane renders the header pane showing breadcrumb (file path), stats, and staging status.
// Format: src/auth.go  +45/-12  [STAGED: 2/3 hunks]
// Branch name is shown in the top-left title of the pane border.
func (m Model) renderHeaderPane(width, height int) string {
	innerWidth := width - 2 // borders
	innerWidth -= 2         // 1 space padding on each side

	// Build title with worktree/branch context
	title := m.buildHeaderTitle()

	// Style definitions
	pathStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)

	// Build breadcrumb (file path or context description)
	breadcrumb := m.buildBreadcrumb()

	// Build right side: stats
	// Stats are based on the selected file/directory, not totals
	var adds, dels int
	var isBinary bool

	// Get stats based on current selection
	if m.focus == focusCommitPicker && m.commitPaneMode == commitPaneModeList {
		// Commit preview: sum all files in the commit
		for _, f := range m.previewCommitFiles {
			adds += f.Additions
			dels += f.Deletions
		}
	} else {
		// File/directory selection: use the active node's stats
		if node := m.getActiveNode(); node != nil {
			if node.IsDir {
				adds, dels = node.TotalStats()
			} else if node.File != nil {
				adds = node.File.Additions
				dels = node.File.Deletions
				isBinary = node.File.IsBinary
			}
		}
	}

	statsStr := formatStats(adds, dels, isBinary)

	// Combine right side components
	var rightParts []string
	if statsStr != "" {
		rightParts = append(rightParts, statsStr)
	}
	rightContent := strings.Join(rightParts, "  ")
	rightWidth := lipgloss.Width(rightContent)

	// Calculate space for breadcrumb
	// Layout: " breadcrumb ... stats staging "
	paddingWidth := 2 // minimum padding between breadcrumb and right side
	breadcrumbMaxWidth := max(innerWidth-rightWidth-paddingWidth, 0)

	// Truncate breadcrumb if needed
	if lipgloss.Width(breadcrumb) > breadcrumbMaxWidth {
		breadcrumb = styles.TruncateString(breadcrumb, breadcrumbMaxWidth)
	}

	// Build the content line
	leftContent := pathStyle.Render(breadcrumb)
	leftWidth := lipgloss.Width(leftContent)

	// Pad between left and right
	padding := max(innerWidth-leftWidth-rightWidth, 1)

	// Add 1 space padding on left and right
	content := " " + leftContent + strings.Repeat(" ", padding) + rightContent + " "

	return panes.BorderedPane(panes.BorderConfig{
		Content:     content,
		Width:       width,
		Height:      height,
		TopLeft:     title,
		TitleColor:  styles.OverlayTitleColor,
		BorderColor: styles.OverlayBorderColor,
	})
}

// buildBreadcrumb constructs a breadcrumb string showing the current file path or context.
func (m Model) buildBreadcrumb() string {
	// For commit preview mode, show commit info
	if m.focus == focusCommitPicker && m.commitPaneMode == commitPaneModeList {
		if len(m.commits) > 0 && m.selectedCommit < len(m.commits) {
			commit := m.commits[m.selectedCommit]
			return commit.ShortHash + " " + commit.Subject
		}
		return "No commits"
	}

	// For file-based views, show the file path
	var node *FileTreeNode
	if m.focus == focusCommitPicker && m.commitPaneMode == commitPaneModeFiles {
		node = m.getSelectedCommitFileNode()
	} else {
		node = m.getSelectedWorkingDirNode()
	}

	if node == nil {
		if m.focus == focusFileList {
			return "Working Directory"
		}
		return ""
	}

	// Show full path for files, path with "/" suffix for directories
	if node.IsDir {
		return node.Path + "/"
	}
	return node.Path
}

// buildHeaderTitle constructs the header title based on the current git context.
// Returns one of four formats:
//   - "[worktree-dir] branch" when viewing a non-default worktree
//   - "other-branch" when viewing commits from another branch
//   - "branch" when on default worktree viewing HEAD
//   - "HEAD" when currentBranch is empty (detached HEAD state)
func (m Model) buildHeaderTitle() string {
	// Case 1: Non-default worktree - show "[worktree-dir] branch"
	if m.currentWorktreePath != "" && m.currentWorktreePath != m.originalWorkDir {
		wtDir := filepath.Base(m.currentWorktreePath)
		branch := m.currentWorktreeBranch
		if branch == "" {
			branch = m.currentBranch
		}
		if branch == "" {
			branch = "HEAD"
		}
		return fmt.Sprintf("[%s] %s", wtDir, branch)
	}

	// Case 2: Viewing commits from a different branch - just show that branch
	if m.viewingBranch != "" && m.viewingBranch != m.currentBranch {
		return m.viewingBranch
	}

	// Case 3 & 4: Default state - just show branch or "HEAD"
	if m.currentBranch != "" {
		return m.currentBranch
	}
	return "HEAD"
}

// getActiveFile returns the currently focused file based on focus pane and commit pane mode.
// Returns nil if no file is selected or available (including when a directory is selected).
func (m Model) getActiveFile() *DiffFile {
	switch m.focus {
	case focusFileList:
		// Working directory files pane - use tree selection
		return m.getSelectedWorkingDirFile()
	case focusCommitPicker:
		// Commit pane - depends on mode
		if m.commitPaneMode == commitPaneModeFiles {
			return m.getSelectedCommitFile()
		}
	case focusDiffPane:
		// Diff pane shows whatever was last selected in a left pane
		// Check lastLeftFocus to determine which source
		if m.lastLeftFocus == focusCommitPicker && m.commitPaneMode == commitPaneModeFiles {
			return m.getSelectedCommitFile()
		} else {
			// Default to working dir files - use tree selection
			return m.getSelectedWorkingDirFile()
		}
	}
	return nil
}

// getFileKey returns a unique key for a DiffFile to use in scroll position storage.
// Uses NewPath as the primary key, falling back to OldPath for deleted files.
func getFileKey(file *DiffFile) string {
	if file == nil {
		return ""
	}
	if file.NewPath != "" && file.NewPath != "/dev/null" {
		return file.NewPath
	}
	return file.OldPath
}

// saveScrollPosition saves the current scroll position for the active file.
func (m *Model) saveScrollPosition() {
	file := m.getActiveFile()
	key := getFileKey(file)
	if key == "" {
		return
	}
	m.scrollPositions[key] = m.getDiffYOffset()
}

// restoreScrollPosition restores the saved scroll position for a file, if any.
// Returns the scroll offset to use, or 0 if no saved position.
// Clamps the position to the valid range for the current content.
func (m *Model) restoreScrollPosition(file *DiffFile) int {
	key := getFileKey(file)
	if key == "" {
		return 0
	}
	pos, exists := m.scrollPositions[key]
	if !exists {
		return 0
	}
	// Clamp to valid range (content height may have changed)
	var maxOffset int
	if m.useVirtualScrolling && m.virtualViewport != nil {
		totalLines := m.virtualViewport.TotalLines()
		height := m.virtualViewport.Height()
		maxOffset = max(totalLines-height, 0)
	} else {
		maxOffset = max(m.diffViewport.TotalLineCount()-m.diffViewport.Height, 0)
	}
	if pos > maxOffset {
		pos = maxOffset
	}
	return pos
}

// renderDiffPane renders the diff content panel.
// When using VirtualViewport (for large diffs), renders directly with BorderedPane.
// Otherwise, uses ScrollablePane for smaller diffs.
// Title and stats are shown in the header pane, hunk indicator in top-right border.
func (m *Model) renderDiffPane(width, height int) string {
	// For virtual scrolling, render directly using VirtualViewport
	// This bypasses the viewport.SetContent() overhead entirely
	if m.useVirtualScrolling && m.virtualViewport != nil {
		return m.renderDiffPaneVirtual(width, height)
	}

	// Non-virtual path: render using BorderedPane with scrollbar support
	// (similar to renderDiffPaneVirtual but using diffViewport)
	contentHeight := height - 2 // -2 for top and bottom borders
	contentWidth := width - 2   // -2 for left and right borders

	// Safety check: ensure YOffset doesn't exceed content
	// (can happen if refreshViewport wasn't called before rendering)
	if m.diffViewport.YOffset > 0 && m.diffViewport.TotalLineCount() <= m.diffViewport.YOffset {
		m.diffViewport.SetYOffset(max(0, m.diffViewport.TotalLineCount()-1))
	}

	// Get content from viewport (already set in refreshViewport)
	content := m.diffViewport.View()

	// Pad content to fill the height (important for border rendering)
	contentLines := strings.Count(content, "\n") + 1
	if content == "" {
		contentLines = 0
	}
	if contentLines < contentHeight {
		padding := strings.Repeat("\n", contentHeight-contentLines)
		content += padding
	}

	// If scrollbar is visible, join content with scrollbar column
	// Content width is reduced by 1 for the scrollbar itself
	if m.showScrollbar {
		content = m.joinContentWithScrollbar(content, contentHeight, contentWidth-1)
	}

	// Build scroll indicator for bottom-right
	scrollIndicator := m.buildNonVirtualScrollIndicator()

	return panes.BorderedPane(panes.BorderConfig{
		Content:            content,
		Width:              width,
		Height:             height,
		TopRight:           m.buildHunkIndicator(),
		BottomRight:        scrollIndicator,
		TitleColor:         styles.TextSecondaryColor,
		BorderColor:        styles.OverlayBorderColor,
		Focused:            m.focus == focusDiffPane,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
}

// renderDiffPaneVirtual renders the diff pane using VirtualViewport for large diffs.
// This is the high-performance path that avoids the O(n) string overhead.
func (m *Model) renderDiffPaneVirtual(width, height int) string {
	// Calculate content dimensions (inside borders)
	contentHeight := height - 2 // -2 for top and bottom borders
	contentWidth := width - 2   // -2 for left and right borders

	// Render only visible lines from VirtualViewport
	content := m.virtualViewport.Render()

	// Pad content to fill the height (important for border rendering)
	contentLines := strings.Count(content, "\n") + 1
	if content == "" {
		contentLines = 0
	}
	if contentLines < contentHeight {
		padding := strings.Repeat("\n", contentHeight-contentLines)
		content += padding
	}

	// If scrollbar is visible, join content with scrollbar column
	// Content width is reduced by 1 for the scrollbar itself
	if m.showScrollbar {
		content = m.joinContentWithScrollbar(content, contentHeight, contentWidth-1)
	}

	// Build scroll indicator for bottom-right
	scrollIndicator := m.buildVirtualScrollIndicator()

	return panes.BorderedPane(panes.BorderConfig{
		Content:            content,
		Width:              width,
		Height:             height,
		TopRight:           m.buildHunkIndicator(),
		BottomRight:        scrollIndicator,
		TitleColor:         styles.TextSecondaryColor,
		BorderColor:        styles.OverlayBorderColor,
		Focused:            m.focus == focusDiffPane,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
}

// joinContentWithScrollbar joins diff content lines with scrollbar column on the right.
// contentWidth is the width of content area (excluding scrollbar).
func (m *Model) joinContentWithScrollbar(content string, viewportHeight, contentWidth int) string {
	// Build scrollbar configuration
	cfg := DefaultScrollbarConfig()
	cfg.ViewportHeight = viewportHeight

	// Get scroll position and total lines
	if m.useVirtualScrolling && m.virtualViewport != nil {
		cfg.TotalLines = m.virtualViewport.TotalLines()
		cfg.ScrollOffset = m.virtualViewport.YOffset()
	} else {
		// For non-virtual mode, use diffViewport
		cfg.TotalLines = m.diffViewport.TotalLineCount()
		cfg.ScrollOffset = m.diffViewport.YOffset
	}

	// Render the scrollbar
	scrollbar := RenderScrollbar(cfg)
	scrollbarLines := strings.Split(scrollbar, "\n")

	// Join content lines with scrollbar lines
	contentLines := strings.Split(content, "\n")
	var result strings.Builder
	result.Grow(len(content) + viewportHeight*2) // estimate

	for i := range viewportHeight {
		if i > 0 {
			result.WriteByte('\n')
		}

		// Add content line, padded to fixed width so scrollbar aligns
		line := ""
		if i < len(contentLines) {
			line = contentLines[i]
		}
		// Pad line to contentWidth (accounts for ANSI escape codes)
		lineWidth := lipgloss.Width(line)
		if lineWidth < contentWidth {
			line += strings.Repeat(" ", contentWidth-lineWidth)
		}
		result.WriteString(line)

		// Add scrollbar column
		if i < len(scrollbarLines) {
			result.WriteString(scrollbarLines[i])
		}
	}

	return result.String()
}

// buildVirtualScrollIndicator builds the scroll position indicator for virtual viewport.
func (m *Model) buildVirtualScrollIndicator() string {
	if m.virtualViewport == nil {
		return ""
	}

	// Show percentage or "top"/"bottom"
	if m.virtualViewport.AtTop() {
		return ""
	}
	if m.virtualViewport.AtBottom() {
		return "end"
	}

	percent := int(m.virtualViewport.ScrollPercent() * 100)
	return fmt.Sprintf("%d%%", percent)
}

// buildNonVirtualScrollIndicator builds the scroll position indicator for non-virtual viewport.
func (m *Model) buildNonVirtualScrollIndicator() string {
	// Check if content fits in viewport (no scrolling needed)
	if m.diffViewport.TotalLineCount() <= m.diffViewport.Height {
		return ""
	}

	// Show percentage or "top"/"bottom"
	if m.diffViewport.AtBottom() {
		return "end"
	}
	if m.diffViewport.YOffset == 0 {
		return ""
	}

	percent := int(m.diffViewport.ScrollPercent() * 100)
	return fmt.Sprintf("%d%%", percent)
}

// getOrComputeWordDiff returns cached word diff or computes it for the file.
// Uses the file path as the cache key.
func (m *Model) getOrComputeWordDiff(file DiffFile) *fileWordDiff {
	// Use NewPath as key (fallback to OldPath for deleted files)
	key := file.NewPath
	if key == "" {
		key = file.OldPath
	}
	if key == "" {
		return nil
	}

	// Check cache first
	if cached, ok := m.wordDiffCache[key]; ok {
		return cached
	}

	// Compute word diff
	wordDiff := computeFileWordDiff(file)

	// Cache the result (if cache is initialized)
	if m.wordDiffCache != nil {
		m.wordDiffCache[key] = &wordDiff
	}

	return &wordDiff
}

// getActiveNode returns the currently selected tree node based on focus pane.
// Returns nil if no selection or not in a file-based focus.
func (m Model) getActiveNode() *FileTreeNode {
	switch m.focus {
	case focusFileList:
		return m.getSelectedWorkingDirNode()
	case focusCommitPicker:
		if m.commitPaneMode == commitPaneModeFiles {
			return m.getSelectedCommitFileNode()
		}
	case focusDiffPane:
		if m.lastLeftFocus == focusCommitPicker && m.commitPaneMode == commitPaneModeFiles {
			return m.getSelectedCommitFileNode()
		}
		return m.getSelectedWorkingDirNode()
	}
	return nil
}

// renderMultiFileDiff renders a slice of DiffFiles as concatenated diff content.
// Each file gets a header, its diff content, and a trailing empty line.
// The height parameter is passed to the diff renderers for height-aware rendering;
// pass 0 when rendering into a scrollable viewport.
func (m Model) renderMultiFileDiff(files []DiffFile, width, height int) string {
	var allContent []string

	for _, file := range files {
		// Add file header
		filename := file.NewPath
		if file.IsDeleted {
			filename = file.OldPath
		}
		allContent = append(allContent, renderFileHeader(filename, file, width))

		// Add file diff content
		if file.IsBinary {
			allContent = append(allContent, "Binary file")
		} else if len(file.Hunks) == 0 {
			allContent = append(allContent, "No changes")
		} else {
			wordDiff := m.getOrComputeWordDiff(file)
			// Use appropriate renderer based on view mode
			var fileContent string
			if m.viewMode == ViewModeSideBySide {
				fileContent = renderDiffContentSideBySide(file, wordDiff, width, height)
			} else {
				fileContent = renderDiffContentWithWordDiff(file, wordDiff, width, height)
			}
			allContent = append(allContent, fileContent)
		}
		allContent = append(allContent, "") // Empty line between files
	}

	return strings.Join(allContent, "\n")
}

// renderDirectoryDiffContent renders the combined diff for all files under a directory.
func (m Model) renderDirectoryDiffContent(dirNode *FileTreeNode, width int) string {
	filePointers := dirNode.CollectFiles()
	if len(filePointers) == 0 {
		return "No files in directory"
	}

	// Convert []*DiffFile to []DiffFile for helper
	files := make([]DiffFile, len(filePointers))
	for i, f := range filePointers {
		files[i] = *f
	}

	return m.renderMultiFileDiff(files, width, 0)
}

// renderError renders an error state.
// If the error is a DiffError, uses the enhanced error state with recovery actions.
func (m Model) renderError(width, height int) string {
	if m.err == nil {
		return ""
	}

	// Check if error is a DiffError for enhanced rendering
	var diffErr DiffError
	if errors.As(m.err, &diffErr) {
		return renderErrorState(diffErr, width, height)
	}

	// Fallback for standard errors - create a generic DiffError
	genericErr := NewDiffError(ErrCategoryGitOp, m.err.Error()).
		WithHelpText("An unexpected error occurred.")
	return renderErrorState(genericErr, width, height)
}

// Overlay renders the diff viewer centered on the given background.
func (m Model) Overlay(bg string) string {
	if !m.visible {
		return bg
	}
	fg := m.View()
	return overlay.Place(overlay.Config{
		Width:    m.width,
		Height:   m.height,
		Position: overlay.Center,
	}, fg, bg)
}

// Visible returns whether the overlay is currently visible.
func (m Model) Visible() bool {
	return m.visible
}

// Show makes the overlay visible and sets loading state.
// Resets both file and commit state.
func (m Model) Show() Model {
	m.visible = true
	m.err = nil
	// Reset working directory state
	m.workingDirFiles = nil
	m.workingDirTree = nil
	m.selectedWorkingDirNode = 0
	m.workingDirTreeScrollTop = 0
	// Reset commit state
	m.commits = nil
	m.selectedCommit = 0
	m.commitScrollTop = 0
	m.currentBranch = ""
	m.lastLeftFocus = focusFileList
	// Reset commit pane mode state
	m.commitPaneMode = commitPaneModeList
	m.commitFiles = nil
	m.commitFilesTree = nil
	m.selectedCommitFileNode = 0
	m.commitFilesTreeScrollTop = 0
	m.inspectedCommit = nil
	// Reset commit preview state
	m.previewCommitFiles = nil
	m.previewCommitHash = ""
	m.previewCommitLoading = false
	// Reset scroll positions for fresh start
	m.scrollPositions = make(map[string]int)
	return m
}

// ShowAndLoad makes the overlay visible and returns commands to load both
// working directory diff and commit history concurrently.
// Also pre-loads branches and worktrees to avoid flash when switching tabs.
// This is the preferred way to show the diff viewer.
func (m Model) ShowAndLoad() (Model, tea.Cmd) {
	m = m.Show()
	return m, tea.Batch(
		m.LoadWorkingDirDiff(),
		m.LoadCommits(),
		m.loadBranches(),
		m.loadWorktrees(),
	)
}

// Hide makes the overlay invisible.
func (m Model) Hide() Model {
	m.visible = false
	return m
}

// SetSize updates the overlay's knowledge of viewport size.
// If the terminal is too narrow for side-by-side view and the user prefers it,
// the effective view mode is constrained to unified.
func (m Model) SetSize(width, height int) Model {
	m.width = width
	m.height = height

	// Enforce view mode constraints based on width.
	// If terminal is too narrow for side-by-side, force unified view.
	// When terminal widens again, don't auto-switch (respect user preference).
	if width < minSideBySideWidth && m.viewMode == ViewModeSideBySide {
		m.viewMode = ViewModeUnified
		// Update virtual content view mode
		if m.virtualContent != nil {
			m.virtualContent.SetViewMode(m.viewMode)
		}
	} else if width >= minSideBySideWidth && m.preferredViewMode == ViewModeSideBySide {
		// Terminal is wide enough and user prefers side-by-side, restore it
		m.viewMode = ViewModeSideBySide
		if m.virtualContent != nil {
			m.virtualContent.SetViewMode(m.viewMode)
		}
	}

	m.refreshViewport()
	return m
}

// SetGitExecutor sets the git executor for diff operations.
func (m Model) SetGitExecutor(ge git.GitExecutor) Model {
	m.gitExecutor = ge
	return m
}

// LoadCommits creates a command to asynchronously fetch commit history.
// Loads up to 50 commits and the current branch name.
// Handles ErrDetachedHead by using "HEAD" as the branch name.
func (m Model) LoadCommits() tea.Cmd {
	return func() tea.Msg {
		if m.gitExecutor == nil {
			return CommitsLoadedMsg{Commits: nil, Branch: "", Err: nil}
		}

		// Get commit history
		commits, err := m.gitExecutor.GetCommitLog(commitHistoryLimit)
		if err != nil {
			return CommitsLoadedMsg{Err: err}
		}

		// Get current branch name for header display
		branch, err := m.gitExecutor.GetCurrentBranch()
		if err != nil {
			// Handle detached HEAD gracefully - use "HEAD" as branch name
			if errors.Is(err, git.ErrDetachedHead) {
				branch = "HEAD"
			} else {
				return CommitsLoadedMsg{Err: err}
			}
		}

		return CommitsLoadedMsg{Commits: commits, Branch: branch}
	}
}

// LoadWorkingDirDiff creates a command to asynchronously fetch working directory changes.
func (m Model) LoadWorkingDirDiff() tea.Cmd {
	return func() tea.Msg {
		if m.gitExecutor == nil {
			return WorkingDirDiffLoadedMsg{Files: nil, Err: nil}
		}

		output, err := m.gitExecutor.GetWorkingDirDiff()
		if err != nil {
			return WorkingDirDiffLoadedMsg{Err: err}
		}

		files, err := parseDiff(output)
		if err != nil {
			return WorkingDirDiffLoadedMsg{Err: err}
		}

		// Also fetch untracked files and add them as new files
		untrackedPaths, err := m.gitExecutor.GetUntrackedFiles()
		if err != nil {
			// Non-fatal: just log and continue with tracked files only
			return WorkingDirDiffLoadedMsg{Files: files}
		}

		// Add untracked files as new (untracked) files with synthetic diff content
		for _, path := range untrackedPaths {
			file := DiffFile{
				NewPath:     path,
				IsNew:       true,
				IsUntracked: true,
			}

			// Read file content to create synthetic diff showing all lines as additions
			content, err := m.gitExecutor.GetFileContent(path)
			if err == nil && content != "" {
				lines := strings.Split(content, "\n")
				var diffLines []DiffLine
				for i, line := range lines {
					// Skip trailing empty line from Split
					if i == len(lines)-1 && line == "" {
						continue
					}
					diffLines = append(diffLines, DiffLine{
						Type:       LineAddition,
						NewLineNum: i + 1,
						Content:    line,
					})
				}
				file.Additions = len(diffLines)
				if len(diffLines) > 0 {
					file.Hunks = []DiffHunk{{
						NewStart: 1,
						NewCount: len(diffLines),
						Header:   fmt.Sprintf("@@ -0,0 +1,%d @@ (new file)", len(diffLines)),
						Lines:    diffLines,
					}}
				}
			}

			files = append(files, file)
		}

		return WorkingDirDiffLoadedMsg{Files: files}
	}
}

// LoadCommitFiles creates a command to asynchronously fetch files changed in a specific commit.
func (m Model) LoadCommitFiles(hash string) tea.Cmd {
	return func() tea.Msg {
		if m.gitExecutor == nil {
			return CommitFilesLoadedMsg{Hash: hash, Files: nil, Err: nil}
		}

		output, err := m.gitExecutor.GetCommitDiff(hash)
		if err != nil {
			return CommitFilesLoadedMsg{Hash: hash, Err: err}
		}

		files, err := parseDiff(output)
		if err != nil {
			return CommitFilesLoadedMsg{Hash: hash, Err: err}
		}

		return CommitFilesLoadedMsg{Hash: hash, Files: files}
	}
}

// LoadCommitPreview creates a command to asynchronously fetch the full diff for a commit preview.
// This is used when a commit is highlighted in ListMode to show the full diff in the diff pane.
// NOTE: Callers must set m.previewCommitHash and m.previewCommitLoading = true before calling,
// since this method uses a value receiver and cannot modify the caller's model.
func (m Model) LoadCommitPreview(hash string) tea.Cmd {
	return func() tea.Msg {
		if m.gitExecutor == nil {
			return CommitPreviewLoadedMsg{Hash: hash, Files: nil, Err: nil}
		}

		output, err := m.gitExecutor.GetCommitDiff(hash)
		if err != nil {
			return CommitPreviewLoadedMsg{Hash: hash, Err: err}
		}

		files, err := parseDiff(output)
		if err != nil {
			return CommitPreviewLoadedMsg{Hash: hash, Err: err}
		}

		return CommitPreviewLoadedMsg{Hash: hash, Files: files}
	}
}

// refreshViewport updates the viewport content based on current focus and mode.
// When focused on FileList or CommitPicker in FilesMode, shows single file diff.
// When focused on CommitPicker in ListMode, shows full commit diff (all files).
func (m *Model) refreshViewport() {
	if m.width == 0 || m.height == 0 {
		return
	}

	boxWidth := m.boxWidth()
	contentHeight := m.contentHeight()
	fileListWidth := m.fileListWidth(boxWidth)
	// diffWidth is the width of the diff pane INCLUDING its borders
	// This must match renderSplitView's calculation
	diffWidth := boxWidth - fileListWidth
	// diffContentWidth is the content area inside the diff pane (minus border chars)
	diffContentWidth := diffWidth - 2
	// diffPaneHeight is the height of the diff pane (contentHeight minus headerPane, minus borders)
	// This must match renderSplitView's calculation: diffHeight = height - headerPaneHeight
	// Then inside renderDiffPane: contentHeight = height - 2 (for borders)
	diffPaneContentHeight := contentHeight - headerPaneHeight - 2

	// Initialize viewport height (width set later after showScrollbar is determined)
	m.diffViewport.Height = diffPaneContentHeight

	// Get files to display and determine if single-file view
	var files []DiffFile
	var singleFile *DiffFile
	var dirNode *FileTreeNode
	isCommitPreview := m.focus == focusCommitPicker && m.commitPaneMode == commitPaneModeList
	if isCommitPreview {
		files = m.previewCommitFiles
	} else {
		// Check if a directory is selected
		if node := m.getActiveNode(); node != nil && node.IsDir {
			dirNode = node
			// Collect files from directory for line count calculation
			filePointers := node.CollectFiles()
			for _, f := range filePointers {
				if f != nil {
					files = append(files, *f)
				}
			}
		} else {
			singleFile = m.getActiveFile()
			if singleFile != nil {
				files = []DiffFile{*singleFile}
			}
		}
	}

	// Calculate total line count to determine if we need virtual scrolling
	totalLines := countTotalLines(files)

	// For commit preview, add lines for commit header (hash, author, date, empty, subject, empty)
	if isCommitPreview && len(m.commits) > 0 && m.selectedCommit < len(m.commits) {
		totalLines += 6 // commit header lines
	}

	// Determine if scrollbar will be visible (affects content width)
	// Scrollbar shows when content exceeds viewport height (diff pane content area)
	m.showScrollbar = totalLines > diffPaneContentHeight

	// Reserve 1 character for scrollbar when visible
	// effectiveContentWidth is the width available for actual diff content
	effectiveContentWidth := diffContentWidth
	if m.showScrollbar {
		effectiveContentWidth = diffContentWidth - 1
	}

	// Set viewport width to match content width (must be done after showScrollbar is determined)
	m.diffViewport.Width = effectiveContentWidth

	// Use virtual scrolling for large diffs in both unified and side-by-side modes.
	// Side-by-side virtual scrolling is now supported via pre-computed aligned pairs.
	if totalLines > virtualScrollingThreshold {
		m.useVirtualScrolling = true
		m.virtualContent = NewVirtualContent(files, DefaultVirtualContentConfig())
		m.virtualContent.SetWidth(effectiveContentWidth)
		m.virtualContent.SetViewMode(m.viewMode) // Sync view mode for rendering

		// Create VirtualViewport for efficient rendering (no padding overhead)
		m.virtualViewport = NewVirtualViewport(m.virtualContent)
		m.virtualViewport.SetSize(effectiveContentWidth, contentHeight)
		m.lastViewportYOffset = 0

		// Restore scroll position for single-file views, otherwise start at top
		if singleFile != nil {
			savedPos := m.restoreScrollPosition(singleFile)
			if savedPos > 0 {
				m.virtualViewport.SetYOffset(savedPos)
				m.lastViewportYOffset = savedPos
			}
		}

		// Note: We no longer set content on diffViewport for virtual scrolling.
		// Rendering is done directly via m.virtualViewport.Render() in renderDiffPane.
	} else {
		// Standard rendering for smaller diffs
		m.useVirtualScrolling = false
		m.virtualContent = nil
		m.virtualViewport = nil

		var content string
		if isCommitPreview {
			content = m.renderFullCommitDiff(effectiveContentWidth, contentHeight)
		} else if dirNode != nil {
			// Directory view: render all files under the directory
			content = m.renderDirectoryDiffContent(dirNode, effectiveContentWidth)
		} else if singleFile != nil {
			wordDiff := m.getOrComputeWordDiff(*singleFile)
			// Use view mode to select renderer
			// Side-by-side renders both columns as single combined lines,
			// so scrolling is automatically synchronized
			if m.viewMode == ViewModeSideBySide {
				content = renderDiffContentSideBySide(*singleFile, wordDiff, effectiveContentWidth, contentHeight)
			} else {
				content = renderDiffContentWithWordDiff(*singleFile, wordDiff, effectiveContentWidth, contentHeight)
			}
		}
		m.diffViewport.SetContent(content)

		// Restore scroll position for single-file views, otherwise start at top
		// Directory views don't restore position (they're aggregate views)
		if singleFile != nil {
			savedPos := m.restoreScrollPosition(singleFile)
			if savedPos > 0 {
				m.diffViewport.SetYOffset(savedPos)
			} else {
				m.diffViewport.GotoTop()
			}
		} else {
			m.diffViewport.GotoTop()
		}
	}
}

// countTotalLines counts the total number of diff lines across all files.
func countTotalLines(files []DiffFile) int {
	total := 0
	for _, file := range files {
		for _, hunk := range file.Hunks {
			total += len(hunk.Lines)
		}
		// Add 1 for file header in multi-file views
		if len(files) > 1 {
			total++ // file header
			total++ // empty line between files
		}
	}
	return total
}

// scrollDiffUp scrolls the diff content up by n lines.
// Uses VirtualViewport when virtual scrolling is enabled, otherwise bubbles viewport.
func (m *Model) scrollDiffUp(n int) {
	if m.useVirtualScrolling && m.virtualViewport != nil {
		m.virtualViewport.ScrollUp(n)
	} else {
		m.diffViewport.ScrollUp(n)
	}
}

// scrollDiffDown scrolls the diff content down by n lines.
// Uses VirtualViewport when virtual scrolling is enabled, otherwise bubbles viewport.
func (m *Model) scrollDiffDown(n int) {
	if m.useVirtualScrolling && m.virtualViewport != nil {
		m.virtualViewport.ScrollDown(n)
	} else {
		m.diffViewport.ScrollDown(n)
	}
}

// gotoTopDiff scrolls to the top of the diff content.
func (m *Model) gotoTopDiff() {
	if m.useVirtualScrolling && m.virtualViewport != nil {
		m.virtualViewport.GotoTop()
	} else {
		m.diffViewport.GotoTop()
	}
}

// gotoBottomDiff scrolls to the bottom of the diff content.
func (m *Model) gotoBottomDiff() {
	if m.useVirtualScrolling && m.virtualViewport != nil {
		m.virtualViewport.GotoBottom()
	} else {
		m.diffViewport.GotoBottom()
	}
}

// getDiffYOffset returns the current scroll position in the diff content.
func (m *Model) getDiffYOffset() int {
	if m.useVirtualScrolling && m.virtualViewport != nil {
		return m.virtualViewport.YOffset()
	}
	return m.diffViewport.YOffset
}

// setDiffYOffset sets the scroll position in the diff content.
// Note: For diffViewport, we directly assign YOffset to match legacy behavior.
// The viewport's SetYOffset method clamps values, which can prevent navigation
// to positions that haven't been rendered yet.
func (m *Model) setDiffYOffset(offset int) {
	if m.useVirtualScrolling && m.virtualViewport != nil {
		m.virtualViewport.SetYOffset(offset)
	} else {
		m.diffViewport.YOffset = offset
	}
}

// renderFullCommitDiff renders all files from the preview commit as a single diff view.
// Includes commit header (hash, author, date, message) followed by all file diffs.
func (m Model) renderFullCommitDiff(width, height int) string {
	// Ensure we have a valid selected commit
	if len(m.commits) == 0 || m.selectedCommit >= len(m.commits) {
		return renderEnhancedEmptyState(width, height)
	}

	commit := m.commits[m.selectedCommit]

	// If preview files are loading or belong to a different commit, show loading state
	if m.previewCommitLoading || m.previewCommitHash != commit.Hash {
		return renderEnhancedLoadingState(width, height, "Loading commit diff...")
	}

	if len(m.previewCommitFiles) == 0 {
		return renderEnhancedEmptyState(width, height)
	}

	var allContent []string

	// Add commit header
	allContent = append(allContent, m.renderCommitHeader(commit, width))
	allContent = append(allContent, "") // Empty line after header

	// Render all files
	allContent = append(allContent, m.renderMultiFileDiff(m.previewCommitFiles, width, height))

	return strings.Join(allContent, "\n")
}

// renderCommitHeader renders the commit metadata header (similar to git log -p output).
func (m Model) renderCommitHeader(commit git.CommitInfo, width int) string {
	hashStyle := lipgloss.NewStyle().Foreground(styles.DiffHunkColor).Bold(true)
	authorStyle := lipgloss.NewStyle().Foreground(styles.TextSecondaryColor)
	dateStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	subjectStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)

	var lines []string

	// Commit hash line
	hashLine := hashStyle.Render("commit " + commit.Hash)
	lines = append(lines, hashLine)

	// Author line
	authorLine := authorStyle.Render("Author: " + commit.Author)
	lines = append(lines, authorLine)

	// Date line with relative time
	relTime := shared.FormatRelativeTimeFrom(commit.Date, m.clock.Now())
	dateLine := dateStyle.Render("Date:   " + commit.Date.Format("Mon Jan 2 15:04:05 2006 -0700") + " (" + relTime + ")")
	lines = append(lines, dateLine)

	// Empty line before subject
	lines = append(lines, "")

	// Subject (commit message first line) - indented like git log
	subject := subjectStyle.Render("    " + commit.Subject)
	if lipgloss.Width(subject) > width {
		subject = styles.TruncateString(subject, width)
	}
	lines = append(lines, subject)

	return strings.Join(lines, "\n")
}

// renderFileHeader renders a header line for a file in the full commit diff.
func renderFileHeader(filename string, file DiffFile, width int) string {
	filenameStyle := lipgloss.NewStyle().
		Foreground(styles.TextPrimaryColor).
		Bold(true).
		Background(styles.SelectionBackgroundColor)

	// Calculate stats width to reserve space
	statsWidth := 0
	if file.IsBinary {
		statsWidth = 8 // " binary"
	} else {
		if file.Additions > 0 {
			statsWidth += len(fmt.Sprintf("+%d", file.Additions)) + 1
		}
		if file.Deletions > 0 {
			statsWidth += len(fmt.Sprintf("-%d", file.Deletions)) + 1
		}
	}

	// Truncate filename if needed (leave room for stats)
	maxFilenameWidth := max(width-statsWidth, 10)
	if lipgloss.Width(filename) > maxFilenameWidth {
		filename = styles.TruncateString(filename, maxFilenameWidth)
	}

	// Render filename with highlight (no background on stats)
	result := filenameStyle.Render(filename)

	// Append stats with color-coded styling (no background)
	if file.IsBinary {
		binaryStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
		result += " " + binaryStyle.Render("binary")
	} else {
		if file.Additions > 0 {
			addStyle := lipgloss.NewStyle().Foreground(styles.DiffAdditionColor)
			result += " " + addStyle.Render(fmt.Sprintf("+%d", file.Additions))
		}
		if file.Deletions > 0 {
			delStyle := lipgloss.NewStyle().Foreground(styles.DiffDeletionColor)
			result += " " + delStyle.Render(fmt.Sprintf("-%d", file.Deletions))
		}
	}

	return result
}

// boxWidth returns the calculated box width based on screen size.
func (m Model) boxWidth() int {
	return max(min(m.width-boxPaddingX, boxMaxWidth), boxMinWidth)
}

// boxHeight returns the total height of the diffviewer box.
func (m Model) boxHeight() int {
	return m.contentHeight() + footerPaneHeight
}

// contentHeight returns the available content height for the main panes (excluding footer).
func (m Model) contentHeight() int {
	availableHeight := min(m.height-boxPaddingY, m.height-4)
	return max(availableHeight-footerPaneHeight, 5)
}

// fileListWidth returns the width for the file list panel.
func (m Model) fileListWidth(totalWidth int) int {
	calculated := int(float64(totalWidth) * fileListRatio)
	return max(min(calculated, fileListMaxWidth), fileListMinWidth)
}

// loadWorktrees creates a command to asynchronously fetch available worktrees.
func (m Model) loadWorktrees() tea.Cmd {
	return func() tea.Msg {
		if m.gitExecutor == nil {
			return WorktreesLoadedMsg{Err: errors.New("no git executor")}
		}
		worktrees, err := m.gitExecutor.ListWorktrees()
		return WorktreesLoadedMsg{Worktrees: worktrees, Err: err}
	}
}

// handleWorktreeSelected processes worktree selection.
// It finds the matching WorktreeInfo from worktreeList, creates a new git executor
// via the factory (if available), updates state fields, clears viewingBranch, and
// triggers a reload.
func (m Model) handleWorktreeSelected(wtPath string) (Model, tea.Cmd) {
	// If no factory is available, gracefully no-op (can't switch worktrees without factory)
	if m.gitExecutorFactory == nil {
		return m, nil
	}

	// Find matching WorktreeInfo from worktreeList
	var selectedWorktree *git.WorktreeInfo
	for _, wt := range m.worktreeList {
		if wt.Path == wtPath {
			selectedWorktree = &wt
			break
		}
	}

	if selectedWorktree == nil {
		m.err = fmt.Errorf("worktree not found: %s", filepath.Base(wtPath))
		return m, nil
	}

	// Create new executor via factory for the selected worktree
	m.gitExecutor = m.gitExecutorFactory(wtPath)

	// Update state fields
	m.currentWorktreePath = selectedWorktree.Path
	m.currentWorktreeBranch = selectedWorktree.Branch

	// Clear viewingBranch (reset to HEAD of the new worktree)
	m.viewingBranch = ""

	// Trigger reload - this will reload commits and working dir diff using the new executor
	return m.ShowAndLoad()
}

// handleBranchSelection handles branch selection from the Branches tab.
// It loads commits for the selected branch and auto-switches to the Commits tab.
func (m Model) handleBranchSelection(branchName string) (Model, tea.Cmd) {
	m.activeCommitTab = commitsTabCommits
	return m, m.loadCommitsForBranch(branchName)
}

// handleWorktreeSelectionFromTab handles worktree selection from the Worktrees tab.
// It validates the worktree path exists, then switches to it and auto-switches to Commits tab.
func (m Model) handleWorktreeSelectionFromTab(wtPath string) (Model, tea.Cmd) {
	// Validate worktree still exists
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		m.err = fmt.Errorf("worktree no longer exists: %s", filepath.Base(wtPath))
		return m, nil
	}
	m.activeCommitTab = commitsTabCommits
	return m.handleWorktreeSelected(wtPath)
}

// loadBranches creates a command to asynchronously fetch available branches.
func (m Model) loadBranches() tea.Cmd {
	return func() tea.Msg {
		if m.gitExecutor == nil {
			return BranchesLoadedMsg{Err: errors.New("no git executor")}
		}
		branches, err := m.gitExecutor.ListBranches()
		return BranchesLoadedMsg{Branches: branches, Err: err}
	}
}

// nextCommitTab cycles to the next tab and triggers lazy loading if needed.
func (m Model) nextCommitTab() (Model, tea.Cmd) {
	m.activeCommitTab = (m.activeCommitTab + 1) % 3
	return m.ensureTabDataLoaded()
}

// prevCommitTab cycles to the previous tab and triggers lazy loading if needed.
func (m Model) prevCommitTab() (Model, tea.Cmd) {
	m.activeCommitTab = (m.activeCommitTab + 2) % 3 // +2 mod 3 = -1 mod 3
	return m.ensureTabDataLoaded()
}

// ensureTabDataLoaded triggers async loading for the active tab if not already loaded.
func (m Model) ensureTabDataLoaded() (Model, tea.Cmd) {
	switch m.activeCommitTab {
	case commitsTabBranches:
		if !m.branchListLoaded {
			return m, m.loadBranches()
		}
	case commitsTabWorktrees:
		if !m.worktreeListLoaded {
			return m, m.loadWorktrees()
		}
	}
	return m, nil
}

// loadCommitsForBranch creates a command to asynchronously fetch commits for a specific branch.
func (m Model) loadCommitsForBranch(branch string) tea.Cmd {
	return func() tea.Msg {
		if m.gitExecutor == nil {
			return CommitsForBranchLoadedMsg{Err: errors.New("no git executor"), Branch: branch}
		}
		commits, err := m.gitExecutor.GetCommitLogForRef(branch, commitHistoryLimit)
		return CommitsForBranchLoadedMsg{Commits: commits, Branch: branch, Err: err}
	}
}

// toggleHelpOverlay shows the context-sensitive help overlay.
// The help content changes based on the current focus pane.
func (m Model) toggleHelpOverlay() Model {
	m.showHelpOverlay = !m.showHelpOverlay
	if m.showHelpOverlay {
		// Set help context based on current focus
		ctx := m.currentHelpContext()
		m.helpOverlay = newHelp().
			SetContext(ctx).
			SetSize(m.boxWidth(), m.boxHeight())
	}
	return m
}

// currentHelpContext determines the appropriate help context based on focus and mode.
func (m Model) currentHelpContext() helpContext {
	switch m.focus {
	case focusFileList:
		return helpContextFileList
	case focusCommitPicker:
		if m.commitPaneMode == commitPaneModeFiles {
			return helpContextCommitFiles
		}
		return helpContextCommits
	case focusDiffPane:
		return helpContextDiff
	default:
		return helpContextFileList
	}
}

// executeCommand handles execution of a command based on its commandID.
func (m *Model) executeCommand(cmdID commandID) (Model, tea.Cmd) {
	switch cmdID {
	// Navigation commands
	case cmdFocusFileList:
		m.focus = focusFileList
		m.refreshViewport()
		return *m, nil

	case cmdFocusCommits:
		m.focus = focusCommitPicker
		// Load commit preview if we have commits and preview not already loaded
		if len(m.commits) > 0 && m.commitPaneMode == commitPaneModeList {
			hash := m.commits[m.selectedCommit].Hash
			if m.previewCommitHash != hash {
				m.previewCommitHash = hash
				m.previewCommitLoading = true
				return *m, m.LoadCommitPreview(hash)
			}
		}
		m.refreshViewport()
		return *m, nil

	case cmdFocusDiff:
		if m.focus != focusDiffPane {
			m.lastLeftFocus = m.focus
		}
		m.focus = focusDiffPane
		m.refreshViewport()
		return *m, nil

	case cmdCyclePanes:
		prevFocus := m.focus
		switch m.focus {
		case focusFileList:
			m.focus = focusCommitPicker
		case focusCommitPicker:
			m.focus = focusDiffPane
		case focusDiffPane:
			m.focus = focusFileList
		}
		// Load commit preview when switching to CommitPicker
		if m.focus == focusCommitPicker && prevFocus != focusCommitPicker {
			if len(m.commits) > 0 && m.commitPaneMode == commitPaneModeList {
				hash := m.commits[m.selectedCommit].Hash
				if m.previewCommitHash != hash {
					m.previewCommitHash = hash
					m.previewCommitLoading = true
					return *m, m.LoadCommitPreview(hash)
				}
			}
		}
		m.refreshViewport()
		return *m, nil

	case cmdNextFile:
		switch m.focus {
		case focusFileList:
			return m.handleFileListDown()
		case focusCommitPicker:
			return m.handleCommitPaneDown()
		case focusDiffPane:
			m.scrollDiffDown(1)
		}
		return *m, nil

	case cmdPrevFile:
		switch m.focus {
		case focusFileList:
			return m.handleFileListUp()
		case focusCommitPicker:
			return m.handleCommitPaneUp()
		case focusDiffPane:
			m.scrollDiffUp(1)
		}
		return *m, nil

	// Scrolling commands
	case cmdScrollUp:
		m.scrollDiffUp(m.diffViewport.Height / 2)
		return *m, nil

	case cmdScrollDown:
		m.scrollDiffDown(m.diffViewport.Height / 2)
		return *m, nil

	case cmdGotoTop:
		if m.focus == focusDiffPane {
			m.gotoTopDiff()
		}
		return *m, nil

	case cmdGotoBottom:
		if m.focus == focusDiffPane {
			m.gotoBottomDiff()
		}
		return *m, nil

	// Actions
	case cmdSelectItem:
		switch m.focus {
		case focusFileList:
			if m.workingDirTree != nil {
				nodes := m.workingDirTree.VisibleNodes()
				if m.selectedWorkingDirNode < len(nodes) {
					node := nodes[m.selectedWorkingDirNode]
					if node.IsDir {
						return m.handleFileListToggle()
					}
				}
			}
			// For files, focus diff pane
			if m.getSelectedWorkingDirFile() != nil {
				m.lastLeftFocus = focusFileList
				m.focus = focusDiffPane
				m.refreshViewport()
			}
			return *m, nil
		case focusCommitPicker:
			// Tab-aware selection handling
			switch m.activeCommitTab {
			case commitsTabCommits:
				if m.commitPaneMode == commitPaneModeList {
					return m.drillIntoCommit()
				}
				if m.commitFilesTree != nil {
					nodes := m.commitFilesTree.VisibleNodes()
					if m.selectedCommitFileNode < len(nodes) {
						node := nodes[m.selectedCommitFileNode]
						if node.IsDir {
							return m.handleCommitFilesToggle()
						}
					}
				}
				// For files, focus diff pane
				if m.getSelectedCommitFile() != nil {
					m.lastLeftFocus = focusCommitPicker
					m.focus = focusDiffPane
					m.refreshViewport()
				}
			case commitsTabBranches:
				// Select branch and switch to Commits tab
				if len(m.branchList) > 0 && m.selectedBranch < len(m.branchList) {
					return m.handleBranchSelection(m.branchList[m.selectedBranch].Name)
				}
			case commitsTabWorktrees:
				// Select worktree and switch to Commits tab
				if len(m.worktreeList) > 0 && m.selectedWorktree < len(m.worktreeList) {
					return m.handleWorktreeSelectionFromTab(m.worktreeList[m.selectedWorktree].Path)
				}
			}
			return *m, nil
		}
		return *m, nil

	case cmdGoBack:
		if m.focus == focusCommitPicker && m.commitPaneMode == commitPaneModeFiles {
			m.commitPaneMode = commitPaneModeList
			m.commitFiles = nil
			m.inspectedCommit = nil
			m.refreshViewport()
			return *m, nil
		}
		// Otherwise close the overlay
		m.visible = false
		return *m, func() tea.Msg { return HideDiffViewerMsg{} }

	// View mode commands
	case cmdToggleViewMode:
		return m.toggleViewMode()

	// Navigation direction commands (h/l keys)
	case cmdFocusLeft:
		// h key: Move up in left column (CommitPicker → FileList), or return from DiffPane
		switch m.focus {
		case focusCommitPicker:
			m.focus = focusFileList
		case focusDiffPane:
			m.focus = m.lastLeftFocus
		}
		return *m, nil

	case cmdFocusRight:
		// l key: Move down in left column (FileList → CommitPicker)
		if m.focus == focusFileList {
			m.focus = focusCommitPicker
			// Load commit preview if we have commits and preview not already loaded
			if len(m.commits) > 0 && m.commitPaneMode == commitPaneModeList {
				hash := m.commits[m.selectedCommit].Hash
				if m.previewCommitHash != hash {
					m.previewCommitHash = hash
					m.previewCommitLoading = true
					return *m, m.LoadCommitPreview(hash)
				}
			}
			m.refreshViewport()
		}
		return *m, nil

	// Hunk navigation commands
	case cmdNextHunk:
		return m.navigateToNextHunk()

	case cmdPrevHunk:
		return m.navigateToPrevHunk()

	// Copy commands
	case cmdCopyHunk:
		return m.copyCurrentHunk()

	// Help and UI
	case cmdShowHelp:
		return m.toggleHelpOverlay(), nil

	case cmdCloseViewer:
		m.visible = false
		return *m, func() tea.Msg { return HideDiffViewerMsg{} }
	}

	return *m, nil
}

// navigateToNextHunk jumps to the next hunk in the diff.
// Works in single-file view (working directory or commit files), directory view, and commit preview.
// In single-file view, wraps to the first hunk when reaching the end.
// In multi-file views (directory or commit preview), moves to the next file's first hunk if needed.
func (m Model) navigateToNextHunk() (Model, tea.Cmd) {
	currentOffset := (&m).getDiffYOffset()

	// Get hunk positions based on current view
	hunkPositions := m.getHunkPositionsForCurrentView()
	if len(hunkPositions) == 0 {
		return m, nil
	}

	// Find the next hunk position after current offset
	for _, pos := range hunkPositions {
		if pos > currentOffset {
			(&m).setDiffYOffset(pos)
			return m, nil
		}
	}

	// Wrap to first hunk
	(&m).setDiffYOffset(hunkPositions[0])
	return m, nil
}

// navigateToPrevHunk jumps to the previous hunk in the diff.
// Works in single-file view (working directory or commit files), directory view, and commit preview.
// In single-file view, wraps to the last hunk when reaching the beginning.
// In multi-file views (directory or commit preview), moves to the previous file's last hunk if needed.
func (m Model) navigateToPrevHunk() (Model, tea.Cmd) {
	currentOffset := (&m).getDiffYOffset()

	// Get hunk positions based on current view
	hunkPositions := m.getHunkPositionsForCurrentView()
	if len(hunkPositions) == 0 {
		return m, nil
	}

	// Find the previous hunk position before current offset
	for i := len(hunkPositions) - 1; i >= 0; i-- {
		if hunkPositions[i] < currentOffset {
			(&m).setDiffYOffset(hunkPositions[i])
			return m, nil
		}
	}

	// Wrap to last hunk
	(&m).setDiffYOffset(hunkPositions[len(hunkPositions)-1])
	return m, nil
}

// toggleViewMode switches between unified and side-by-side view modes.
// If the terminal is too narrow for side-by-side view, returns a ViewModeConstrainedMsg
// instead of switching. Scroll position is preserved as much as possible when switching modes.
func (m Model) toggleViewMode() (Model, tea.Cmd) {
	// Determine which mode to toggle to
	var targetMode ViewMode
	if m.preferredViewMode == ViewModeUnified {
		targetMode = ViewModeSideBySide
	} else {
		targetMode = ViewModeUnified
	}

	// Update user preference regardless of width constraints
	m.preferredViewMode = targetMode

	// Check if we can actually use the target mode
	if targetMode == ViewModeSideBySide && m.width < minSideBySideWidth {
		// Terminal too narrow for side-by-side - return message for toast
		return m, func() tea.Msg {
			return ViewModeConstrainedMsg{
				RequestedMode: ViewModeSideBySide,
				MinWidth:      minSideBySideWidth,
				CurrentWidth:  m.width,
			}
		}
	}

	// Can switch to target mode
	m.viewMode = targetMode

	// Update virtual content view mode if using virtual scrolling
	if m.virtualContent != nil {
		m.virtualContent.SetViewMode(m.viewMode)
	}

	// Refresh the viewport to re-render with the new view mode
	// The scroll position (YOffset) is preserved automatically
	m.refreshViewport()

	return m, nil
}

// getHunkPositionsForCurrentView returns hunk positions based on the current view mode.
// Handles single-file view, directory view, and commit preview.
func (m Model) getHunkPositionsForCurrentView() []int {
	isCommitPreview := m.focus == focusCommitPicker && m.commitPaneMode == commitPaneModeList

	if isCommitPreview {
		return m.getHunkPositionsForCommitPreview()
	}

	// Check if viewing a directory
	if node := m.getActiveNode(); node != nil && node.IsDir {
		return m.getHunkPositionsForDirectory(node)
	}

	// Single-file view
	file := m.getActiveFile()
	if file == nil || len(file.Hunks) == 0 {
		return nil
	}
	return m.getHunkPositionsForFile(file)
}

// getCurrentHunkIndex returns the 1-based index of the hunk currently visible in the viewport.
// Returns 0 if no hunks are visible or no file is selected.
// The current hunk is determined by finding the last hunk position <= viewport Y offset.
func (m Model) getCurrentHunkIndex() int {
	positions := m.getHunkPositionsForCurrentView()
	if len(positions) == 0 {
		return 0
	}

	currentPos := (&m).getDiffYOffset()
	currentHunk := 1 // 1-based index

	for i, pos := range positions {
		if pos <= currentPos {
			currentHunk = i + 1
		} else {
			break
		}
	}

	return currentHunk
}

// getTotalHunkCount returns the total number of hunks in the current view.
func (m Model) getTotalHunkCount() int {
	positions := m.getHunkPositionsForCurrentView()
	return len(positions)
}

// buildHunkIndicator returns a string like "2 / 10 hunks" for the diff pane border.
// Returns empty string if no hunks or no file is selected.
func (m Model) buildHunkIndicator() string {
	total := m.getTotalHunkCount()
	if total == 0 {
		return ""
	}

	current := m.getCurrentHunkIndex()
	if total == 1 {
		return "1 hunk"
	}
	return fmt.Sprintf("%d / %d hunks", current, total)
}

// getHunkPositionsForFile calculates the line positions of each hunk header in a single file.
// Returns a slice of viewport Y positions where each hunk header starts.
func (m Model) getHunkPositionsForFile(file *DiffFile) []int {
	if file == nil || len(file.Hunks) == 0 {
		return nil
	}

	var positions []int
	lineNum := 0

	for _, hunk := range file.Hunks {
		// Record the position of this hunk header
		positions = append(positions, lineNum)

		// Count all lines in this hunk (including the header line which is part of Lines)
		lineNum += len(hunk.Lines)
	}

	return positions
}

// getHunkPositionsForCommitPreview calculates the line positions of each hunk header
// across all files in the commit preview view.
// This accounts for commit header, file headers, and spacing between files.
func (m Model) getHunkPositionsForCommitPreview() []int {
	if len(m.previewCommitFiles) == 0 {
		return nil
	}

	var positions []int
	lineNum := 0

	// Account for commit header if present
	if len(m.commits) > 0 && m.selectedCommit < len(m.commits) {
		// Commit header typically takes several lines:
		// "commit <hash>", "Author: ...", "Date: ...", "", "    <subject>", ""
		lineNum += 6 // Approximate commit header lines
	}

	for _, file := range m.previewCommitFiles {
		// File header line
		lineNum++ // renderFileHeader output is 1 line

		if file.IsBinary {
			lineNum++ // "Binary file"
		} else if len(file.Hunks) == 0 {
			lineNum++ // "No changes"
		} else {
			// For each hunk in this file
			for _, hunk := range file.Hunks {
				// Record the position of this hunk header
				positions = append(positions, lineNum)

				// Count all lines in this hunk
				lineNum += len(hunk.Lines)
			}
		}

		lineNum++ // Empty line between files
	}

	return positions
}

// getHunkPositionsForDirectory calculates the line positions of each hunk header
// across all files in a directory view.
// This accounts for file headers and spacing between files.
func (m Model) getHunkPositionsForDirectory(dirNode *FileTreeNode) []int {
	if dirNode == nil || !dirNode.IsDir {
		return nil
	}

	files := dirNode.CollectFiles()
	if len(files) == 0 {
		return nil
	}

	var positions []int
	lineNum := 0

	for _, file := range files {
		// File header line
		lineNum++ // renderFileHeader output is 1 line

		if file.IsBinary {
			lineNum++ // "Binary file"
		} else if len(file.Hunks) == 0 {
			lineNum++ // "No changes"
		} else {
			// For each hunk in this file
			for _, hunk := range file.Hunks {
				// Record the position of this hunk header
				positions = append(positions, lineNum)

				// Count all lines in this hunk
				lineNum += len(hunk.Lines)
			}
		}

		lineNum++ // Empty line between files
	}

	return positions
}

// getCurrentHunk returns the hunk currently visible in the viewport.
// Returns nil if no hunk is currently visible (e.g., at file header or empty state).
// For single-file views, uses the active file. For commit preview, uses the preview files.
func (m Model) getCurrentHunk() *DiffHunk {
	isCommitPreview := m.focus == focusCommitPicker && m.commitPaneMode == commitPaneModeList
	currentOffset := (&m).getDiffYOffset()

	if isCommitPreview {
		// Multi-file commit preview mode
		files := m.previewCommitFiles
		if len(files) == 0 {
			return nil
		}

		lineNum := 0

		// Account for commit header if present
		if len(m.commits) > 0 && m.selectedCommit < len(m.commits) {
			lineNum += 6 // Approximate commit header lines
		}

		for _, file := range files {
			// File header line
			lineNum++

			if file.IsBinary {
				lineNum++
			} else if len(file.Hunks) == 0 {
				lineNum++
			} else {
				for i := range file.Hunks {
					hunk := &file.Hunks[i]
					hunkStart := lineNum
					hunkEnd := lineNum + len(hunk.Lines)

					// Check if current offset is within this hunk
					if currentOffset >= hunkStart && currentOffset < hunkEnd {
						return hunk
					}

					lineNum = hunkEnd
				}
			}

			lineNum++ // Empty line between files
		}

		return nil
	}

	// Single-file view
	file := m.getActiveFile()
	if file == nil || len(file.Hunks) == 0 {
		return nil
	}

	lineNum := 0
	for i := range file.Hunks {
		hunk := &file.Hunks[i]
		hunkStart := lineNum
		hunkEnd := lineNum + len(hunk.Lines)

		// Check if current offset is within this hunk
		if currentOffset >= hunkStart && currentOffset < hunkEnd {
			return hunk
		}

		lineNum = hunkEnd
	}

	return nil
}

// formatHunkAsDiff formats a hunk as raw diff text with +/- markers.
// This is the format expected when pasting diffs into other tools.
func formatHunkAsDiff(hunk *DiffHunk) string {
	if hunk == nil || len(hunk.Lines) == 0 {
		return ""
	}

	var sb strings.Builder

	// Write the hunk header
	sb.WriteString(hunk.Header)
	sb.WriteString("\n")

	// Write each line with its prefix
	for _, line := range hunk.Lines {
		// Skip the hunk header line (it's included in Lines but we already wrote it)
		if line.Type == LineHunkHeader {
			continue
		}

		switch line.Type {
		case LineAddition:
			sb.WriteString("+")
		case LineDeletion:
			sb.WriteString("-")
		case LineContext:
			sb.WriteString(" ")
		}
		sb.WriteString(line.Content)
		sb.WriteString("\n")
	}

	return sb.String()
}

// copyCurrentHunk copies the currently visible hunk to the clipboard.
// Returns a HunkCopiedMsg with the result.
func (m Model) copyCurrentHunk() (Model, tea.Cmd) {
	if m.clipboard == nil {
		return m, func() tea.Msg {
			return HunkCopiedMsg{Err: errors.New("clipboard not available")}
		}
	}

	hunk := m.getCurrentHunk()
	if hunk == nil {
		return m, func() tea.Msg {
			return HunkCopiedMsg{Err: errors.New("no hunk at current position")}
		}
	}

	diffText := formatHunkAsDiff(hunk)
	if diffText == "" {
		return m, func() tea.Msg {
			return HunkCopiedMsg{Err: errors.New("hunk is empty")}
		}
	}

	// Count lines
	lineCount := strings.Count(diffText, "\n")

	err := m.clipboard.Copy(diffText)
	return m, func() tea.Msg {
		return HunkCopiedMsg{LineCount: lineCount, Err: err}
	}
}
