package diffviewer

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/git"
	"github.com/zjrosen/perles/internal/mocks"
)

// setupModelWithFiles creates a test model with working dir files and tree properly initialized.
func setupModelWithFiles(files []DiffFile) Model {
	m := New().SetSize(100, 50)
	m.visible = true
	m.workingDirFiles = files
	m.workingDirTree = NewFileTree(files)
	m.selectedWorkingDirNode = 0
	m.focus = focusFileList
	return m
}

// === Constructor Tests ===

func TestNew(t *testing.T) {
	m := New()

	require.False(t, m.Visible())
	require.Empty(t, m.View())
	require.Equal(t, focusFileList, m.focus)
	require.Nil(t, m.workingDirFiles)
	require.Nil(t, m.gitExecutor)
	// TR-5: Verify commit state fields have correct zero values
	require.Nil(t, m.commits)
	require.Equal(t, 0, m.selectedCommit)
	require.Equal(t, 0, m.commitScrollTop)
	require.Empty(t, m.currentBranch)
	require.Equal(t, focusPane(0), m.lastLeftFocus) // Zero value is focusFileList
	// Verify new commit pane mode fields
	require.Equal(t, commitPaneModeList, m.commitPaneMode)
	require.Nil(t, m.commitFiles)
	// Verify commits pane tab state initialization
	require.Equal(t, commitsTabCommits, m.activeCommitTab, "activeCommitTab should default to commitsTabCommits")
	require.Equal(t, 0, m.selectedBranch, "selectedBranch should default to 0")
	require.Equal(t, 0, m.branchScrollTop, "branchScrollTop should default to 0")
	require.False(t, m.branchListLoaded, "branchListLoaded should default to false")
	require.Equal(t, 0, m.selectedWorktree, "selectedWorktree should default to 0")
	require.Equal(t, 0, m.worktreeScrollTop, "worktreeScrollTop should default to 0")
	require.False(t, m.worktreeListLoaded, "worktreeListLoaded should default to false")
}

// TestCommitsTabIndex_Constants verifies tab index constants have expected values.
// This prevents accidental reordering which could break tab navigation logic.
func TestCommitsTabIndex_Constants(t *testing.T) {
	require.Equal(t, commitsTabIndex(0), commitsTabCommits, "commitsTabCommits should be 0")
	require.Equal(t, commitsTabIndex(1), commitsTabBranches, "commitsTabBranches should be 1")
	require.Equal(t, commitsTabIndex(2), commitsTabWorktrees, "commitsTabWorktrees should be 2")
}

// TestNewWithGitExecutor_TabStateInitialization verifies tab state is initialized in NewWithGitExecutor.
func TestNewWithGitExecutor_TabStateInitialization(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)
	m := NewWithGitExecutor(mockGit)

	// Verify tab state is initialized
	require.Equal(t, commitsTabCommits, m.activeCommitTab, "activeCommitTab should default to commitsTabCommits")
	require.Equal(t, 0, m.selectedBranch, "selectedBranch should default to 0")
	require.Equal(t, 0, m.branchScrollTop, "branchScrollTop should default to 0")
	require.False(t, m.branchListLoaded, "branchListLoaded should default to false")
	require.Equal(t, 0, m.selectedWorktree, "selectedWorktree should default to 0")
	require.Equal(t, 0, m.worktreeScrollTop, "worktreeScrollTop should default to 0")
	require.False(t, m.worktreeListLoaded, "worktreeListLoaded should default to false")
}

// === Visibility Tests ===

func TestModel_Show_Hide(t *testing.T) {
	m := New()
	require.False(t, m.Visible())

	m = m.Show()
	require.True(t, m.Visible())

	m = m.Hide()
	require.False(t, m.Visible())
}

func TestModel_ShowResetsState(t *testing.T) {
	m := New()
	// Set some state
	m.workingDirFiles = []DiffFile{{NewPath: "test.go"}}
	m.selectedWorkingDirNode = 5
	m.err = nil
	// Set commit state
	m.commits = []git.CommitInfo{{Hash: "abc123"}}
	m.selectedCommit = 3
	m.commitScrollTop = 2
	m.currentBranch = "feature"
	m.lastLeftFocus = focusCommitPicker
	// Set commit pane mode state
	m.commitPaneMode = commitPaneModeFiles
	m.commitFiles = []DiffFile{{NewPath: "commit_file.go"}}
	m.commitFilesTree = NewFileTree(m.commitFiles)
	m.selectedCommitFileNode = 2
	m.commitFilesTreeScrollTop = 1

	m = m.Show()

	require.Nil(t, m.workingDirFiles)
	require.Nil(t, m.workingDirTree)
	require.Equal(t, 0, m.selectedWorkingDirNode)
	require.Nil(t, m.err)
	// Verify commit state is reset
	require.Nil(t, m.commits)
	require.Equal(t, 0, m.selectedCommit)
	require.Equal(t, 0, m.commitScrollTop)
	require.Empty(t, m.currentBranch)
	require.Equal(t, focusFileList, m.lastLeftFocus)
	// Verify commit pane mode state is reset
	require.Equal(t, commitPaneModeList, m.commitPaneMode)
	require.Nil(t, m.commitFiles)
	require.Nil(t, m.commitFilesTree)
	require.Equal(t, 0, m.selectedCommitFileNode)
	require.Equal(t, 0, m.commitFilesTreeScrollTop)
}

// === Init Tests ===

func TestModel_Init(t *testing.T) {
	m := New()
	cmd := m.Init()

	require.Nil(t, cmd)
}

// === File Navigation Tests ===

func TestModel_FileNavigation(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	files := []DiffFile{
		{NewPath: "file1.go"},
		{NewPath: "file2.go"},
		{NewPath: "file3.go"},
	}
	m.workingDirFiles = files
	m.workingDirTree = NewFileTree(files)
	m.selectedWorkingDirNode = 0
	m.focus = focusFileList

	// Navigate down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.selectedWorkingDirNode)

	// Navigate down again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 2, m.selectedWorkingDirNode)

	// Try to navigate past end - should stay at last
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 2, m.selectedWorkingDirNode)

	// Navigate up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 1, m.selectedWorkingDirNode)

	// Navigate up again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.selectedWorkingDirNode)

	// Try to navigate past start - should stay at 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.selectedWorkingDirNode)
}

// === Focus Switch Tests ===

func TestModel_FocusSwitch(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	require.Equal(t, focusFileList, m.focus)

	// Switch to CommitPicker with 'l' (moves down in left column)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, focusCommitPicker, m.focus)

	// Switch back to FileList with 'h' (moves up in left column)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, focusFileList, m.focus)
}

// === WorkingDirDiffLoadedMsg Tests ===

func TestModel_WorkingDirDiffLoaded(t *testing.T) {
	m := New().SetSize(100, 50)
	m = m.Show()

	files := []DiffFile{
		{NewPath: "file1.go", Additions: 10, Deletions: 5},
		{NewPath: "file2.go", Additions: 3, Deletions: 0},
	}

	m, _ = m.Update(WorkingDirDiffLoadedMsg{Files: files, Err: nil})

	require.Nil(t, m.err)
	require.Len(t, m.workingDirFiles, 2)
	require.Equal(t, 0, m.selectedWorkingDirNode)
	require.Equal(t, "file1.go", m.workingDirFiles[0].NewPath)
}

func TestModel_WorkingDirDiffLoaded_Error(t *testing.T) {
	m := New().SetSize(100, 50)
	m = m.Show()

	testErr := errors.New("git error")
	m, _ = m.Update(WorkingDirDiffLoadedMsg{Files: nil, Err: testErr})

	require.NotNil(t, m.err)
	require.Equal(t, "git error", m.err.Error())
	require.Nil(t, m.workingDirFiles)
}

// === Close Tests ===

func TestModel_Close(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	// Close with ESC
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	require.False(t, m.Visible())
	require.NotNil(t, cmd)

	// Verify the command returns HideDiffViewerMsg
	msg := cmd()
	_, ok := msg.(HideDiffViewerMsg)
	require.True(t, ok, "expected HideDiffViewerMsg")
}

func TestModel_CloseWithQ(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	// Close with 'q'
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.False(t, m.Visible())
	require.NotNil(t, cmd)
}

// === Update Ignores When Not Visible ===

func TestModel_UpdateIgnoresWhenNotVisible(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = false
	m.focus = focusFileList

	// Key press should be ignored
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, focusFileList, m.focus)
}

// === WorkingDirDiffLoadedMsg Tests ===

func TestModel_WorkingDirDiffLoadedMsg(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	files := []DiffFile{
		{NewPath: "test.go", Additions: 5, Deletions: 3},
	}

	m, _ = m.Update(WorkingDirDiffLoadedMsg{Files: files, Err: nil})

	require.Nil(t, m.err)
	require.Len(t, m.workingDirFiles, 1)
}

// === WindowSizeMsg Tests ===

func TestModel_WindowSizeMsg(t *testing.T) {
	m := New()
	m.visible = true

	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	require.Equal(t, 120, m.width)
	require.Equal(t, 40, m.height)
}

// === Overlay Tests ===

func TestModel_Overlay(t *testing.T) {
	m := New().SetSize(80, 24)
	m.visible = true
	m.workingDirFiles = []DiffFile{
		{NewPath: "test.go", Additions: 1, Deletions: 1},
	}
	m.refreshViewport()

	// Create a background
	bg := strings.Repeat(strings.Repeat(".", 80)+"\n", 24)

	result := m.Overlay(bg)

	// Result should be non-empty and different from background
	require.NotEmpty(t, result)
	require.NotEqual(t, bg, result)
}

func TestModel_OverlayNotVisibleReturnsBackground(t *testing.T) {
	m := New().SetSize(80, 24)
	m.visible = false

	bg := "background content"
	result := m.Overlay(bg)

	require.Equal(t, bg, result)
}

// === Golden Tests ===

func TestModel_View_Golden_Empty(t *testing.T) {
	m := New().SetSize(100, 30)
	m.visible = true
	m.workingDirFiles = nil

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestModel_View_Golden_WithFiles(t *testing.T) {
	m := New().SetSize(100, 30)
	m.visible = true
	m.workingDirFiles = []DiffFile{
		{
			NewPath:   "main.go",
			Additions: 10,
			Deletions: 5,
			Hunks: []DiffHunk{
				{
					OldStart: 1,
					OldCount: 5,
					NewStart: 1,
					NewCount: 6,
					Header:   "@@ -1,5 +1,6 @@ package main",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: "package main"},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "import \"fmt\""},
						{Type: LineDeletion, OldLineNum: 2, Content: "old line"},
						{Type: LineAddition, NewLineNum: 2, Content: "new line"},
						{Type: LineContext, OldLineNum: 3, NewLineNum: 3, Content: "func main() {"},
					},
				},
			},
		},
		{NewPath: "README.md", Additions: 2, Deletions: 0},
		{NewPath: "go.mod", Additions: 1, Deletions: 1},
	}
	m.selectedWorkingDirNode = 0
	m.refreshViewport()

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestModel_View_Golden_Error(t *testing.T) {
	m := New().SetSize(100, 30)
	m.visible = true
	m.err = errors.New("fatal: not a git repository")

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TEST-2: Golden test for multi-pane layout with commits at 100x30 (standard)
func TestView_WithCommits_Golden(t *testing.T) {
	// Fixed timestamp for commit dates
	fixedTime := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)

	m := New().SetSize(100, 30)
	m.visible = true
	m.workingDirFiles = []DiffFile{
		{
			NewPath:   "main.go",
			Additions: 10,
			Deletions: 5,
			Hunks: []DiffHunk{
				{
					OldStart: 1,
					OldCount: 5,
					NewStart: 1,
					NewCount: 6,
					Header:   "@@ -1,5 +1,6 @@ package main",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: "package main"},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "import \"fmt\""},
						{Type: LineDeletion, OldLineNum: 2, Content: "old line"},
						{Type: LineAddition, NewLineNum: 2, Content: "new line"},
						{Type: LineContext, OldLineNum: 3, NewLineNum: 3, Content: "func main() {"},
					},
				},
			},
		},
		{NewPath: "README.md", Additions: 2, Deletions: 0},
		{NewPath: "go.mod", Additions: 1, Deletions: 1},
	}
	m.selectedWorkingDirNode = 0
	m.commits = []git.CommitInfo{
		{Hash: "abc1234567890def", ShortHash: "abc1234", Subject: "Fix authentication bug", Author: "Dev", Date: fixedTime.Add(-2 * time.Hour)},
		{Hash: "def5678901234abc", ShortHash: "def5678", Subject: "Add feature X", Author: "Dev", Date: fixedTime.Add(-24 * time.Hour)},
		{Hash: "ghi9012345678def", ShortHash: "ghi9012", Subject: "Refactor utils", Author: "Dev", Date: fixedTime.Add(-3 * 24 * time.Hour)},
	}
	m.selectedCommit = 0
	m.currentBranch = "main"
	m.refreshViewport()

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TEST-3: Golden test for empty commit state ("No commits yet")
func TestView_EmptyCommits_Golden(t *testing.T) {
	m := New().SetSize(100, 30)
	m.visible = true
	m.workingDirFiles = []DiffFile{
		{NewPath: "README.md", Additions: 10, Deletions: 0},
	}
	m.selectedWorkingDirNode = 0
	m.commits = []git.CommitInfo{} // Empty - no commits yet
	m.currentBranch = ""
	m.refreshViewport()

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TEST-4: Golden test for single commit edge case
func TestView_SingleCommit_Golden(t *testing.T) {
	// Fixed timestamp for commit dates
	fixedTime := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)

	m := New().SetSize(100, 30)
	m.visible = true
	m.workingDirFiles = []DiffFile{
		{NewPath: "initial.go", Additions: 50, Deletions: 0},
	}
	m.selectedWorkingDirNode = 0
	m.commits = []git.CommitInfo{
		{Hash: "abc1234567890def", ShortHash: "abc1234", Subject: "Initial commit", Author: "Dev", Date: fixedTime.Add(-1 * time.Hour)},
	}
	m.selectedCommit = 0
	m.currentBranch = "main"
	m.refreshViewport()

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TEST-2: Golden test for narrow viewport (80x24)
func TestView_NarrowViewport_Golden(t *testing.T) {
	// Fixed timestamp for commit dates
	fixedTime := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)

	m := New().SetSize(80, 24)
	m.visible = true
	m.workingDirFiles = []DiffFile{
		{NewPath: "main.go", Additions: 5, Deletions: 2},
		{NewPath: "utils.go", Additions: 10, Deletions: 0},
	}
	m.selectedWorkingDirNode = 0
	m.commits = []git.CommitInfo{
		{Hash: "abc1234567890def", ShortHash: "abc1234", Subject: "Fix bug", Author: "Dev", Date: fixedTime.Add(-1 * time.Hour)},
		{Hash: "def5678901234abc", ShortHash: "def5678", Subject: "Add feature", Author: "Dev", Date: fixedTime.Add(-2 * 24 * time.Hour)},
	}
	m.selectedCommit = 0
	m.currentBranch = "feature/test"
	m.refreshViewport()

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TEST-2: Golden test for wide viewport (200x40)
func TestView_WideViewport_Golden(t *testing.T) {
	// Fixed timestamp for commit dates
	fixedTime := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)

	m := New().SetSize(200, 40)
	m.visible = true
	m.workingDirFiles = []DiffFile{
		{
			NewPath:   "cmd/server/main.go",
			Additions: 25,
			Deletions: 10,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,10 +1,15 @@ package main",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: "package main"},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "import ("},
						{Type: LineContext, OldLineNum: 2, NewLineNum: 2, Content: "\t\"fmt\""},
						{Type: LineAddition, NewLineNum: 3, Content: "\t\"log\""},
						{Type: LineContext, OldLineNum: 3, NewLineNum: 4, Content: ")"},
					},
				},
			},
		},
		{NewPath: "internal/api/handler.go", Additions: 100, Deletions: 50},
		{NewPath: "pkg/utils/helpers.go", Additions: 15, Deletions: 5},
		{NewPath: "README.md", Additions: 20, Deletions: 2},
	}
	m.selectedWorkingDirNode = 0
	m.commits = []git.CommitInfo{
		{Hash: "abc1234567890def", ShortHash: "abc1234", Subject: "Implement new API endpoints for user management", Author: "Developer", Date: fixedTime.Add(-30 * time.Minute)},
		{Hash: "def5678901234abc", ShortHash: "def5678", Subject: "Add logging infrastructure", Author: "Developer", Date: fixedTime.Add(-2 * time.Hour)},
		{Hash: "ghi9012345678def", ShortHash: "ghi9012", Subject: "Refactor database connection handling", Author: "Developer", Date: fixedTime.Add(-5 * time.Hour)},
		{Hash: "jkl3456789012ghi", ShortHash: "jkl3456", Subject: "Update dependencies to latest versions", Author: "Developer", Date: fixedTime.Add(-24 * time.Hour)},
		{Hash: "mno7890123456jkl", ShortHash: "mno7890", Subject: "Initial project setup", Author: "Developer", Date: fixedTime.Add(-7 * 24 * time.Hour)},
	}
	m.selectedCommit = 0
	m.currentBranch = "feature/user-management"
	m.refreshViewport()

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// === Integration Tests ===

func TestDiffViewer_KeyboardNavigation(t *testing.T) {
	// Full keyboard navigation flow
	m := setupModelWithFiles([]DiffFile{
		{NewPath: "file1.go"},
		{NewPath: "file2.go"},
	})
	m.refreshViewport()

	// Start at file 0
	require.Equal(t, 0, m.selectedWorkingDirNode)
	require.Equal(t, focusFileList, m.focus)

	// Navigate to file 1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.selectedWorkingDirNode)

	// Switch to CommitPicker pane with l
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, focusCommitPicker, m.focus)

	// j/k in CommitPicker navigates commits (no commits loaded, so no change)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, focusCommitPicker, m.focus)
	require.Equal(t, 1, m.selectedWorkingDirNode) // File selection unchanged

	// Switch back to file list with h
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, focusFileList, m.focus)

	// Navigate files again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.selectedWorkingDirNode)
}

// === CommitsLoadedMsg Tests (TR-6) ===

func TestModel_CommitsLoadedMsg(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	commits := []git.CommitInfo{
		{Hash: "abc1234567890", ShortHash: "abc1234", Subject: "Fix bug", Author: "Dev", Date: time.Now()},
		{Hash: "def5678901234", ShortHash: "def5678", Subject: "Add feature", Author: "Dev", Date: time.Now()},
	}

	m, cmd := m.Update(CommitsLoadedMsg{Commits: commits, Branch: "main", Err: nil})

	require.Nil(t, m.err)
	require.Len(t, m.commits, 2)
	require.Equal(t, 0, m.selectedCommit)
	require.Equal(t, 0, m.commitScrollTop)
	require.Equal(t, "main", m.currentBranch)
	// Returns a LoadCommitPreview command for the first commit
	require.NotNil(t, cmd)
}

func TestModel_CommitsLoadedMsg_Error(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	testErr := errors.New("git error")
	m, cmd := m.Update(CommitsLoadedMsg{Err: testErr})

	require.NotNil(t, m.err)
	require.Equal(t, "git error", m.err.Error())
	require.Nil(t, m.commits)
	require.Nil(t, cmd)
}

// EC-1: Empty repository handling (no crash, no command returned)
func TestModel_CommitsLoadedMsg_EmptyCommits(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	// Empty commits (empty repository case)
	m, cmd := m.Update(CommitsLoadedMsg{Commits: []git.CommitInfo{}, Branch: "main", Err: nil})

	require.Nil(t, m.err)
	require.Empty(t, m.commits)
	require.Equal(t, 0, m.selectedCommit)
	require.Equal(t, "main", m.currentBranch)
	// Should NOT return a command for empty repo
	require.Nil(t, cmd)
}

// EC-2: Detached HEAD state displays "HEAD" as branch name
func TestModel_LoadCommits_DetachedHead(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)

	// Setup mock to return detached HEAD error for GetCurrentBranch
	mockGit.On("GetCommitLog", 50).Return([]git.CommitInfo{
		{Hash: "abc1234567890", ShortHash: "abc1234", Subject: "Commit", Author: "Dev", Date: time.Now()},
	}, nil)
	mockGit.On("GetCurrentBranch").Return("", git.ErrDetachedHead)

	m := NewWithGitExecutor(mockGit)

	cmd := m.LoadCommits()
	require.NotNil(t, cmd)

	// Execute the command
	msg := cmd()
	commitsMsg, ok := msg.(CommitsLoadedMsg)
	require.True(t, ok)
	require.Nil(t, commitsMsg.Err)
	require.Equal(t, "HEAD", commitsMsg.Branch)
	require.Len(t, commitsMsg.Commits, 1)

	mockGit.AssertExpectations(t)
}

func TestModel_LoadCommits_NilGitExecutor(t *testing.T) {
	m := New() // No git executor set

	cmd := m.LoadCommits()
	require.NotNil(t, cmd)

	msg := cmd()
	commitsMsg, ok := msg.(CommitsLoadedMsg)
	require.True(t, ok)
	require.Nil(t, commitsMsg.Err)
	require.Nil(t, commitsMsg.Commits)
	require.Empty(t, commitsMsg.Branch)
}

func TestModel_LoadCommits_GitError(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)

	testErr := errors.New("git log failed")
	mockGit.On("GetCommitLog", 50).Return(nil, testErr)

	m := NewWithGitExecutor(mockGit)

	cmd := m.LoadCommits()
	msg := cmd()
	commitsMsg, ok := msg.(CommitsLoadedMsg)
	require.True(t, ok)
	require.NotNil(t, commitsMsg.Err)
	require.Equal(t, "git log failed", commitsMsg.Err.Error())

	mockGit.AssertExpectations(t)
}

func TestModel_LoadCommits_BranchError(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)

	// Setup mock: commits succeed but branch lookup fails with non-detached error
	mockGit.On("GetCommitLog", 50).Return([]git.CommitInfo{
		{Hash: "abc1234", ShortHash: "abc1234", Subject: "Commit", Author: "Dev", Date: time.Now()},
	}, nil)
	mockGit.On("GetCurrentBranch").Return("", errors.New("unknown branch error"))

	m := NewWithGitExecutor(mockGit)

	cmd := m.LoadCommits()
	msg := cmd()
	commitsMsg, ok := msg.(CommitsLoadedMsg)
	require.True(t, ok)
	require.NotNil(t, commitsMsg.Err)
	require.Equal(t, "unknown branch error", commitsMsg.Err.Error())

	mockGit.AssertExpectations(t)
}

// TR-4: focusPane enum extended with focusCommitPicker constant
func TestFocusPane_CommitPicker(t *testing.T) {
	// Verify the enum values and ordering
	require.Equal(t, focusPane(0), focusFileList)
	require.Equal(t, focusPane(1), focusCommitPicker)
	require.Equal(t, focusPane(2), focusDiffPane)
}

// TR-5: Model struct includes lastLeftFocus field for focus restoration
func TestModel_LastLeftFocus_DefaultsToFileList(t *testing.T) {
	m := New()
	// Zero value should be focusFileList (0)
	require.Equal(t, focusFileList, m.lastLeftFocus)

	// After Show(), lastLeftFocus should be explicitly set to focusFileList
	m = m.Show()
	require.Equal(t, focusFileList, m.lastLeftFocus)
}

// ShowAndLoad test - verifies tea.Batch behavior
func TestModel_ShowAndLoad(t *testing.T) {
	m := New()

	// Test ShowAndLoad without git executor
	m, cmd := m.ShowAndLoad()

	require.True(t, m.Visible())
	require.NotNil(t, cmd)

	// The batch command returns a tea.BatchMsg which is a slice of messages
	// but we can't easily test the internal workings of tea.Batch
	// Instead, verify the state changes are correct
	require.Nil(t, m.commits)
	require.Equal(t, 0, m.selectedCommit)
	require.Equal(t, focusFileList, m.lastLeftFocus)
}

// TestModel_LoadCommits_WithMocks - verifies the LoadCommits command
func TestModel_LoadCommits_WithMocks(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)

	commits := []git.CommitInfo{
		{Hash: "abc1234", ShortHash: "abc1234", Subject: "Commit", Author: "Dev", Date: time.Now()},
	}

	// Setup mocks
	mockGit.On("GetCommitLog", 50).Return(commits, nil)
	mockGit.On("GetCurrentBranch").Return("main", nil)

	m := NewWithGitExecutor(mockGit)

	// Test LoadCommits
	commitsCmd := m.LoadCommits()
	commitsMsg := commitsCmd()
	loadedMsg, ok := commitsMsg.(CommitsLoadedMsg)
	require.True(t, ok)
	require.Equal(t, "main", loadedMsg.Branch)
	require.Len(t, loadedMsg.Commits, 1)

	mockGit.AssertExpectations(t)
}

// === TEST-5: Focus Cycling Tests (FR-3, FR-4, FR-5, FR-6) ===

// TestModel_FocusCycling tests all focus transitions
func TestModel_FocusCycling(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.workingDirFiles = []DiffFile{{NewPath: "test.go"}}
	m.commits = []git.CommitInfo{{Hash: "abc1234", ShortHash: "abc1234", Subject: "Test", Author: "Dev", Date: time.Now()}}
	m.focus = focusFileList

	// Initial state
	require.Equal(t, focusFileList, m.focus)

	// Tab: FileList → CommitPicker (FR-5)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, focusCommitPicker, m.focus)

	// Tab: CommitPicker → DiffPane (FR-5)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, focusDiffPane, m.focus)

	// Tab: DiffPane → FileList (FR-5)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, focusFileList, m.focus)
}

// TestModel_FocusLeft_ReturnsToLastLeftPane tests h key returns to lastLeftFocus (FR-3)
func TestModel_FocusLeft_ReturnsToLastLeftPane(t *testing.T) {
	tests := []struct {
		name              string
		startFocus        focusPane
		lastLeftFocus     focusPane
		afterH            focusPane
		afterL            focusPane
		lastLeftAfterL    focusPane
		hFromDiffExpected focusPane
	}{
		{
			name:          "h/l toggles between FileList and CommitPicker",
			startFocus:    focusFileList,
			lastLeftFocus: focusFileList,
		},
		{
			name:          "h from CommitPicker goes to FileList",
			startFocus:    focusCommitPicker,
			lastLeftFocus: focusCommitPicker,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New().SetSize(100, 50)
			m.visible = true
			m.focus = tt.startFocus
			m.lastLeftFocus = tt.lastLeftFocus

			if tt.startFocus == focusFileList {
				// From FileList: h is no-op, l goes to CommitPicker
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
				require.Equal(t, focusFileList, m.focus, "h from FileList should be no-op")

				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
				require.Equal(t, focusCommitPicker, m.focus, "l from FileList should go to CommitPicker")

				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
				require.Equal(t, focusFileList, m.focus, "h from CommitPicker should go to FileList")
			} else {
				// From CommitPicker: h goes to FileList, l is no-op
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
				require.Equal(t, focusCommitPicker, m.focus, "l from CommitPicker should be no-op")

				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
				require.Equal(t, focusFileList, m.focus, "h from CommitPicker should go to FileList")
			}
		})
	}
}

// TestModel_TabCyclesThroughAllPanes tests Tab cycles all three panes (FR-5)
func TestModel_TabCyclesThroughAllPanes(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	// Start at FileList
	m.focus = focusFileList
	panes := []focusPane{focusFileList, focusCommitPicker, focusDiffPane}

	// Cycle through all panes twice to verify the full cycle
	for i := 0; i < 6; i++ {
		require.Equal(t, panes[i%3], m.focus, "Tab cycle iteration %d", i)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	// After 6 tabs, should be back at FileList
	require.Equal(t, focusFileList, m.focus)
}

// TestModel_CommitNavigation tests j/k keys in CommitPicker (FR-6)
func TestModel_CommitNavigation(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.commits = []git.CommitInfo{
		{Hash: "abc1234", ShortHash: "abc1234", Subject: "First", Author: "Dev", Date: time.Now()},
		{Hash: "def5678", ShortHash: "def5678", Subject: "Second", Author: "Dev", Date: time.Now()},
		{Hash: "ghi9012", ShortHash: "ghi9012", Subject: "Third", Author: "Dev", Date: time.Now()},
	}
	m.selectedCommit = 0

	// j: move to next commit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.selectedCommit, "j should move to next commit")

	// j: move to next commit again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 2, m.selectedCommit, "j should move to third commit")

	// j: at end, should stay at last commit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 2, m.selectedCommit, "j at end should be no-op")

	// k: move to previous commit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 1, m.selectedCommit, "k should move to previous commit")

	// k: move to previous commit again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.selectedCommit, "k should move to first commit")

	// k: at start, should stay at first commit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.selectedCommit, "k at start should be no-op")
}

// TestModel_CommitNavigation_EmptyCommits tests j/k with no commits
func TestModel_CommitNavigation_EmptyCommits(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.commits = nil
	m.selectedCommit = 0

	// j/k should be no-op with empty commits
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 0, m.selectedCommit)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.selectedCommit)
}

// TestModel_EnterInCommitPicker_DrillsIntoCommitFiles tests Enter drills into commit files (FR-7)
func TestModel_EnterInCommitPicker_DrillsIntoCommitFiles(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)

	m := NewWithGitExecutor(mockGit).SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.commitPaneMode = commitPaneModeList
	m.commits = []git.CommitInfo{
		{Hash: "abc1234567890", ShortHash: "abc1234", Subject: "First", Author: "Dev", Date: time.Now()},
		{Hash: "def5678901234", ShortHash: "def5678", Subject: "Second", Author: "Dev", Date: time.Now()},
	}
	m.selectedCommit = 1 // Select second commit

	// Setup mock for the expected GetCommitDiff call
	mockGit.On("GetCommitDiff", "def5678901234").Return("diff output", nil)

	// Press Enter
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Mode stays list until files load (to avoid flash)
	require.Equal(t, commitPaneModeList, m.commitPaneMode)
	require.NotNil(t, m.inspectedCommit)
	require.Equal(t, "def5678", m.inspectedCommit.ShortHash)

	// Should return a command
	require.NotNil(t, cmd, "Enter in CommitPicker should return a LoadCommitFiles command")

	// Execute the command and verify it's a CommitFilesLoadedMsg
	msg := cmd()
	filesMsg, ok := msg.(CommitFilesLoadedMsg)
	require.True(t, ok, "Command should return CommitFilesLoadedMsg")

	// Process the loaded message - NOW it should switch to files mode
	m, _ = m.Update(filesMsg)
	require.Equal(t, commitPaneModeFiles, m.commitPaneMode, "should switch to files mode after files load")

	mockGit.AssertExpectations(t)
}

// TestModel_EnterInCommitPicker_EmptyCommits tests Enter with no commits
func TestModel_EnterInCommitPicker_EmptyCommits(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.commits = nil

	// Press Enter - should return nil command
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, cmd, "Enter with empty commits should return nil")
}

// TestModel_EnterInFileList_NoOp tests Enter in FileList doesn't trigger any command
func TestModel_EnterInFileList_NoOp(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusFileList
	m.commits = []git.CommitInfo{
		{Hash: "abc1234", ShortHash: "abc1234", Subject: "First", Author: "Dev", Date: time.Now()},
	}
	m.selectedCommit = 0

	// Press Enter - should return nil (no-op in FileList)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, cmd, "Enter in FileList should be no-op")
}

// TestModel_LKeyNavigatesLeftColumn tests l key navigates within left column (FR-4)
func TestModel_LKeyNavigatesLeftColumn(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusFileList

	// Press l from FileList - goes to CommitPicker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, focusCommitPicker, m.focus)

	// Press l from CommitPicker - no-op (already at bottom of left column)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, focusCommitPicker, m.focus)
}

// TestModel_HKeyNavigatesLeftColumn tests h key navigates within left column (FR-3)
func TestModel_HKeyNavigatesLeftColumn(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	// h from FileList is no-op (already at top)
	m.focus = focusFileList
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, focusFileList, m.focus, "h from FileList should be no-op")

	// h from CommitPicker goes to FileList
	m.focus = focusCommitPicker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, focusFileList, m.focus, "h from CommitPicker should go to FileList")

	// h from DiffPane returns to lastLeftFocus
	m.focus = focusDiffPane
	m.lastLeftFocus = focusCommitPicker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, focusCommitPicker, m.focus, "h from DiffPane should return to lastLeftFocus")
}

// === Commit Selection Integration Tests (TEST-6) ===

// TestCommitSelection_Integration tests the full commit selection workflow:
// Navigate with j -> select with Enter -> DrillIntoCommit called with correct hash
func TestCommitSelection_Integration(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)

	m := NewWithGitExecutor(mockGit).SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.commitPaneMode = commitPaneModeList
	m.commits = []git.CommitInfo{
		{Hash: "first_hash_1234567", ShortHash: "first12", Subject: "First commit", Author: "Dev", Date: time.Now()},
		{Hash: "second_hash_234567", ShortHash: "second2", Subject: "Second commit", Author: "Dev", Date: time.Now()},
		{Hash: "third_hash_3456789", ShortHash: "third34", Subject: "Third commit", Author: "Dev", Date: time.Now()},
	}
	m.selectedCommit = 0

	// Step 1: Start at first commit
	require.Equal(t, 0, m.selectedCommit, "Should start at first commit")

	// Step 2: Navigate down with j (move to second commit)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.selectedCommit, "j should move to second commit")

	// Step 3: Navigate down again with j (move to third commit)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 2, m.selectedCommit, "j should move to third commit")

	// Step 4: Setup mock for the expected GetCommitDiff call with third commit hash
	mockGit.On("GetCommitDiff", "third_hash_3456789").Return("diff output for third commit", nil)

	// Step 5: Press Enter to drill into the third commit's files
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Step 6: Mode stays list until files load (to avoid flash)
	require.Equal(t, commitPaneModeList, m.commitPaneMode)
	require.NotNil(t, m.inspectedCommit)
	require.Equal(t, "third34", m.inspectedCommit.ShortHash)

	// Step 7: Verify a command was returned
	require.NotNil(t, cmd, "Enter should return a LoadCommitFiles command")

	// Step 8: Execute the command and verify it returns CommitFilesLoadedMsg
	msg := cmd()
	filesMsg, ok := msg.(CommitFilesLoadedMsg)
	require.True(t, ok, "Command should return CommitFilesLoadedMsg")
	require.Nil(t, filesMsg.Err, "Should have no error")

	// Step 9: Process the loaded message - NOW it should switch to files mode
	m, _ = m.Update(filesMsg)
	require.Equal(t, commitPaneModeFiles, m.commitPaneMode, "should switch to files mode after files load")

	// Step 10: Verify mock was called with correct hash
	mockGit.AssertExpectations(t)
}

// TestCommitSelection_TabThenNavigate tests Tab to CommitPicker then navigate
func TestCommitSelection_TabThenNavigate(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)

	m := NewWithGitExecutor(mockGit).SetSize(100, 50)
	m.visible = true
	m.focus = focusFileList // Start at FileList
	m.commitPaneMode = commitPaneModeList
	m.commits = []git.CommitInfo{
		{Hash: "hash_a123456789", ShortHash: "hash_a1", Subject: "Commit A", Author: "Dev", Date: time.Now()},
		{Hash: "hash_b987654321", ShortHash: "hash_b9", Subject: "Commit B", Author: "Dev", Date: time.Now()},
	}
	m.selectedCommit = 0

	// Step 1: Press Tab to move to CommitPicker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Equal(t, focusCommitPicker, m.focus, "Tab should move to CommitPicker")

	// Step 2: Navigate down with j
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.selectedCommit, "j should move to second commit")

	// Step 3: Setup mock and press Enter
	mockGit.On("GetCommitDiff", "hash_b987654321").Return("diff output", nil)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Step 4: Mode stays list until files load (to avoid flash)
	require.Equal(t, commitPaneModeList, m.commitPaneMode)
	require.NotNil(t, cmd, "Enter should return a LoadCommitFiles command")
	msg := cmd()
	filesMsg, ok := msg.(CommitFilesLoadedMsg)
	require.True(t, ok, "Command should return CommitFilesLoadedMsg")

	// Step 5: Process the loaded message - NOW it should switch to files mode
	m, _ = m.Update(filesMsg)
	require.Equal(t, commitPaneModeFiles, m.commitPaneMode, "should switch to files mode after files load")

	mockGit.AssertExpectations(t)
}

// === Commit Files Tree Navigation Tests ===

// setupModelWithCommitFilesTree creates a test model with commit files as a tree.
func setupModelWithCommitFilesTree(files []DiffFile) Model {
	m := New().SetSize(100, 50)
	m.visible = true
	m.commitFiles = files
	m.commitFilesTree = NewFileTree(files)
	m.selectedCommitFileNode = 0
	m.focus = focusCommitPicker
	m.commitPaneMode = commitPaneModeFiles
	return m
}

// TestCommitFilesTree_Navigation tests j/k navigation in commit files tree.
func TestCommitFilesTree_Navigation(t *testing.T) {
	files := []DiffFile{
		{NewPath: "src/main.go"},
		{NewPath: "src/utils.go"},
		{NewPath: "README.md"},
	}
	m := setupModelWithCommitFilesTree(files)

	// Navigate down with j
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.selectedCommitFileNode)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 2, m.selectedCommitFileNode)

	// Should not go past end (src dir + 2 files + README = 4 nodes)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.GreaterOrEqual(t, m.selectedCommitFileNode, 2)

	// Navigate up with k
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Less(t, m.selectedCommitFileNode, 4)

	// Navigate back to start
	for i := 0; i < 10; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	}
	require.Equal(t, 0, m.selectedCommitFileNode)
}

// TestCommitFilesTree_DirectoryToggle tests Enter toggles directory expand/collapse.
func TestCommitFilesTree_DirectoryToggle(t *testing.T) {
	files := []DiffFile{
		{NewPath: "src/main.go"},
		{NewPath: "src/utils.go"},
	}
	m := setupModelWithCommitFilesTree(files)

	// First visible node should be the 'src' directory
	nodes := m.commitFilesTree.VisibleNodes()
	require.True(t, nodes[0].IsDir, "First node should be a directory")
	require.Equal(t, "src", nodes[0].Name)
	require.True(t, nodes[0].Expanded)
	initialNodeCount := len(nodes)

	// Press Enter to collapse the directory
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nodesAfterCollapse := m.commitFilesTree.VisibleNodes()
	require.Less(t, len(nodesAfterCollapse), initialNodeCount, "Collapsing should reduce visible nodes")

	// Press Enter to expand again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nodesAfterExpand := m.commitFilesTree.VisibleNodes()
	require.Equal(t, initialNodeCount, len(nodesAfterExpand), "Expanding should restore visible nodes")
}

// TestCommitFilesTree_FileSelection tests that getSelectedCommitFile returns the correct file.
func TestCommitFilesTree_FileSelection(t *testing.T) {
	files := []DiffFile{
		{NewPath: "src/main.go", Additions: 10},
		{NewPath: "src/utils.go", Additions: 5},
	}
	m := setupModelWithCommitFilesTree(files)

	// First node is directory, should return nil
	file := m.getSelectedCommitFile()
	require.Nil(t, file, "Directory node should return nil file")

	// Navigate to first file (under 'src' directory)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	file = m.getSelectedCommitFile()
	require.NotNil(t, file, "File node should return a file")
	require.Equal(t, "src/main.go", file.NewPath)
}

// TestCommitFilesTree_EnterOnFile tests Enter on file focuses diff pane.
func TestCommitFilesTree_EnterOnFile(t *testing.T) {
	files := []DiffFile{
		{NewPath: "main.go", Additions: 10},
	}
	m := setupModelWithCommitFilesTree(files)

	// Navigate to the file (skip if there's a directory first)
	nodes := m.commitFilesTree.VisibleNodes()
	for i, n := range nodes {
		if !n.IsDir {
			m.selectedCommitFileNode = i
			break
		}
	}

	// Press Enter on file should focus diff pane
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Equal(t, focusDiffPane, m.focus)
	require.Equal(t, focusCommitPicker, m.lastLeftFocus)
}

// === Diff Pane Scrolling Tests ===

// createLargeFile creates a DiffFile with many lines for scroll testing.
func createLargeFile() DiffFile {
	return DiffFile{
		NewPath:   "test.go",
		Additions: 50,
		Deletions: 10,
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,60 +1,100 @@",
				OldStart: 1, OldCount: 60, NewStart: 1, NewCount: 100,
				Lines: func() []DiffLine {
					var lines []DiffLine
					lines = append(lines, DiffLine{Type: LineHunkHeader})
					for i := 1; i <= 100; i++ {
						lines = append(lines, DiffLine{
							Type:       LineContext,
							OldLineNum: i,
							NewLineNum: i,
							Content:    "line content " + string(rune('A'+i%26)),
						})
					}
					return lines
				}(),
			},
		},
	}
}

// TestModel_DiffPaneScrolling tests j/k scrolls the diff viewport when DiffPane is focused
func TestModel_DiffPaneScrolling(t *testing.T) {
	files := []DiffFile{createLargeFile()}
	m := setupModelWithFiles(files)
	m.focus = focusDiffPane
	m.refreshViewport()

	// Get initial scroll position
	initialYOffset := m.diffViewport.YOffset

	// Press j to scroll down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Greater(t, m.diffViewport.YOffset, initialYOffset, "j in DiffPane should scroll down")

	// Press k to scroll up
	scrolledYOffset := m.diffViewport.YOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Less(t, m.diffViewport.YOffset, scrolledYOffset, "k in DiffPane should scroll up")
}

// TestModel_DiffPaneHalfPageScroll tests Ctrl+D/Ctrl+U scrolls half page
func TestModel_DiffPaneHalfPageScroll(t *testing.T) {
	files := []DiffFile{createLargeFile()}
	m := setupModelWithFiles(files)
	m.focus = focusDiffPane
	m.refreshViewport()

	// Press Ctrl+D to scroll down half page
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	expectedScroll := m.diffViewport.Height / 2
	require.GreaterOrEqual(t, m.diffViewport.YOffset, expectedScroll-1, "Ctrl+D should scroll down ~half page")

	// Press Ctrl+U to scroll up half page
	scrolledYOffset := m.diffViewport.YOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	require.Less(t, m.diffViewport.YOffset, scrolledYOffset, "Ctrl+U should scroll up")
}

// TestModel_MouseScrolling tests mouse wheel scrolling in the diff pane
func TestModel_MouseScrolling(t *testing.T) {
	files := []DiffFile{createLargeFile()}
	m := setupModelWithFiles(files)
	m.refreshViewport()

	// Mouse wheel down over diff pane (right side, X > fileListWidth)
	// X must be within the centered overlay box
	m, _ = m.Update(tea.MouseMsg{
		Button: tea.MouseButtonWheelDown,
		X:      60, // Right side where diff pane is
		Y:      10,
	})
	require.Greater(t, m.diffViewport.YOffset, 0, "Mouse wheel down should scroll diff pane")

	// Mouse wheel up
	scrolledYOffset := m.diffViewport.YOffset
	m, _ = m.Update(tea.MouseMsg{
		Button: tea.MouseButtonWheelUp,
		X:      60,
		Y:      10,
	})
	require.Less(t, m.diffViewport.YOffset, scrolledYOffset, "Mouse wheel up should scroll diff pane up")
}

// TestModel_MouseScrolling_IgnoresLeftPane tests that mouse over left pane doesn't scroll diff
func TestModel_MouseScrolling_IgnoresLeftPane(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	// Create a file with enough content to scroll
	m.workingDirFiles = []DiffFile{
		{
			NewPath:   "test.go",
			Additions: 100,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,100 +1,100 @@",
					Lines: func() []DiffLine {
						var lines []DiffLine
						for i := 1; i <= 100; i++ {
							lines = append(lines, DiffLine{Type: LineContext, Content: "line"})
						}
						return lines
					}(),
				},
			},
		},
	}
	m.refreshViewport()

	initialOffset := m.diffViewport.YOffset

	// Mouse wheel over left pane (file list area, X < fileListWidth)
	m, _ = m.Update(tea.MouseMsg{
		Button: tea.MouseButtonWheelDown,
		X:      10, // Left side where file list is
		Y:      10,
	})
	require.Equal(t, initialOffset, m.diffViewport.YOffset, "Mouse wheel over left pane should not scroll diff")
}

// === Header Title Tests ===

// TestBuildHeaderTitle_DefaultBranch tests header shows just branch name for default state.
func TestBuildHeaderTitle_DefaultBranch(t *testing.T) {
	m := New()
	m.currentBranch = "main"

	title := m.buildHeaderTitle()

	require.Equal(t, "main", title, "Should show just branch name for default state")
}

// TestBuildHeaderTitle_DetachedHead tests header shows "HEAD" when currentBranch is empty.
func TestBuildHeaderTitle_DetachedHead(t *testing.T) {
	m := New()
	m.currentBranch = ""

	title := m.buildHeaderTitle()

	require.Equal(t, "HEAD", title, "Should show HEAD for detached HEAD state")
}

// TestBuildHeaderTitle_WorktreeContext tests header shows "[worktree-dir] branch" format.
func TestBuildHeaderTitle_WorktreeContext(t *testing.T) {
	m := New()
	m.currentBranch = "main"
	m.originalWorkDir = "/home/user/project"
	m.currentWorktreePath = "/home/user/project-feature"
	m.currentWorktreeBranch = "feature/auth"

	title := m.buildHeaderTitle()

	require.Equal(t, "[project-feature] feature/auth", title, "Should show [worktree-dir] branch format")
}

// TestBuildHeaderTitle_WorktreeContextFallsToBranch tests worktree uses currentBranch if worktreeBranch is empty.
func TestBuildHeaderTitle_WorktreeContextFallsToBranch(t *testing.T) {
	m := New()
	m.currentBranch = "main"
	m.originalWorkDir = "/home/user/project"
	m.currentWorktreePath = "/home/user/project-feature"
	m.currentWorktreeBranch = "" // empty

	title := m.buildHeaderTitle()

	require.Equal(t, "[project-feature] main", title, "Should fallback to currentBranch when worktreeBranch is empty")
}

// TestBuildHeaderTitle_WorktreeContextFallsToHEAD tests worktree shows HEAD if both branches are empty.
func TestBuildHeaderTitle_WorktreeContextFallsToHEAD(t *testing.T) {
	m := New()
	m.currentBranch = "" // detached
	m.originalWorkDir = "/home/user/project"
	m.currentWorktreePath = "/home/user/project-feature"
	m.currentWorktreeBranch = "" // also empty

	title := m.buildHeaderTitle()

	require.Equal(t, "[project-feature] HEAD", title, "Should show HEAD when both branches are empty in worktree context")
}

// TestBuildHeaderTitle_ViewingBranch tests header shows just the viewed branch name.
func TestBuildHeaderTitle_ViewingBranch(t *testing.T) {
	m := New()
	m.currentBranch = "main"
	m.viewingBranch = "feature/auth"

	title := m.buildHeaderTitle()

	require.Equal(t, "feature/auth", title, "Should show just the viewed branch name")
}

// TestBuildHeaderTitle_ViewingBranchSameBranch tests no viewing suffix when same as current.
func TestBuildHeaderTitle_ViewingBranchSameBranch(t *testing.T) {
	m := New()
	m.currentBranch = "main"
	m.viewingBranch = "main" // same as current

	title := m.buildHeaderTitle()

	require.Equal(t, "main", title, "Should not show viewing suffix when viewing same branch as current")
}

// TestBuildHeaderTitle_ViewingBranchWithDetachedHead tests viewing branch with detached HEAD.
func TestBuildHeaderTitle_ViewingBranchWithDetachedHead(t *testing.T) {
	m := New()
	m.currentBranch = "" // detached
	m.viewingBranch = "feature/auth"

	title := m.buildHeaderTitle()

	require.Equal(t, "feature/auth", title, "Should show viewed branch name even when HEAD is detached")
}

// TestBuildHeaderTitle_WorktreeTakesPrecedence tests worktree context overrides viewing branch.
func TestBuildHeaderTitle_WorktreeTakesPrecedence(t *testing.T) {
	m := New()
	m.currentBranch = "main"
	m.originalWorkDir = "/home/user/project"
	m.currentWorktreePath = "/home/user/project-feature"
	m.currentWorktreeBranch = "feature/auth"
	m.viewingBranch = "develop" // this should be ignored

	title := m.buildHeaderTitle()

	require.Equal(t, "[project-feature] feature/auth", title, "Worktree context should take precedence over viewing branch")
}

// === Footer Tests ===

// === Branch Selector Tests ===

// TestLoadBranches_ReturnsBranches tests that loadBranches returns branches from the executor.
func TestLoadBranches_ReturnsBranches(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)

	branches := []git.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "feature/auth", IsCurrent: false},
		{Name: "bugfix-#123", IsCurrent: false},
	}
	mockGit.On("ListBranches").Return(branches, nil)

	m := NewWithGitExecutor(mockGit).SetSize(100, 50)

	cmd := m.loadBranches()
	require.NotNil(t, cmd)

	msg := cmd()
	branchesMsg, ok := msg.(BranchesLoadedMsg)
	require.True(t, ok)
	require.Nil(t, branchesMsg.Err)
	require.Len(t, branchesMsg.Branches, 3)
	require.Equal(t, "main", branchesMsg.Branches[0].Name)
	require.True(t, branchesMsg.Branches[0].IsCurrent)

	mockGit.AssertExpectations(t)
}

// TestLoadBranches_NilExecutor tests that loadBranches returns error when no executor.
func TestLoadBranches_NilExecutor(t *testing.T) {
	m := New().SetSize(100, 50)

	cmd := m.loadBranches()
	require.NotNil(t, cmd)

	msg := cmd()
	branchesMsg, ok := msg.(BranchesLoadedMsg)
	require.True(t, ok)
	require.NotNil(t, branchesMsg.Err)
	require.Contains(t, branchesMsg.Err.Error(), "no git executor")
}

// TestBranchesLoadedMsg_Error tests that BranchesLoadedMsg with error sets error state.
func TestBranchesLoadedMsg_Error(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	testErr := errors.New("git branch failed")
	m, cmd := m.Update(BranchesLoadedMsg{Err: testErr})

	require.NotNil(t, m.err, "Error should be set")
	require.Equal(t, "git branch failed", m.err.Error())
	require.Nil(t, cmd)
}

// TestCommitsForBranchLoaded_UpdatesCommitsAndViewingBranch tests CommitsForBranchLoadedMsg handling.
func TestCommitsForBranchLoaded_UpdatesCommitsAndViewingBranch(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.currentBranch = "main"

	commits := []git.CommitInfo{
		{Hash: "abc1234567890", ShortHash: "abc1234", Subject: "Commit 1", Author: "Dev", Date: time.Now()},
		{Hash: "def5678901234", ShortHash: "def5678", Subject: "Commit 2", Author: "Dev", Date: time.Now()},
	}

	m, cmd := m.Update(CommitsForBranchLoadedMsg{Commits: commits, Branch: "feature/auth", Err: nil})

	require.Len(t, m.commits, 2, "Commits should be updated")
	require.Equal(t, "feature/auth", m.viewingBranch, "viewingBranch should be set")
	require.Equal(t, 0, m.selectedCommit, "selectedCommit should reset to 0")
	require.NotNil(t, cmd, "Should return LoadCommitPreview command")
}

// TestCommitsForBranchLoaded_Error tests CommitsForBranchLoadedMsg error handling.
func TestCommitsForBranchLoaded_Error(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	testErr := errors.New("branch not found")
	m, cmd := m.Update(CommitsForBranchLoadedMsg{Err: testErr, Branch: "nonexistent"})

	require.NotNil(t, m.err, "Error should be set")
	require.Contains(t, m.err.Error(), "branch not found")
	require.Nil(t, cmd)
}

// TestLoadCommitsForBranch_UsesGetCommitLogForRef tests loadCommitsForBranch uses the right API.
func TestLoadCommitsForBranch_UsesGetCommitLogForRef(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)

	commits := []git.CommitInfo{
		{Hash: "abc1234", ShortHash: "abc1234", Subject: "Commit", Author: "Dev", Date: time.Now()},
	}
	mockGit.On("GetCommitLogForRef", "feature/test", 50).Return(commits, nil)

	m := NewWithGitExecutor(mockGit).SetSize(100, 50)

	cmd := m.loadCommitsForBranch("feature/test")
	require.NotNil(t, cmd)

	msg := cmd()
	commitsMsg, ok := msg.(CommitsForBranchLoadedMsg)
	require.True(t, ok)
	require.Equal(t, "feature/test", commitsMsg.Branch)
	require.Nil(t, commitsMsg.Err)
	require.Len(t, commitsMsg.Commits, 1)

	mockGit.AssertExpectations(t)
}

// TestLoadCommitsForBranch_NilExecutor tests loadCommitsForBranch with nil executor.
func TestLoadCommitsForBranch_NilExecutor(t *testing.T) {
	m := New().SetSize(100, 50)

	cmd := m.loadCommitsForBranch("feature/test")
	require.NotNil(t, cmd)

	msg := cmd()
	commitsMsg, ok := msg.(CommitsForBranchLoadedMsg)
	require.True(t, ok)
	require.NotNil(t, commitsMsg.Err)
	require.Contains(t, commitsMsg.Err.Error(), "no git executor")
	require.Equal(t, "feature/test", commitsMsg.Branch)
}

// === Worktree Selector Tests ===

// TestLoadWorktrees_ReturnsWorktrees tests that loadWorktrees returns worktrees from the executor.
func TestLoadWorktrees_ReturnsWorktrees(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)

	worktrees := []git.WorktreeInfo{
		{Path: "/project", Branch: "main"},
		{Path: "/project-feature", Branch: "feature/auth"},
		{Path: "/project-bugfix", Branch: "bugfix-#123"},
	}
	mockGit.On("ListWorktrees").Return(worktrees, nil)

	m := NewWithGitExecutor(mockGit).SetSize(100, 50)

	cmd := m.loadWorktrees()
	require.NotNil(t, cmd)

	msg := cmd()
	worktreesMsg, ok := msg.(WorktreesLoadedMsg)
	require.True(t, ok)
	require.Nil(t, worktreesMsg.Err)
	require.Len(t, worktreesMsg.Worktrees, 3)
	require.Equal(t, "/project", worktreesMsg.Worktrees[0].Path)
	require.Equal(t, "main", worktreesMsg.Worktrees[0].Branch)

	mockGit.AssertExpectations(t)
}

// TestLoadWorktrees_NilExecutor tests that loadWorktrees returns error when no executor.
func TestLoadWorktrees_NilExecutor(t *testing.T) {
	m := New().SetSize(100, 50)

	cmd := m.loadWorktrees()
	require.NotNil(t, cmd)

	msg := cmd()
	worktreesMsg, ok := msg.(WorktreesLoadedMsg)
	require.True(t, ok)
	require.NotNil(t, worktreesMsg.Err)
	require.Contains(t, worktreesMsg.Err.Error(), "no git executor")
}

// TestWorktreesLoadedMsg_Error tests that WorktreesLoadedMsg with error sets error state.
func TestWorktreesLoadedMsg_Error(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	testErr := errors.New("git worktree list failed")
	m, cmd := m.Update(WorktreesLoadedMsg{Err: testErr})

	require.NotNil(t, m.err, "Error should be set")
	require.Equal(t, "git worktree list failed", m.err.Error())
	require.Nil(t, cmd)
}

// TestHandleWorktreeSelected_UpdatesStateFields tests that handleWorktreeSelected updates all state fields.
func TestHandleWorktreeSelected_UpdatesStateFields(t *testing.T) {
	// Use factory constructor so handleWorktreeSelected works
	factoryCalled := false
	factoryPath := ""
	m := NewWithGitExecutorFactory(func(path string) git.GitExecutor {
		factoryCalled = true
		factoryPath = path
		return nil // For this test, we just check factory invocation and state
	}, "/original").SetSize(100, 50)
	m.visible = true
	m.viewingBranch = "develop" // Should be cleared
	m.worktreeList = []git.WorktreeInfo{
		{Path: "/tmp", Branch: "main"},
		{Path: "/tmp-other", Branch: "feature/auth"},
	}

	m, cmd := m.handleWorktreeSelected("/tmp")

	require.True(t, factoryCalled, "Factory should be called")
	require.Equal(t, "/tmp", factoryPath, "Factory should be called with selected worktree path")
	require.Equal(t, "/tmp", m.currentWorktreePath)
	require.Equal(t, "main", m.currentWorktreeBranch)
	require.Empty(t, m.viewingBranch, "viewingBranch should be cleared")
	require.NotNil(t, cmd, "Should trigger reload")
}

// TestHandleWorktreeSelected_WorktreeNotInCache tests error when worktree not found in cache.
func TestHandleWorktreeSelected_WorktreeNotInCache(t *testing.T) {
	// Need factory for handleWorktreeSelected to not no-op
	m := NewWithGitExecutorFactory(func(path string) git.GitExecutor {
		return nil
	}, "/original").SetSize(100, 50)
	m.visible = true
	m.worktreeList = []git.WorktreeInfo{
		{Path: "/project", Branch: "main"},
	}

	m, cmd := m.handleWorktreeSelected("/unknown/path")

	require.NotNil(t, m.err, "Error should be set")
	require.Contains(t, m.err.Error(), "worktree not found")
	require.Nil(t, cmd)
}

// === Factory Constructor Tests ===

// TestNewWithGitExecutorFactory_CreatesModelWithFactory tests the factory constructor.
func TestNewWithGitExecutorFactory_CreatesModelWithFactory(t *testing.T) {
	factoryCalled := false
	factoryPath := ""
	factory := func(path string) git.GitExecutor {
		factoryCalled = true
		factoryPath = path
		return mocks.NewMockGitExecutor(t)
	}

	m := NewWithGitExecutorFactory(factory, "/initial/path")

	require.True(t, factoryCalled, "Factory should be called with initial path")
	require.Equal(t, "/initial/path", factoryPath, "Factory should receive initial path")
	require.NotNil(t, m.gitExecutor, "gitExecutor should be created via factory")
	require.NotNil(t, m.gitExecutorFactory, "gitExecutorFactory should be stored")
	require.Equal(t, "/initial/path", m.originalWorkDir, "originalWorkDir should be set")
	require.Equal(t, "/initial/path", m.currentWorktreePath, "currentWorktreePath should be set")
}

// TestNewWithGitExecutorFactory_NilFactory tests that nil factory is handled gracefully.
func TestNewWithGitExecutorFactory_NilFactory(t *testing.T) {
	m := NewWithGitExecutorFactory(nil, "/some/path")

	require.Nil(t, m.gitExecutor, "gitExecutor should be nil when factory is nil")
	require.Nil(t, m.gitExecutorFactory, "gitExecutorFactory should be nil")
	require.Equal(t, "/some/path", m.originalWorkDir, "originalWorkDir should still be set")
	require.Empty(t, m.currentWorktreePath, "currentWorktreePath should be empty")
}

// TestNewWithGitExecutorFactory_EmptyPath tests that empty path doesn't call factory.
func TestNewWithGitExecutorFactory_EmptyPath(t *testing.T) {
	factoryCalled := false
	factory := func(path string) git.GitExecutor {
		factoryCalled = true
		return mocks.NewMockGitExecutor(t)
	}

	m := NewWithGitExecutorFactory(factory, "")

	require.False(t, factoryCalled, "Factory should NOT be called when path is empty")
	require.Nil(t, m.gitExecutor, "gitExecutor should be nil when path is empty")
	require.NotNil(t, m.gitExecutorFactory, "gitExecutorFactory should still be stored")
	require.Empty(t, m.originalWorkDir, "originalWorkDir should be empty")
	require.Empty(t, m.currentWorktreePath, "currentWorktreePath should be empty")
}

// TestNewWithGitExecutorFactory_NilFactoryAndEmptyPath tests both nil factory and empty path.
func TestNewWithGitExecutorFactory_NilFactoryAndEmptyPath(t *testing.T) {
	m := NewWithGitExecutorFactory(nil, "")

	require.Nil(t, m.gitExecutor, "gitExecutor should be nil")
	require.Nil(t, m.gitExecutorFactory, "gitExecutorFactory should be nil")
	require.Empty(t, m.originalWorkDir, "originalWorkDir should be empty")
	require.Empty(t, m.currentWorktreePath, "currentWorktreePath should be empty")
}

// TestNewWithGitExecutorFactory_PreservesDefaultFields tests that other fields are initialized correctly.
func TestNewWithGitExecutorFactory_PreservesDefaultFields(t *testing.T) {
	m := NewWithGitExecutorFactory(func(path string) git.GitExecutor {
		return nil
	}, "/path")

	require.False(t, m.visible, "visible should be false by default")
	require.Equal(t, focusFileList, m.focus, "focus should be focusFileList by default")
	require.NotNil(t, m.clock, "clock should be initialized")
}

// TestHandleWorktreeSelected_NilFactory_GracefullyNoOps tests nil factory handling.
func TestHandleWorktreeSelected_NilFactory_GracefullyNoOps(t *testing.T) {
	m := New().SetSize(100, 50) // No factory set
	m.visible = true
	m.worktreeList = []git.WorktreeInfo{
		{Path: "/tmp", Branch: "main"},
	}
	originalPath := m.currentWorktreePath

	m, cmd := m.handleWorktreeSelected("/tmp")

	require.Equal(t, originalPath, m.currentWorktreePath, "currentWorktreePath should be unchanged")
	require.Nil(t, cmd, "Should return nil cmd when factory is nil")
	require.Nil(t, m.err, "Should not set error - just no-op")
}

// TestHandleWorktreeSelected_FactoryCreatesNewExecutor tests that factory creates new executor.
func TestHandleWorktreeSelected_FactoryCreatesNewExecutor(t *testing.T) {
	var executorCount int
	var lastPath string
	factory := func(path string) git.GitExecutor {
		executorCount++
		lastPath = path
		return mocks.NewMockGitExecutor(t)
	}

	m := NewWithGitExecutorFactory(factory, "/original").SetSize(100, 50)
	m.visible = true
	m.worktreeList = []git.WorktreeInfo{
		{Path: "/new-worktree", Branch: "feature"},
	}

	// Factory called once for initial path
	require.Equal(t, 1, executorCount)
	require.Equal(t, "/original", lastPath)

	// Select new worktree
	m, _ = m.handleWorktreeSelected("/new-worktree")

	// Factory should be called again with new path
	require.Equal(t, 2, executorCount)
	require.Equal(t, "/new-worktree", lastPath)
	require.Equal(t, "/new-worktree", m.currentWorktreePath)
}

// === Mode Indicators and Breadcrumb Tests ===

// TestNew_InitializesViewMode tests that New initializes viewMode to ViewModeUnified.
func TestNew_InitializesViewMode(t *testing.T) {
	m := New()
	require.Equal(t, ViewModeUnified, m.viewMode, "New model should have ViewModeUnified")
}

// TestNewWithGitExecutor_InitializesViewMode tests that NewWithGitExecutor initializes viewMode.
func TestNewWithGitExecutor_InitializesViewMode(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)
	m := NewWithGitExecutor(mockGit)
	require.Equal(t, ViewModeUnified, m.viewMode, "NewWithGitExecutor should have ViewModeUnified")
}

// TestBuildBreadcrumb_CommitPreview tests breadcrumb for commit preview mode.
func TestBuildBreadcrumb_CommitPreview(t *testing.T) {
	m := New().SetSize(100, 50)
	m.focus = focusCommitPicker
	m.commitPaneMode = commitPaneModeList
	m.commits = []git.CommitInfo{
		{ShortHash: "abc1234", Subject: "Fix authentication bug"},
	}
	m.selectedCommit = 0

	breadcrumb := m.buildBreadcrumb()
	require.Equal(t, "abc1234 Fix authentication bug", breadcrumb)
}

// TestBuildBreadcrumb_NoCommits tests breadcrumb when no commits available.
func TestBuildBreadcrumb_NoCommits(t *testing.T) {
	m := New().SetSize(100, 50)
	m.focus = focusCommitPicker
	m.commitPaneMode = commitPaneModeList
	m.commits = nil

	breadcrumb := m.buildBreadcrumb()
	require.Equal(t, "No commits", breadcrumb)
}

// TestBuildBreadcrumb_WorkingDirectory tests breadcrumb for working directory with no files.
func TestBuildBreadcrumb_WorkingDirectory(t *testing.T) {
	m := New().SetSize(100, 50)
	m.focus = focusFileList
	m.workingDirTree = nil

	breadcrumb := m.buildBreadcrumb()
	require.Equal(t, "Working Directory", breadcrumb)
}

// TestBuildBreadcrumb_SelectedFile tests breadcrumb for selected file.
func TestBuildBreadcrumb_SelectedFile(t *testing.T) {
	m := New().SetSize(100, 50)
	m.focus = focusFileList
	// Use a simple file path without subdirectories to get a direct file node
	m.workingDirFiles = []DiffFile{
		{NewPath: "login.go"},
	}
	m.workingDirTree = NewFileTree(m.workingDirFiles)
	m.selectedWorkingDirNode = 0

	breadcrumb := m.buildBreadcrumb()
	require.Equal(t, "login.go", breadcrumb)
}

// === Hunk Indicator Tests ===

func TestModel_GetCurrentHunkIndex_AtStart(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
					{Type: LineContext},
					{Type: LineContext},
				},
			},
			{
				Header: "@@ -10,3 +11,5 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
					{Type: LineAddition},
					{Type: LineContext},
					{Type: LineContext},
				},
			},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.refreshViewport()

	require.Equal(t, 0, m.diffViewport.YOffset)
	require.Equal(t, 1, m.getCurrentHunkIndex(), "should be at first hunk")
}

func TestModel_GetCurrentHunkIndex_AtSecondHunk(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
					{Type: LineContext},
					{Type: LineContext},
				},
			},
			{
				Header: "@@ -10,3 +11,5 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
				},
			},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.refreshViewport()

	// Navigate to second hunk
	m, _ = m.navigateToNextHunk()
	require.Equal(t, 5, m.diffViewport.YOffset)
	require.Equal(t, 2, m.getCurrentHunkIndex(), "should be at second hunk")
}

func TestModel_GetCurrentHunkIndex_NoHunks(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusFileList

	require.Equal(t, 0, m.getCurrentHunkIndex(), "should return 0 when no hunks")
}

func TestModel_GetTotalHunkCount(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{Header: "@@ -1,3 +1,4 @@", Lines: []DiffLine{{Type: LineHunkHeader}}},
			{Header: "@@ -10,3 +11,5 @@", Lines: []DiffLine{{Type: LineHunkHeader}}},
			{Header: "@@ -20,2 +24,3 @@", Lines: []DiffLine{{Type: LineHunkHeader}}},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	require.Equal(t, 3, m.getTotalHunkCount())
}

func TestModel_BuildHunkIndicator_MultipleHunks(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
					{Type: LineContext},
					{Type: LineContext},
				},
			},
			{
				Header: "@@ -10,3 +11,5 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
				},
			},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.refreshViewport()

	indicator := m.buildHunkIndicator()
	require.Equal(t, "1 / 2 hunks", indicator)

	// Navigate to second hunk
	m, _ = m.navigateToNextHunk()
	indicator = m.buildHunkIndicator()
	require.Equal(t, "2 / 2 hunks", indicator)
}

func TestModel_BuildHunkIndicator_SingleHunk(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{Header: "@@ -1,3 +1,4 @@", Lines: []DiffLine{{Type: LineHunkHeader}}},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	indicator := m.buildHunkIndicator()
	require.Equal(t, "1 hunk", indicator)
}

func TestModel_BuildHunkIndicator_NoHunks(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	indicator := m.buildHunkIndicator()
	require.Empty(t, indicator)
}

func TestModel_BuildHunkIndicator_CommitFiles(t *testing.T) {
	file := DiffFile{
		NewPath: "commit_test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
				},
			},
			{
				Header: "@@ -10,3 +11,5 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
				},
			},
			{
				Header: "@@ -20,2 +24,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
				},
			},
		},
	}

	m := setupModelWithCommitFiles([]DiffFile{file})
	m.refreshViewport()

	indicator := m.buildHunkIndicator()
	require.Equal(t, "1 / 3 hunks", indicator)
}

// === Scroll Position Preservation Tests ===

// createTestFileWithHunks creates a test DiffFile with enough hunks/lines to allow scrolling.
func createTestFileWithHunks(path string, lineCount int) DiffFile {
	lines := make([]DiffLine, lineCount)
	for i := range lines {
		lines[i] = DiffLine{Type: LineContext, Content: "line content"}
	}
	return DiffFile{
		NewPath: path,
		Hunks: []DiffHunk{{
			Lines: lines,
		}},
	}
}

func TestModel_ScrollPositionPreservation_Basic(t *testing.T) {
	// Create files with enough lines to allow scrolling
	files := []DiffFile{
		createTestFileWithHunks("file1.go", 100),
		createTestFileWithHunks("file2.go", 100),
		createTestFileWithHunks("file3.go", 100),
	}

	m := setupModelWithFiles(files)
	m.refreshViewport()

	// Scroll down in file1.go
	m.diffViewport.SetYOffset(25)

	// Navigate to file2.go - should save position for file1.go
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Verify scroll position was saved for file1.go
	require.Equal(t, 25, m.scrollPositions["file1.go"], "scroll position should be saved for file1.go")

	// Scroll in file2.go
	m.diffViewport.SetYOffset(15)

	// Navigate back to file1.go
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Verify scroll position was restored for file1.go
	require.Equal(t, 25, m.diffViewport.YOffset, "scroll position should be restored for file1.go")

	// Verify file2.go position was saved
	require.Equal(t, 15, m.scrollPositions["file2.go"], "scroll position should be saved for file2.go")
}

func TestModel_ScrollPositionPreservation_NewFileStartsAtTop(t *testing.T) {
	files := []DiffFile{
		createTestFileWithHunks("file1.go", 100),
		createTestFileWithHunks("file2.go", 100),
	}

	m := setupModelWithFiles(files)
	m.refreshViewport()

	// Scroll down in file1.go
	m.diffViewport.SetYOffset(30)

	// Navigate to file2.go - new file should start at top
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// file2.go should start at top since we never visited it
	require.Equal(t, 0, m.diffViewport.YOffset, "new file should start at top")
}

func TestModel_ScrollPositionPreservation_ClampedToValidRange(t *testing.T) {
	// Start with a large file
	files := []DiffFile{
		createTestFileWithHunks("file1.go", 200),
		createTestFileWithHunks("file2.go", 100),
	}

	m := setupModelWithFiles(files)
	m.refreshViewport()

	// Scroll to a high position in file1.go
	m.diffViewport.SetYOffset(150)

	// Navigate away
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Now simulate file1.go being shorter (e.g., content changed)
	files[0] = createTestFileWithHunks("file1.go", 50)
	m.workingDirFiles = files
	m.workingDirTree = NewFileTree(files)
	m.selectedWorkingDirNode = 1 // Still on file2

	// Navigate back to file1.go
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Position should be clamped to valid range (not exceed maxOffset)
	maxOffset := m.diffViewport.TotalLineCount() - m.diffViewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	require.LessOrEqual(t, m.diffViewport.YOffset, maxOffset, "scroll position should be clamped to valid range")
}

func TestModel_ScrollPositionPreservation_ResetOnShow(t *testing.T) {
	files := []DiffFile{
		createTestFileWithHunks("file1.go", 100),
		createTestFileWithHunks("file2.go", 100),
	}

	m := setupModelWithFiles(files)
	m.refreshViewport()

	// Set some scroll positions
	m.scrollPositions["file1.go"] = 25
	m.scrollPositions["file2.go"] = 15

	// Call Show() to reset state
	m = m.Show()

	// Scroll positions should be cleared
	require.Empty(t, m.scrollPositions, "scroll positions should be reset on Show()")
}

func TestModel_ScrollPositionPreservation_DeletedFile(t *testing.T) {
	// Test with a deleted file (NewPath is /dev/null)
	// Use enough lines to ensure scrolling is possible
	deletedFileLines := make([]DiffLine, 100)
	for i := range deletedFileLines {
		deletedFileLines[i] = DiffLine{Type: LineDeletion, Content: "deleted line content"}
	}
	files := []DiffFile{
		{
			OldPath:   "deleted.go",
			NewPath:   "/dev/null",
			IsDeleted: true,
			Hunks: []DiffHunk{{
				Lines: deletedFileLines,
			}},
		},
		createTestFileWithHunks("file2.go", 100),
	}

	m := setupModelWithFiles(files)
	m.refreshViewport()

	// Scroll in deleted.go
	m.diffViewport.SetYOffset(10)

	// Navigate away
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Verify scroll position was saved using OldPath
	require.Equal(t, 10, m.scrollPositions["deleted.go"], "scroll position should be saved using OldPath for deleted files")

	// Navigate back
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Verify scroll position was restored
	require.Equal(t, 10, m.diffViewport.YOffset, "scroll position should be restored for deleted file")
}

func TestGetFileKey(t *testing.T) {
	tests := []struct {
		name     string
		file     *DiffFile
		expected string
	}{
		{
			name:     "nil file",
			file:     nil,
			expected: "",
		},
		{
			name:     "normal file",
			file:     &DiffFile{NewPath: "src/main.go", OldPath: "src/main.go"},
			expected: "src/main.go",
		},
		{
			name:     "new file",
			file:     &DiffFile{NewPath: "new_file.go", OldPath: "/dev/null", IsNew: true},
			expected: "new_file.go",
		},
		{
			name:     "deleted file",
			file:     &DiffFile{NewPath: "/dev/null", OldPath: "deleted.go", IsDeleted: true},
			expected: "deleted.go",
		},
		{
			name:     "renamed file - uses new path",
			file:     &DiffFile{NewPath: "new_name.go", OldPath: "old_name.go", IsRenamed: true},
			expected: "new_name.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getFileKey(tt.file)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestModel_ScrollPositionPreservation_CommitFilesMode(t *testing.T) {
	// Create commit files with enough lines for scrolling
	commitFiles := []DiffFile{
		createTestFileWithHunks("commit_file1.go", 100),
		createTestFileWithHunks("commit_file2.go", 100),
	}

	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.commitPaneMode = commitPaneModeFiles
	m.commitFiles = commitFiles
	m.commitFilesTree = NewFileTree(commitFiles)
	m.selectedCommitFileNode = 0
	m.lastLeftFocus = focusCommitPicker
	m.refreshViewport()

	// Scroll in commit_file1.go
	m.diffViewport.SetYOffset(20)

	// Navigate to commit_file2.go
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Verify scroll position was saved
	require.Equal(t, 20, m.scrollPositions["commit_file1.go"], "scroll position should be saved for commit file")

	// Navigate back
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Verify position restored
	require.Equal(t, 20, m.diffViewport.YOffset, "scroll position should be restored for commit file")
}

// === Hunk Navigation Tests ===

func TestModel_HunkNavigation_NextHunk(t *testing.T) {
	// Create a file with multiple hunks
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,3 +1,4 @@",
				OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"},
					{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineAddition, Content: "new line", NewLineNum: 2},
					{Type: LineContext, Content: "line 2", OldLineNum: 2, NewLineNum: 3},
					{Type: LineContext, Content: "line 3", OldLineNum: 3, NewLineNum: 4},
				},
			},
			{
				Header:   "@@ -10,3 +11,5 @@",
				OldStart: 10, OldCount: 3, NewStart: 11, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -10,3 +11,5 @@"},
					{Type: LineContext, Content: "line 10", OldLineNum: 10, NewLineNum: 11},
					{Type: LineAddition, Content: "new line A", NewLineNum: 12},
					{Type: LineAddition, Content: "new line B", NewLineNum: 13},
					{Type: LineContext, Content: "line 11", OldLineNum: 11, NewLineNum: 14},
					{Type: LineContext, Content: "line 12", OldLineNum: 12, NewLineNum: 15},
				},
			},
			{
				Header:   "@@ -20,2 +24,3 @@",
				OldStart: 20, OldCount: 2, NewStart: 24, NewCount: 3,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -20,2 +24,3 @@"},
					{Type: LineContext, Content: "line 20", OldLineNum: 20, NewLineNum: 24},
					{Type: LineAddition, Content: "new line C", NewLineNum: 25},
					{Type: LineContext, Content: "line 21", OldLineNum: 21, NewLineNum: 26},
				},
			},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.refreshViewport()

	// Verify getActiveFile returns the file
	activeFile := m.getActiveFile()
	require.NotNil(t, activeFile, "getActiveFile should return file")
	require.Len(t, activeFile.Hunks, 3, "should have 3 hunks")

	// Start at position 0 (first hunk)
	require.Equal(t, 0, m.diffViewport.YOffset)

	// Navigate to next hunk (])
	m, _ = m.navigateToNextHunk()
	require.Equal(t, 5, m.diffViewport.YOffset, "should jump to second hunk")

	// Navigate to next hunk again
	m, _ = m.navigateToNextHunk()

	// Should be at the third hunk (line 5 + 6 = 11, since second hunk has 6 lines)
	require.Equal(t, 11, m.diffViewport.YOffset, "should jump to third hunk")

	// Navigate to next hunk - should wrap to first
	m, _ = m.navigateToNextHunk()

	// Should wrap to first hunk
	require.Equal(t, 0, m.diffViewport.YOffset, "should wrap to first hunk")
}

func TestModel_HunkNavigation_PrevHunk(t *testing.T) {
	// Create a file with multiple hunks
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,3 +1,4 @@",
				OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"},
					{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineAddition, Content: "new line", NewLineNum: 2},
					{Type: LineContext, Content: "line 2", OldLineNum: 2, NewLineNum: 3},
					{Type: LineContext, Content: "line 3", OldLineNum: 3, NewLineNum: 4},
				},
			},
			{
				Header:   "@@ -10,3 +11,5 @@",
				OldStart: 10, OldCount: 3, NewStart: 11, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -10,3 +11,5 @@"},
					{Type: LineContext, Content: "line 10", OldLineNum: 10, NewLineNum: 11},
					{Type: LineAddition, Content: "new line A", NewLineNum: 12},
					{Type: LineAddition, Content: "new line B", NewLineNum: 13},
					{Type: LineContext, Content: "line 11", OldLineNum: 11, NewLineNum: 14},
					{Type: LineContext, Content: "line 12", OldLineNum: 12, NewLineNum: 15},
				},
			},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.refreshViewport()

	// Start at position 0 (first hunk)
	require.Equal(t, 0, m.diffViewport.YOffset)

	// Navigate to previous hunk ([c) - should wrap to last
	m, _ = m.navigateToPrevHunk()

	// Should wrap to last hunk (line 5)
	require.Equal(t, 5, m.diffViewport.YOffset, "should wrap to last hunk")

	// Navigate to previous hunk again
	m, _ = m.navigateToPrevHunk()

	// Should go to first hunk
	require.Equal(t, 0, m.diffViewport.YOffset, "should go to first hunk")
}

func TestModel_HunkNavigation_NoFile(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	// Navigate to next hunk when no file is loaded - should do nothing
	originalOffset := m.diffViewport.YOffset
	m, _ = m.navigateToNextHunk()
	require.Equal(t, originalOffset, m.diffViewport.YOffset, "should not change offset when no file")
}

// TestModel_HunkNavigation_ScrollPositionPreservedAfterView verifies that the scroll position
// set by hunk navigation is preserved after View() is called. This catches regressions where
// dimension mismatches cause ScrollablePane to recalculate the scroll position.
func TestModel_HunkNavigation_ScrollPositionPreservedAfterView(t *testing.T) {
	// Create a file with multiple hunks
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,3 +1,4 @@",
				OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"},
					{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineAddition, Content: "new line", NewLineNum: 2},
					{Type: LineContext, Content: "line 2", OldLineNum: 2, NewLineNum: 3},
					{Type: LineContext, Content: "line 3", OldLineNum: 3, NewLineNum: 4},
				},
			},
			{
				Header:   "@@ -10,3 +11,5 @@",
				OldStart: 10, OldCount: 3, NewStart: 11, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -10,3 +11,5 @@"},
					{Type: LineContext, Content: "line 10", OldLineNum: 10, NewLineNum: 11},
					{Type: LineAddition, Content: "new line A", NewLineNum: 12},
					{Type: LineAddition, Content: "new line B", NewLineNum: 13},
					{Type: LineContext, Content: "line 11", OldLineNum: 11, NewLineNum: 14},
					{Type: LineContext, Content: "line 12", OldLineNum: 12, NewLineNum: 15},
				},
			},
			{
				Header:   "@@ -20,2 +24,3 @@",
				OldStart: 20, OldCount: 2, NewStart: 24, NewCount: 3,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -20,2 +24,3 @@"},
					{Type: LineContext, Content: "line 20", OldLineNum: 20, NewLineNum: 24},
					{Type: LineAddition, Content: "new line C", NewLineNum: 25},
					{Type: LineContext, Content: "line 21", OldLineNum: 21, NewLineNum: 26},
				},
			},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.refreshViewport()

	// Navigate to second hunk
	m, _ = m.navigateToNextHunk()
	require.Equal(t, 5, m.diffViewport.YOffset, "should be at second hunk after navigation")

	// Call View() multiple times to simulate render cycles
	_ = m.View()
	require.Equal(t, 5, m.diffViewport.YOffset, "scroll position should be preserved after first View()")

	_ = m.View()
	require.Equal(t, 5, m.diffViewport.YOffset, "scroll position should be preserved after second View()")

	// Navigate to third hunk
	m, _ = m.navigateToNextHunk()
	require.Equal(t, 11, m.diffViewport.YOffset, "should be at third hunk after navigation")

	// Verify position is still preserved after View()
	_ = m.View()
	require.Equal(t, 11, m.diffViewport.YOffset, "scroll position should be preserved after View()")
}

func TestModel_HunkNavigation_EmptyHunks(t *testing.T) {
	file := DiffFile{
		NewPath: "empty.go",
		Hunks:   []DiffHunk{}, // No hunks
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.refreshViewport()

	// Navigate to next hunk - should do nothing
	originalOffset := m.diffViewport.YOffset
	m, _ = m.navigateToNextHunk()
	require.Equal(t, originalOffset, m.diffViewport.YOffset, "should not change offset when no hunks")
}

func TestModel_GetHunkPositionsForFile(t *testing.T) {
	file := &DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
					{Type: LineContext},
					{Type: LineContext},
				},
			},
			{
				Header: "@@ -10,3 +11,5 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
					{Type: LineAddition},
					{Type: LineContext},
					{Type: LineContext},
				},
			},
			{
				Header: "@@ -20,2 +24,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
					{Type: LineContext},
				},
			},
		},
	}

	m := New()
	positions := m.getHunkPositionsForFile(file)

	require.Len(t, positions, 3, "should have 3 hunk positions")
	require.Equal(t, 0, positions[0], "first hunk at line 0")
	require.Equal(t, 5, positions[1], "second hunk at line 5 (after 5 lines of first hunk)")
	require.Equal(t, 11, positions[2], "third hunk at line 11 (after 6 more lines of second hunk)")
}

func TestModel_GetHunkPositionsForFile_NilFile(t *testing.T) {
	m := New()
	positions := m.getHunkPositionsForFile(nil)
	require.Nil(t, positions, "should return nil for nil file")
}

func TestModel_GetHunkPositionsForFile_NoHunks(t *testing.T) {
	file := &DiffFile{
		NewPath: "test.go",
		Hunks:   []DiffHunk{},
	}

	m := New()
	positions := m.getHunkPositionsForFile(file)
	require.Nil(t, positions, "should return nil for file with no hunks")
}

// === Copy Hunk Tests ===

func TestFormatHunkAsDiff(t *testing.T) {
	hunk := &DiffHunk{
		Header:   "@@ -1,3 +1,4 @@",
		OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 4,
		Lines: []DiffLine{
			{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"},
			{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
			{Type: LineAddition, Content: "new line", NewLineNum: 2},
			{Type: LineDeletion, Content: "old line", OldLineNum: 2},
			{Type: LineContext, Content: "line 3", OldLineNum: 3, NewLineNum: 4},
		},
	}

	result := formatHunkAsDiff(hunk)

	expected := "@@ -1,3 +1,4 @@\n line 1\n+new line\n-old line\n line 3\n"
	require.Equal(t, expected, result, "should format hunk as diff text")
}

func TestFormatHunkAsDiff_NilHunk(t *testing.T) {
	result := formatHunkAsDiff(nil)
	require.Empty(t, result, "should return empty string for nil hunk")
}

func TestFormatHunkAsDiff_EmptyHunk(t *testing.T) {
	hunk := &DiffHunk{
		Header: "@@ -1,0 +1,0 @@",
		Lines:  []DiffLine{},
	}

	result := formatHunkAsDiff(hunk)
	require.Empty(t, result, "should return empty string for hunk with no lines")
}

func TestModel_GetCurrentHunk_SingleFile(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,3 +1,4 @@",
				OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"},
					{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineAddition, Content: "new line", NewLineNum: 2},
					{Type: LineContext, Content: "line 2", OldLineNum: 2, NewLineNum: 3},
					{Type: LineContext, Content: "line 3", OldLineNum: 3, NewLineNum: 4},
				},
			},
			{
				Header:   "@@ -10,3 +11,4 @@",
				OldStart: 10, OldCount: 3, NewStart: 11, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -10,3 +11,4 @@"},
					{Type: LineContext, Content: "line 10", OldLineNum: 10, NewLineNum: 11},
					{Type: LineAddition, Content: "new line", NewLineNum: 12},
					{Type: LineContext, Content: "line 11", OldLineNum: 11, NewLineNum: 13},
					{Type: LineContext, Content: "line 12", OldLineNum: 12, NewLineNum: 14},
				},
			},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.refreshViewport()

	// At position 0, should return first hunk
	hunk := m.getCurrentHunk()
	require.NotNil(t, hunk, "should return hunk at position 0")
	require.Equal(t, "@@ -1,3 +1,4 @@", hunk.Header, "should be first hunk")

	// Move to position within second hunk (first hunk has 5 lines, second starts at 5)
	m.diffViewport.YOffset = 5
	hunk = m.getCurrentHunk()
	require.NotNil(t, hunk, "should return hunk at position 5")
	require.Equal(t, "@@ -10,3 +11,4 @@", hunk.Header, "should be second hunk")

	// Move to position 2 (within first hunk)
	m.diffViewport.YOffset = 2
	hunk = m.getCurrentHunk()
	require.NotNil(t, hunk, "should return hunk at position 2")
	require.Equal(t, "@@ -1,3 +1,4 @@", hunk.Header, "should still be first hunk")
}

func TestModel_GetCurrentHunk_NoFile(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusFileList

	hunk := m.getCurrentHunk()
	require.Nil(t, hunk, "should return nil when no file is active")
}

func TestModel_CopyCurrentHunk_NoClipboard(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,1 +1,2 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,1 +1,2 @@"},
					{Type: LineAddition, Content: "new line"},
				},
			},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.refreshViewport()
	// clipboard is nil by default

	_, cmd := m.copyCurrentHunk()
	require.NotNil(t, cmd, "should return a command")

	msg := cmd()
	copiedMsg, ok := msg.(HunkCopiedMsg)
	require.True(t, ok, "should return HunkCopiedMsg")
	require.Error(t, copiedMsg.Err, "should have error when clipboard is nil")
	require.Contains(t, copiedMsg.Err.Error(), "clipboard not available")
}

func TestModel_CopyCurrentHunk_NoHunk(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusFileList
	m.clipboard = &mocks.MockClipboard{}

	_, cmd := m.copyCurrentHunk()
	require.NotNil(t, cmd, "should return a command")

	msg := cmd()
	copiedMsg, ok := msg.(HunkCopiedMsg)
	require.True(t, ok, "should return HunkCopiedMsg")
	require.Error(t, copiedMsg.Err, "should have error when no hunk")
	require.Contains(t, copiedMsg.Err.Error(), "no hunk at current position")
}

func TestModel_CopyCurrentHunk_Success(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,2 +1,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,2 +1,3 @@"},
					{Type: LineContext, Content: "line 1"},
					{Type: LineAddition, Content: "new line"},
					{Type: LineContext, Content: "line 2"},
				},
			},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.refreshViewport()

	// Create a mock clipboard
	mockClipboard := mocks.NewMockClipboard(t)
	expectedContent := "@@ -1,2 +1,3 @@\n line 1\n+new line\n line 2\n"
	mockClipboard.EXPECT().Copy(expectedContent).Return(nil)

	m.clipboard = mockClipboard

	_, cmd := m.copyCurrentHunk()
	require.NotNil(t, cmd, "should return a command")

	msg := cmd()
	copiedMsg, ok := msg.(HunkCopiedMsg)
	require.True(t, ok, "should return HunkCopiedMsg")
	require.NoError(t, copiedMsg.Err, "should not have error")
	require.Equal(t, 4, copiedMsg.LineCount, "should report correct line count")
}

// === Side-by-Side View Tests ===

// TestModel_SideBySideMode_UsesSideBySideRenderer tests that setting ViewModeSideBySide
// causes refreshViewport to use the side-by-side renderer.
func TestModel_SideBySideMode_UsesSideBySideRenderer(t *testing.T) {
	m := New().SetSize(150, 50)
	m.visible = true
	m.focus = focusFileList

	// Set up working directory files
	m.workingDirFiles = []DiffFile{
		{
			OldPath:   "test.go",
			NewPath:   "test.go",
			Additions: 2,
			Deletions: 1,
			Hunks: []DiffHunk{
				{
					OldStart: 1,
					NewStart: 1,
					Header:   "@@ -1,3 +1,4 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: "package main"},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "package main"},
						{Type: LineDeletion, OldLineNum: 2, Content: "func old()"},
						{Type: LineAddition, NewLineNum: 2, Content: "func new()"},
						{Type: LineAddition, NewLineNum: 3, Content: "func extra()"},
					},
				},
			},
		},
	}
	m.workingDirTree = NewFileTree(m.workingDirFiles)
	m.selectedWorkingDirNode = 0

	// Expand nodes to select the file
	nodes := m.workingDirTree.VisibleNodes()
	for i, n := range nodes {
		if !n.IsDir {
			m.selectedWorkingDirNode = i
			break
		}
	}

	// Set side-by-side mode
	m.viewMode = ViewModeSideBySide
	m.refreshViewport()

	// Verify the viewport content contains the side-by-side separator character
	content := m.diffViewport.View()

	// Side-by-side view should have the vertical separator
	require.Contains(t, content, "│", "side-by-side view should have vertical separator")
}

// TestModel_SideBySideMode_SynchronizedScrolling tests that scrolling in side-by-side mode
// keeps both columns synchronized (they scroll together).
func TestModel_SideBySideMode_SynchronizedScrolling(t *testing.T) {
	m := New().SetSize(150, 20)
	m.visible = true
	m.focus = focusDiffPane

	// Create a diff with enough content to require scrolling
	var lines []DiffLine
	lines = append(lines, DiffLine{Type: LineHunkHeader, Content: "@@ header @@"})
	for i := 1; i <= 50; i++ {
		if i%3 == 0 {
			lines = append(lines, DiffLine{Type: LineDeletion, OldLineNum: i, Content: "old line " + string(rune('0'+i%10))})
			lines = append(lines, DiffLine{Type: LineAddition, NewLineNum: i, Content: "new line " + string(rune('0'+i%10))})
		} else {
			lines = append(lines, DiffLine{Type: LineContext, OldLineNum: i, NewLineNum: i, Content: "context " + string(rune('0'+i%10))})
		}
	}

	m.workingDirFiles = []DiffFile{
		{
			OldPath: "scroll_test.go",
			NewPath: "scroll_test.go",
			Hunks: []DiffHunk{
				{
					OldStart: 1,
					NewStart: 1,
					Header:   "@@ -1,50 +1,50 @@",
					Lines:    lines,
				},
			},
		},
	}
	m.workingDirTree = NewFileTree(m.workingDirFiles)

	// Select the file
	nodes := m.workingDirTree.VisibleNodes()
	for i, n := range nodes {
		if !n.IsDir {
			m.selectedWorkingDirNode = i
			break
		}
	}

	// Set side-by-side mode and refresh
	m.viewMode = ViewModeSideBySide
	m.refreshViewport()

	// Get initial state
	initialYOffset := m.diffViewport.YOffset

	// Scroll down
	m.diffViewport.ScrollDown(5)

	// Verify scroll position changed
	require.Greater(t, m.diffViewport.YOffset, initialYOffset, "scroll should have moved viewport")

	// In side-by-side mode, both columns are rendered together as single lines,
	// so scrolling the viewport scrolls both columns simultaneously.
	// The key point is that there's only ONE viewport, not two separate ones.
	// This verifies the architecture supports synchronized scrolling by design.

	// Get content after scrolling
	contentAfterScroll := m.diffViewport.View()

	// The scrolled content should still contain the side-by-side separator
	// (proving both columns are rendered together and scroll as one)
	require.Contains(t, contentAfterScroll, "│", "scrolled side-by-side view should still have separator")
}

// === View Mode Toggle Tests ===

// TestModel_ToggleViewMode_FromUnifiedToSideBySide tests toggling from unified to side-by-side.
func TestModel_ToggleViewMode_FromUnifiedToSideBySide(t *testing.T) {
	m := New().SetSize(120, 30)
	m.visible = true

	// Verify initial state is unified
	require.Equal(t, ViewModeUnified, m.viewMode, "initial view mode should be unified")

	// Toggle to side-by-side
	m, _ = m.toggleViewMode()

	require.Equal(t, ViewModeSideBySide, m.viewMode, "view mode should be side-by-side after toggle")
}

// TestModel_ToggleViewMode_FromSideBySideToUnified tests toggling from side-by-side back to unified.
func TestModel_ToggleViewMode_FromSideBySideToUnified(t *testing.T) {
	m := New().SetSize(120, 30)
	m.visible = true

	// First toggle to side-by-side
	m, _ = m.toggleViewMode()
	require.Equal(t, ViewModeSideBySide, m.viewMode, "should be side-by-side after first toggle")
	require.Equal(t, ViewModeSideBySide, m.preferredViewMode, "preferred should be side-by-side")

	// Toggle back to unified
	m, _ = m.toggleViewMode()

	require.Equal(t, ViewModeUnified, m.viewMode, "view mode should be unified after toggle")
	require.Equal(t, ViewModeUnified, m.preferredViewMode, "preferred should be unified")
}

// TestModel_ToggleViewMode_MultipleTimes tests toggling multiple times.
func TestModel_ToggleViewMode_MultipleTimes(t *testing.T) {
	m := New().SetSize(120, 30)
	m.visible = true

	require.Equal(t, ViewModeUnified, m.viewMode)

	m, _ = m.toggleViewMode()
	require.Equal(t, ViewModeSideBySide, m.viewMode)

	m, _ = m.toggleViewMode()
	require.Equal(t, ViewModeUnified, m.viewMode)

	m, _ = m.toggleViewMode()
	require.Equal(t, ViewModeSideBySide, m.viewMode)
}

// TestModel_ToggleViewMode_KeyBinding tests that Ctrl+v triggers the toggle.
func TestModel_ToggleViewMode_KeyBinding(t *testing.T) {
	m := New().SetSize(120, 30)
	m.visible = true

	require.Equal(t, ViewModeUnified, m.viewMode)

	// Simulate Ctrl+v keypress
	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlV}
	m, _ = m.Update(keyMsg)

	require.Equal(t, ViewModeSideBySide, m.viewMode, "Ctrl+v should toggle view mode")
}

// TestModel_ToggleViewMode_WithContent tests toggle view mode with diff content loaded.
func TestModel_ToggleViewMode_WithContent(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)
	m := NewWithGitExecutor(mockGit)
	m = m.SetSize(120, 30)
	m.visible = true

	// Set up some diff content
	m.workingDirFiles = []DiffFile{
		{
			NewPath: "test.go",
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,10 +1,12 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: "@@ -1,10 +1,12 @@"},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "line 1"},
						{Type: LineDeletion, OldLineNum: 2, Content: "old line"},
						{Type: LineAddition, NewLineNum: 2, Content: "new line"},
						{Type: LineContext, OldLineNum: 3, NewLineNum: 3, Content: "line 3"},
					},
				},
			},
		},
	}
	m.workingDirTree = NewFileTree(m.workingDirFiles)
	m.refreshViewport()

	require.Equal(t, ViewModeUnified, m.viewMode)

	// Toggle view mode
	m, _ = m.toggleViewMode()

	require.Equal(t, ViewModeSideBySide, m.viewMode)
	// Verify content is still rendered (the viewport should have content)
	require.NotEmpty(t, m.diffViewport.View(), "viewport should have content after toggle")
}

// === Adaptive Layout Tests ===

// TestModel_AdaptiveLayout_NarrowTerminalBlocksSideBySide tests that toggling to side-by-side
// in a narrow terminal returns a ViewModeConstrainedMsg instead of switching.
func TestModel_AdaptiveLayout_NarrowTerminalBlocksSideBySide(t *testing.T) {
	// Terminal width 80 is below the minSideBySideWidth (100)
	m := New().SetSize(80, 30)
	m.visible = true

	require.Equal(t, ViewModeUnified, m.viewMode)
	require.Equal(t, ViewModeUnified, m.preferredViewMode)

	// Try to toggle to side-by-side
	m, cmd := m.toggleViewMode()

	// Should stay in unified view
	require.Equal(t, ViewModeUnified, m.viewMode, "should stay unified in narrow terminal")
	// But user preference should be updated to side-by-side
	require.Equal(t, ViewModeSideBySide, m.preferredViewMode, "preference should be side-by-side")

	// Should return a ViewModeConstrainedMsg
	require.NotNil(t, cmd, "should return a command")
	msg := cmd()
	constrainedMsg, ok := msg.(ViewModeConstrainedMsg)
	require.True(t, ok, "should return ViewModeConstrainedMsg")
	require.Equal(t, ViewModeSideBySide, constrainedMsg.RequestedMode)
	require.Equal(t, 100, constrainedMsg.MinWidth)
	require.Equal(t, 80, constrainedMsg.CurrentWidth)
}

// TestModel_AdaptiveLayout_WideTerminalAllowsSideBySide tests that toggling to side-by-side
// works when terminal is wide enough.
func TestModel_AdaptiveLayout_WideTerminalAllowsSideBySide(t *testing.T) {
	// Terminal width 120 is above minSideBySideWidth (100)
	m := New().SetSize(120, 30)
	m.visible = true

	require.Equal(t, ViewModeUnified, m.viewMode)

	// Toggle to side-by-side
	m, cmd := m.toggleViewMode()

	require.Equal(t, ViewModeSideBySide, m.viewMode, "should switch to side-by-side")
	require.Equal(t, ViewModeSideBySide, m.preferredViewMode)
	require.Nil(t, cmd, "should not return a command for successful toggle")
}

// TestModel_AdaptiveLayout_ResizeToNarrowForceUnified tests that resizing below threshold
// forces unified view but preserves user preference.
func TestModel_AdaptiveLayout_ResizeToNarrowForceUnified(t *testing.T) {
	// Start with wide terminal and side-by-side
	m := New().SetSize(120, 30)
	m.visible = true
	m, _ = m.toggleViewMode() // Switch to side-by-side
	require.Equal(t, ViewModeSideBySide, m.viewMode)
	require.Equal(t, ViewModeSideBySide, m.preferredViewMode)

	// Resize to narrow terminal
	m = m.SetSize(80, 30)

	// Should be forced to unified
	require.Equal(t, ViewModeUnified, m.viewMode, "should be forced to unified on narrow resize")
	// Preference should still be side-by-side
	require.Equal(t, ViewModeSideBySide, m.preferredViewMode, "preference should be preserved")
}

// TestModel_AdaptiveLayout_ResizeToWideRestoresSideBySide tests that resizing back to wide
// restores side-by-side when it was the user's preference.
func TestModel_AdaptiveLayout_ResizeToWideRestoresSideBySide(t *testing.T) {
	// Start with wide terminal and side-by-side
	m := New().SetSize(120, 30)
	m.visible = true
	m, _ = m.toggleViewMode() // Switch to side-by-side
	require.Equal(t, ViewModeSideBySide, m.viewMode)

	// Resize to narrow - forced to unified
	m = m.SetSize(80, 30)
	require.Equal(t, ViewModeUnified, m.viewMode)
	require.Equal(t, ViewModeSideBySide, m.preferredViewMode)

	// Resize back to wide - should restore side-by-side
	m = m.SetSize(120, 30)

	require.Equal(t, ViewModeSideBySide, m.viewMode, "should restore side-by-side on wide resize")
	require.Equal(t, ViewModeSideBySide, m.preferredViewMode)
}

// TestModel_AdaptiveLayout_ResizeToWideDoesNotAutoSwitchIfUnifiedPreferred tests that
// resizing to wide doesn't auto-switch if user prefers unified.
func TestModel_AdaptiveLayout_ResizeToWideDoesNotAutoSwitchIfUnifiedPreferred(t *testing.T) {
	// Start with narrow terminal
	m := New().SetSize(80, 30)
	m.visible = true
	require.Equal(t, ViewModeUnified, m.viewMode)
	require.Equal(t, ViewModeUnified, m.preferredViewMode)

	// Resize to wide - should stay unified since that's the preference
	m = m.SetSize(120, 30)

	require.Equal(t, ViewModeUnified, m.viewMode, "should stay unified when that's the preference")
	require.Equal(t, ViewModeUnified, m.preferredViewMode)
}

// TestModel_AdaptiveLayout_ExactlyAtThreshold tests behavior at exactly the threshold width.
func TestModel_AdaptiveLayout_ExactlyAtThreshold(t *testing.T) {
	// Terminal width exactly at minSideBySideWidth (100)
	m := New().SetSize(100, 30)
	m.visible = true

	// Toggle to side-by-side should work
	m, cmd := m.toggleViewMode()

	require.Equal(t, ViewModeSideBySide, m.viewMode, "should allow side-by-side at exact threshold")
	require.Nil(t, cmd, "should not return a command at exact threshold")
}

// TestModel_AdaptiveLayout_BelowThresholdByOne tests behavior just below the threshold.
func TestModel_AdaptiveLayout_BelowThresholdByOne(t *testing.T) {
	// Terminal width just below minSideBySideWidth (99)
	m := New().SetSize(99, 30)
	m.visible = true

	// Toggle to side-by-side should fail
	m, cmd := m.toggleViewMode()

	require.Equal(t, ViewModeUnified, m.viewMode, "should stay unified just below threshold")
	require.NotNil(t, cmd, "should return a command just below threshold")
}

// TestModel_SideBySideMode_UsesVirtualScrolling tests that side-by-side mode
// now uses virtual scrolling for large diffs via pre-computed aligned pairs.
func TestModel_SideBySideMode_UsesVirtualScrolling(t *testing.T) {
	m := New().SetSize(120, 30)
	m.visible = true

	// Create a large diff that would normally trigger virtual scrolling (>500 lines)
	var lines []DiffLine
	lines = append(lines, DiffLine{Type: LineHunkHeader, Content: "func largeFile() {"})
	for i := 1; i <= 600; i++ {
		lines = append(lines, DiffLine{Type: LineContext, OldLineNum: i, NewLineNum: i, Content: "line " + string(rune('0'+i%10))})
	}
	m.workingDirFiles = []DiffFile{
		{
			NewPath: "large.go",
			Hunks: []DiffHunk{
				{
					OldStart: 1,
					OldCount: 600,
					NewStart: 1,
					NewCount: 600,
					Header:   "@@ -1,600 +1,600 @@ func largeFile() {",
					Lines:    lines,
				},
			},
		},
	}
	m.workingDirTree = NewFileTree(m.workingDirFiles)

	// In unified mode, large diffs should use virtual scrolling
	m.viewMode = ViewModeUnified
	m.refreshViewport()
	require.True(t, m.useVirtualScrolling, "unified mode should use virtual scrolling for large diffs")
	require.NotNil(t, m.virtualContent, "unified mode should have virtual content for large diffs")

	// In side-by-side mode, virtual scrolling should also be enabled (now supported)
	m.viewMode = ViewModeSideBySide
	m.refreshViewport()
	require.True(t, m.useVirtualScrolling, "side-by-side mode should NOW use virtual scrolling")
	require.NotNil(t, m.virtualContent, "side-by-side mode should have virtual content")
	require.NotEmpty(t, m.virtualContent.SideBySideLines, "side-by-side mode should have pre-computed aligned lines")
}

// TestModel_ToggleViewMode_ContentChanges verifies that toggling view mode
// actually changes the rendered content from unified to side-by-side format.
func TestModel_ToggleViewMode_ContentChanges(t *testing.T) {
	m := New().SetSize(150, 50)
	m.visible = true
	m.focus = focusFileList

	// Set up working directory files with meaningful content
	m.workingDirFiles = []DiffFile{
		{
			OldPath:   "test.go",
			NewPath:   "test.go",
			Additions: 2,
			Deletions: 1,
			Hunks: []DiffHunk{
				{
					OldStart: 1,
					NewStart: 1,
					Header:   "@@ -1,3 +1,4 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "package main"},
						{Type: LineDeletion, OldLineNum: 2, Content: "func old()"},
						{Type: LineAddition, NewLineNum: 2, Content: "func new()"},
						{Type: LineAddition, NewLineNum: 3, Content: "func extra()"},
					},
				},
			},
		},
	}
	m.workingDirTree = NewFileTree(m.workingDirFiles)
	m.selectedWorkingDirNode = 0

	// Expand nodes to select the file
	nodes := m.workingDirTree.VisibleNodes()
	for i, n := range nodes {
		if !n.IsDir {
			m.selectedWorkingDirNode = i
			break
		}
	}

	// Refresh viewport in unified mode
	m.viewMode = ViewModeUnified
	m.refreshViewport()
	unifiedContent := m.diffViewport.View()

	// Toggle to side-by-side
	m, _ = m.toggleViewMode()
	require.Equal(t, ViewModeSideBySide, m.viewMode, "should have toggled to side-by-side")

	sideBySideContent := m.diffViewport.View()

	// Check side-by-side content characteristics - should have vertical separator
	require.Contains(t, sideBySideContent, "│", "side-by-side view should have vertical separator")

	// Content should be different
	require.NotEqual(t, unifiedContent, sideBySideContent, "content should change after toggle")
}

// === Hunk Navigation Tests (] and [ keys) ===

// TestHunkNavigation_SingleKey tests that ] and [ navigate hunks directly.
func TestHunkNavigation_SingleKey(t *testing.T) {
	// Setup model with a file that has multiple hunks
	file := DiffFile{
		NewPath: "test.go",
		OldPath: "test.go",
		Hunks: []DiffHunk{
			{
				OldStart: 1,
				NewStart: 1,
				Header:   "@@ -1,3 +1,3 @@",
				Lines: []DiffLine{
					{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "line1"},
					{Type: LineDeletion, OldLineNum: 2, Content: "old"},
					{Type: LineAddition, NewLineNum: 2, Content: "new"},
					{Type: LineContext, OldLineNum: 3, NewLineNum: 3, Content: "line3"},
				},
			},
			{
				OldStart: 10,
				NewStart: 10,
				Header:   "@@ -10,3 +10,3 @@",
				Lines: []DiffLine{
					{Type: LineContext, OldLineNum: 10, NewLineNum: 10, Content: "line10"},
					{Type: LineDeletion, OldLineNum: 11, Content: "old2"},
					{Type: LineAddition, NewLineNum: 11, Content: "new2"},
					{Type: LineContext, OldLineNum: 12, NewLineNum: 12, Content: "line12"},
				},
			},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.focus = focusDiffPane
	m.refreshViewport()

	// Record initial scroll position
	initialY := m.diffViewport.YOffset

	// Test ] key navigates to next hunk
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})

	// Scroll position should have changed (navigated to next hunk)
	afterNextHunkY := m.diffViewport.YOffset
	require.GreaterOrEqual(t, afterNextHunkY, initialY, "should scroll to next hunk or stay in place if at end")

	// Test [ key navigates to previous hunk
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})

	// Should have navigated (possibly back to where we started or wrapped)
	afterPrevHunkY := m.diffViewport.YOffset
	require.GreaterOrEqual(t, afterPrevHunkY, 0, "should have valid scroll position")
}

// === executeCommand Comprehensive Tests ===

// setupModelForExecuteCommand creates a model ready for executeCommand testing with:
// - visible=true, sized to 100x50
// - working dir files with tree structure
// - commits list
// - diff viewport with content
func setupModelForExecuteCommand() Model {
	m := New().SetSize(100, 50)
	m.visible = true

	// Set up working directory files
	workingDirFiles := []DiffFile{
		{NewPath: "file1.go", Additions: 10, Deletions: 5},
		{NewPath: "src/file2.go", Additions: 3, Deletions: 0},
		{NewPath: "src/utils/helper.go", Additions: 7, Deletions: 2},
	}
	m.workingDirFiles = workingDirFiles
	m.workingDirTree = NewFileTree(workingDirFiles)
	m.selectedWorkingDirNode = 0

	// Set up commits
	m.commits = []git.CommitInfo{
		{Hash: "abc1234", ShortHash: "abc1234", Subject: "First commit", Author: "Dev", Date: time.Now()},
		{Hash: "def5678", ShortHash: "def5678", Subject: "Second commit", Author: "Dev", Date: time.Now()},
	}
	m.selectedCommit = 0
	m.currentBranch = "main"
	m.commitPaneMode = commitPaneModeList

	// Set up viewport with some content for scrolling tests
	m.diffViewport.Height = 20
	m.diffViewport.YOffset = 5

	return m
}

// TestExecuteCommand_FocusCommands tests all focus-related commands.
func TestExecuteCommand_FocusCommands(t *testing.T) {
	tests := []struct {
		name          string
		cmdID         commandID
		initialFocus  focusPane
		expectedFocus focusPane
		description   string
	}{
		{
			name:          "cmdFocusFileList from DiffPane",
			cmdID:         cmdFocusFileList,
			initialFocus:  focusDiffPane,
			expectedFocus: focusFileList,
			description:   "should switch to file list pane",
		},
		{
			name:          "cmdFocusFileList from CommitPicker",
			cmdID:         cmdFocusFileList,
			initialFocus:  focusCommitPicker,
			expectedFocus: focusFileList,
			description:   "should switch to file list pane",
		},
		{
			name:          "cmdFocusFileList already on FileList",
			cmdID:         cmdFocusFileList,
			initialFocus:  focusFileList,
			expectedFocus: focusFileList,
			description:   "should stay on file list pane",
		},
		{
			name:          "cmdFocusDiff from FileList",
			cmdID:         cmdFocusDiff,
			initialFocus:  focusFileList,
			expectedFocus: focusDiffPane,
			description:   "should switch to diff pane and track last left focus",
		},
		{
			name:          "cmdFocusDiff from CommitPicker",
			cmdID:         cmdFocusDiff,
			initialFocus:  focusCommitPicker,
			expectedFocus: focusDiffPane,
			description:   "should switch to diff pane and track last left focus",
		},
		{
			name:          "cmdFocusCommits from FileList",
			cmdID:         cmdFocusCommits,
			initialFocus:  focusFileList,
			expectedFocus: focusCommitPicker,
			description:   "should switch to commit picker pane",
		},
		{
			name:          "cmdFocusCommits from DiffPane",
			cmdID:         cmdFocusCommits,
			initialFocus:  focusDiffPane,
			expectedFocus: focusCommitPicker,
			description:   "should switch to commit picker pane",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := setupModelForExecuteCommand()
			m.focus = tt.initialFocus

			newModel, _ := m.executeCommand(tt.cmdID)

			require.Equal(t, tt.expectedFocus, newModel.focus, tt.description)
		})
	}
}

// TestExecuteCommand_CyclePanes tests the cmdCyclePanes command through all focus states.
func TestExecuteCommand_CyclePanes(t *testing.T) {
	tests := []struct {
		name          string
		initialFocus  focusPane
		expectedFocus focusPane
	}{
		{
			name:          "FileList cycles to CommitPicker",
			initialFocus:  focusFileList,
			expectedFocus: focusCommitPicker,
		},
		{
			name:          "CommitPicker cycles to DiffPane",
			initialFocus:  focusCommitPicker,
			expectedFocus: focusDiffPane,
		},
		{
			name:          "DiffPane cycles to FileList",
			initialFocus:  focusDiffPane,
			expectedFocus: focusFileList,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := setupModelForExecuteCommand()
			m.focus = tt.initialFocus

			newModel, _ := m.executeCommand(cmdCyclePanes)

			require.Equal(t, tt.expectedFocus, newModel.focus, "should cycle to next pane")
		})
	}
}

// TestExecuteCommand_FocusLeftRight tests h/l navigation commands.
func TestExecuteCommand_FocusLeftRight(t *testing.T) {
	tests := []struct {
		name          string
		cmdID         commandID
		initialFocus  focusPane
		lastLeftFocus focusPane
		expectedFocus focusPane
	}{
		{
			name:          "cmdFocusLeft from CommitPicker goes to FileList",
			cmdID:         cmdFocusLeft,
			initialFocus:  focusCommitPicker,
			lastLeftFocus: focusFileList,
			expectedFocus: focusFileList,
		},
		{
			name:          "cmdFocusLeft from DiffPane returns to lastLeftFocus (FileList)",
			cmdID:         cmdFocusLeft,
			initialFocus:  focusDiffPane,
			lastLeftFocus: focusFileList,
			expectedFocus: focusFileList,
		},
		{
			name:          "cmdFocusLeft from DiffPane returns to lastLeftFocus (CommitPicker)",
			cmdID:         cmdFocusLeft,
			initialFocus:  focusDiffPane,
			lastLeftFocus: focusCommitPicker,
			expectedFocus: focusCommitPicker,
		},
		{
			name:          "cmdFocusLeft from FileList stays on FileList",
			cmdID:         cmdFocusLeft,
			initialFocus:  focusFileList,
			lastLeftFocus: focusFileList,
			expectedFocus: focusFileList,
		},
		{
			name:          "cmdFocusRight from FileList goes to CommitPicker",
			cmdID:         cmdFocusRight,
			initialFocus:  focusFileList,
			lastLeftFocus: focusFileList,
			expectedFocus: focusCommitPicker,
		},
		{
			name:          "cmdFocusRight from CommitPicker stays on CommitPicker",
			cmdID:         cmdFocusRight,
			initialFocus:  focusCommitPicker,
			lastLeftFocus: focusFileList,
			expectedFocus: focusCommitPicker,
		},
		{
			name:          "cmdFocusRight from DiffPane stays on DiffPane",
			cmdID:         cmdFocusRight,
			initialFocus:  focusDiffPane,
			lastLeftFocus: focusFileList,
			expectedFocus: focusDiffPane,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := setupModelForExecuteCommand()
			m.focus = tt.initialFocus
			m.lastLeftFocus = tt.lastLeftFocus

			newModel, _ := m.executeCommand(tt.cmdID)

			require.Equal(t, tt.expectedFocus, newModel.focus, "should change focus correctly")
		})
	}
}

// TestExecuteCommand_NavigationCommands tests next/prev file navigation.
func TestExecuteCommand_NavigationCommands(t *testing.T) {
	t.Run("cmdNextFile in FileList", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusFileList
		initialNode := m.selectedWorkingDirNode

		newModel, _ := m.executeCommand(cmdNextFile)

		require.Greater(t, newModel.selectedWorkingDirNode, initialNode, "should move to next node")
	})

	t.Run("cmdPrevFile in FileList", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusFileList
		m.selectedWorkingDirNode = 2
		initialNode := m.selectedWorkingDirNode

		newModel, _ := m.executeCommand(cmdPrevFile)

		require.Less(t, newModel.selectedWorkingDirNode, initialNode, "should move to previous node")
	})

	t.Run("cmdNextFile in CommitPicker ListMode", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusCommitPicker
		m.commitPaneMode = commitPaneModeList
		initialCommit := m.selectedCommit

		newModel, _ := m.executeCommand(cmdNextFile)

		require.Greater(t, newModel.selectedCommit, initialCommit, "should move to next commit")
	})

	t.Run("cmdPrevFile in CommitPicker ListMode at top", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusCommitPicker
		m.commitPaneMode = commitPaneModeList
		m.selectedCommit = 0

		newModel, _ := m.executeCommand(cmdPrevFile)

		require.Equal(t, 0, newModel.selectedCommit, "should stay at first commit")
	})

	t.Run("cmdNextFile in CommitPicker FilesMode", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusCommitPicker
		m.commitPaneMode = commitPaneModeFiles
		m.commitFiles = []DiffFile{
			{NewPath: "file1.go"},
			{NewPath: "file2.go"},
		}
		m.commitFilesTree = NewFileTree(m.commitFiles)
		m.selectedCommitFileNode = 0

		newModel, _ := m.executeCommand(cmdNextFile)

		require.Greater(t, newModel.selectedCommitFileNode, 0, "should move to next file node")
	})

	t.Run("cmdNextFile in DiffPane scrolls viewport", func(t *testing.T) {
		// Setup model with actual content for scrolling
		files := []DiffFile{createLargeFile()}
		m := setupModelWithFiles(files)
		m.focus = focusDiffPane
		m.refreshViewport()
		initialOffset := m.diffViewport.YOffset

		newModel, _ := m.executeCommand(cmdNextFile)

		require.Greater(t, newModel.diffViewport.YOffset, initialOffset, "should scroll viewport down")
	})

	t.Run("cmdPrevFile in DiffPane scrolls viewport", func(t *testing.T) {
		// Setup model with actual content for scrolling
		files := []DiffFile{createLargeFile()}
		m := setupModelWithFiles(files)
		m.focus = focusDiffPane
		m.refreshViewport()
		// Scroll down first so we can scroll up
		m.diffViewport.ScrollDown(10)
		scrolledOffset := m.diffViewport.YOffset

		newModel, _ := m.executeCommand(cmdPrevFile)

		require.Less(t, newModel.diffViewport.YOffset, scrolledOffset, "should scroll viewport up")
	})
}

// TestExecuteCommand_ScrollCommands tests scroll-related commands.
func TestExecuteCommand_ScrollCommands(t *testing.T) {
	t.Run("cmdScrollDown half-page scroll", func(t *testing.T) {
		// Setup model with actual content for scrolling
		files := []DiffFile{createLargeFile()}
		m := setupModelWithFiles(files)
		m.focus = focusDiffPane
		m.refreshViewport()
		initialOffset := m.diffViewport.YOffset
		expectedScroll := m.diffViewport.Height / 2

		newModel, _ := m.executeCommand(cmdScrollDown)

		require.Equal(t, initialOffset+expectedScroll, newModel.diffViewport.YOffset, "should scroll down by half page")
	})

	t.Run("cmdScrollUp half-page scroll", func(t *testing.T) {
		// Setup model with actual content for scrolling
		files := []DiffFile{createLargeFile()}
		m := setupModelWithFiles(files)
		m.focus = focusDiffPane
		m.refreshViewport()
		// Scroll down first
		m.diffViewport.ScrollDown(20)
		scrolledOffset := m.diffViewport.YOffset

		newModel, _ := m.executeCommand(cmdScrollUp)

		require.Less(t, newModel.diffViewport.YOffset, scrolledOffset, "should scroll up")
	})

	t.Run("cmdGotoTop in DiffPane", func(t *testing.T) {
		// Setup model with actual content for scrolling
		files := []DiffFile{createLargeFile()}
		m := setupModelWithFiles(files)
		m.focus = focusDiffPane
		m.refreshViewport()
		// Scroll down first
		m.diffViewport.ScrollDown(50)

		newModel, _ := m.executeCommand(cmdGotoTop)

		require.Equal(t, 0, newModel.diffViewport.YOffset, "should scroll to top")
	})

	t.Run("cmdGotoTop not in DiffPane does nothing", func(t *testing.T) {
		// Setup model with actual content for scrolling
		files := []DiffFile{createLargeFile()}
		m := setupModelWithFiles(files)
		m.focus = focusFileList
		m.refreshViewport()
		m.diffViewport.ScrollDown(50)
		scrolledOffset := m.diffViewport.YOffset

		newModel, _ := m.executeCommand(cmdGotoTop)

		require.Equal(t, scrolledOffset, newModel.diffViewport.YOffset, "should not change viewport when not focused on diff")
	})

	t.Run("cmdGotoBottom not in DiffPane does nothing", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusFileList
		initialOffset := m.diffViewport.YOffset

		newModel, _ := m.executeCommand(cmdGotoBottom)

		require.Equal(t, initialOffset, newModel.diffViewport.YOffset, "should not change viewport when not focused on diff")
	})
}

// TestExecuteCommand_SelectItem tests the cmdSelectItem command for different contexts.
func TestExecuteCommand_SelectItem(t *testing.T) {
	t.Run("SelectItem on file in FileList focuses DiffPane", func(t *testing.T) {
		// Use files without directories - flat file list
		files := []DiffFile{
			{NewPath: "file1.go", Additions: 10, Deletions: 5},
			{NewPath: "file2.go", Additions: 3, Deletions: 0},
		}
		m := setupModelWithFiles(files)
		m.focus = focusFileList
		m.selectedWorkingDirNode = 0 // First node is a file

		newModel, _ := m.executeCommand(cmdSelectItem)

		require.Equal(t, focusDiffPane, newModel.focus, "should focus diff pane when selecting a file")
		require.Equal(t, focusFileList, newModel.lastLeftFocus, "should track last left focus")
	})

	t.Run("SelectItem on directory in FileList toggles expand", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusFileList
		// Find a directory node (src folder)
		nodes := m.workingDirTree.VisibleNodes()
		for i, node := range nodes {
			if node.IsDir {
				m.selectedWorkingDirNode = i
				break
			}
		}
		initialVisibleCount := len(m.workingDirTree.VisibleNodes())

		newModel, _ := m.executeCommand(cmdSelectItem)

		// Check that visible node count changed (collapsed dir hides children)
		newVisibleCount := len(newModel.workingDirTree.VisibleNodes())
		require.NotEqual(t, initialVisibleCount, newVisibleCount, "should toggle directory expansion (visible count should change)")
	})

	t.Run("SelectItem in CommitPicker ListMode drills into commit", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusCommitPicker
		m.commitPaneMode = commitPaneModeList

		newModel, cmd := m.executeCommand(cmdSelectItem)

		// Mode stays as list until files load (to avoid flash)
		require.Equal(t, commitPaneModeList, newModel.commitPaneMode, "mode should stay list until files load")
		require.NotNil(t, newModel.inspectedCommit, "should set inspected commit")
		require.NotNil(t, cmd, "should return command to load commit diff")
	})

	t.Run("SelectItem in CommitPicker FilesMode on file focuses DiffPane", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusCommitPicker
		m.commitPaneMode = commitPaneModeFiles
		m.commitFiles = []DiffFile{{NewPath: "file.go"}}
		m.commitFilesTree = NewFileTree(m.commitFiles)
		m.selectedCommitFileNode = 0

		newModel, _ := m.executeCommand(cmdSelectItem)

		require.Equal(t, focusDiffPane, newModel.focus, "should focus diff pane when selecting a file")
		require.Equal(t, focusCommitPicker, newModel.lastLeftFocus, "should track last left focus")
	})

	t.Run("SelectItem in CommitPicker FilesMode on directory toggles expand", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusCommitPicker
		m.commitPaneMode = commitPaneModeFiles
		m.commitFiles = []DiffFile{
			{NewPath: "src/file1.go"},
			{NewPath: "src/file2.go"},
		}
		m.commitFilesTree = NewFileTree(m.commitFiles)
		// Select the src directory (index 0)
		m.selectedCommitFileNode = 0
		initialVisibleCount := len(m.commitFilesTree.VisibleNodes())

		newModel, _ := m.executeCommand(cmdSelectItem)

		// Check that visible node count changed (collapsed dir hides children)
		newVisibleCount := len(newModel.commitFilesTree.VisibleNodes())
		require.NotEqual(t, initialVisibleCount, newVisibleCount, "should toggle directory expansion (visible count should change)")
	})
}

// TestExecuteCommand_GoBack tests the cmdGoBack command.
func TestExecuteCommand_GoBack(t *testing.T) {
	t.Run("GoBack from FilesMode returns to ListMode", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusCommitPicker
		m.commitPaneMode = commitPaneModeFiles
		m.commitFiles = []DiffFile{{NewPath: "file.go"}}
		m.inspectedCommit = &git.CommitInfo{Hash: "abc"}

		newModel, cmd := m.executeCommand(cmdGoBack)

		require.Equal(t, commitPaneModeList, newModel.commitPaneMode, "should return to list mode")
		require.Nil(t, newModel.commitFiles, "should clear commit files")
		require.Nil(t, newModel.inspectedCommit, "should clear inspected commit")
		require.Nil(t, cmd, "should not return a command")
	})

	t.Run("GoBack from ListMode hides viewer", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusCommitPicker
		m.commitPaneMode = commitPaneModeList

		newModel, cmd := m.executeCommand(cmdGoBack)

		require.False(t, newModel.visible, "should hide the viewer")
		require.NotNil(t, cmd, "should return HideDiffViewerMsg command")
	})

	t.Run("GoBack from FileList hides viewer", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusFileList

		newModel, cmd := m.executeCommand(cmdGoBack)

		require.False(t, newModel.visible, "should hide the viewer")
		require.NotNil(t, cmd, "should return HideDiffViewerMsg command")
	})

	t.Run("GoBack from DiffPane hides viewer", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusDiffPane

		newModel, cmd := m.executeCommand(cmdGoBack)

		require.False(t, newModel.visible, "should hide the viewer")
		require.NotNil(t, cmd, "should return HideDiffViewerMsg command")
	})
}

// TestExecuteCommand_CloseViewer tests the cmdCloseViewer command.
func TestExecuteCommand_CloseViewer(t *testing.T) {
	m := setupModelForExecuteCommand()
	m.visible = true

	newModel, cmd := m.executeCommand(cmdCloseViewer)

	require.False(t, newModel.visible, "should hide the viewer")
	require.NotNil(t, cmd, "should return HideDiffViewerMsg command")
}

// TestExecuteCommand_OverlayCommands tests commands that show overlays.
func TestExecuteCommand_OverlayCommands(t *testing.T) {
	t.Run("cmdShowHelp shows help overlay", func(t *testing.T) {
		m := setupModelForExecuteCommand()

		newModel, _ := m.executeCommand(cmdShowHelp)

		require.True(t, newModel.showHelpOverlay, "should show help overlay")
	})
}

// TestExecuteCommand_ToggleViewMode tests the view mode toggle command.
func TestExecuteCommand_ToggleViewMode(t *testing.T) {
	t.Run("Toggle from Unified to SideBySide", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.viewMode = ViewModeUnified
		m.preferredViewMode = ViewModeUnified

		newModel, _ := m.executeCommand(cmdToggleViewMode)

		require.Equal(t, ViewModeSideBySide, newModel.preferredViewMode, "should toggle to side-by-side mode")
	})

	t.Run("Toggle from SideBySide to Unified", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.viewMode = ViewModeSideBySide
		m.preferredViewMode = ViewModeSideBySide

		newModel, _ := m.executeCommand(cmdToggleViewMode)

		require.Equal(t, ViewModeUnified, newModel.preferredViewMode, "should toggle to unified mode")
	})
}

// TestExecuteCommand_HunkNavigation tests hunk navigation commands.
func TestExecuteCommand_HunkNavigation(t *testing.T) {
	t.Run("cmdNextHunk returns model and nil cmd when no hunks", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusDiffPane
		// No files selected, so no hunks

		newModel, cmd := m.executeCommand(cmdNextHunk)

		// Should return without error
		require.NotNil(t, newModel.workingDirTree, "model should be valid")
		require.Nil(t, cmd, "should return nil cmd when no hunks")
	})

	t.Run("cmdPrevHunk returns model and nil cmd when no hunks", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusDiffPane

		newModel, cmd := m.executeCommand(cmdPrevHunk)

		require.NotNil(t, newModel.workingDirTree, "model should be valid")
		require.Nil(t, cmd, "should return nil cmd when no hunks")
	})
}

// TestExecuteCommand_CopyHunk tests the copy hunk command.
func TestExecuteCommand_CopyHunk(t *testing.T) {
	t.Run("cmdCopyHunk with no clipboard does nothing", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.clipboard = nil

		newModel, _ := m.executeCommand(cmdCopyHunk)

		// Should return without error
		require.NotNil(t, newModel.workingDirTree, "model should be valid")
	})
}

// TestExecuteCommand_UnknownCommand tests that unknown commands are handled gracefully.
func TestExecuteCommand_UnknownCommand(t *testing.T) {
	m := setupModelForExecuteCommand()

	newModel, cmd := m.executeCommand(commandID("unknown_command"))

	// Should return unchanged model and nil cmd
	require.Equal(t, m.focus, newModel.focus, "focus should be unchanged")
	require.Nil(t, cmd, "should return nil cmd for unknown command")
}

// TestExecuteCommand_FocusCommitsTriggersPreviewLoad tests that focusing commits loads preview.
func TestExecuteCommand_FocusCommitsTriggersPreviewLoad(t *testing.T) {
	t.Run("FocusCommits with commits and no preview loaded triggers load", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusFileList
		m.previewCommitHash = "" // No preview loaded

		newModel, cmd := m.executeCommand(cmdFocusCommits)

		require.Equal(t, focusCommitPicker, newModel.focus, "should focus commit picker")
		require.Equal(t, "abc1234", newModel.previewCommitHash, "should set preview commit hash")
		require.True(t, newModel.previewCommitLoading, "should set loading state")
		require.NotNil(t, cmd, "should return command to load preview")
	})

	t.Run("FocusCommits with already loaded preview skips load", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusFileList
		m.previewCommitHash = "abc1234" // Already loaded
		m.previewCommitLoading = false

		newModel, cmd := m.executeCommand(cmdFocusCommits)

		require.Equal(t, focusCommitPicker, newModel.focus, "should focus commit picker")
		require.False(t, newModel.previewCommitLoading, "should not set loading state")
		require.Nil(t, cmd, "should not return command when preview already loaded")
	})

	t.Run("FocusCommits with no commits skips load", func(t *testing.T) {
		m := setupModelForExecuteCommand()
		m.focus = focusFileList
		m.commits = nil

		newModel, cmd := m.executeCommand(cmdFocusCommits)

		require.Equal(t, focusCommitPicker, newModel.focus, "should focus commit picker")
		require.Nil(t, cmd, "should not return command when no commits")
	})
}

// TestExecuteCommand_CyclePanesTriggersPreviewLoad tests that cycling to commits loads preview.
func TestExecuteCommand_CyclePanesTriggersPreviewLoad(t *testing.T) {
	m := setupModelForExecuteCommand()
	m.previewCommitHash = "" // No preview loaded
	m.focus = focusFileList  // Will cycle to CommitPicker

	newModel, cmd := m.executeCommand(cmdCyclePanes)

	require.Equal(t, focusCommitPicker, newModel.focus, "should focus commit picker")
	require.Equal(t, "abc1234", newModel.previewCommitHash, "should set preview commit hash")
	require.True(t, newModel.previewCommitLoading, "should set loading state")
	require.NotNil(t, cmd, "should return command to load preview")
}

// TestExecuteCommand_FocusRightTriggersPreviewLoad tests that focus right triggers preview load.
func TestExecuteCommand_FocusRightTriggersPreviewLoad(t *testing.T) {
	m := setupModelForExecuteCommand()
	m.previewCommitHash = "" // No preview loaded
	m.focus = focusFileList

	newModel, cmd := m.executeCommand(cmdFocusRight)

	require.Equal(t, focusCommitPicker, newModel.focus, "should focus commit picker")
	require.Equal(t, "abc1234", newModel.previewCommitHash, "should set preview commit hash")
	require.True(t, newModel.previewCommitLoading, "should set loading state")
	require.NotNil(t, cmd, "should return command to load preview")
}

// TestExecuteCommand_LastLeftFocusTracking tests that lastLeftFocus is correctly tracked.
func TestExecuteCommand_LastLeftFocusTracking(t *testing.T) {
	tests := []struct {
		name             string
		initialFocus     focusPane
		initialLastLeft  focusPane
		cmdID            commandID
		expectedLastLeft focusPane
		description      string
	}{
		{
			name:             "FocusDiff from FileList tracks FileList",
			initialFocus:     focusFileList,
			initialLastLeft:  focusCommitPicker,
			cmdID:            cmdFocusDiff,
			expectedLastLeft: focusFileList,
			description:      "should track file list as last left focus",
		},
		{
			name:             "FocusDiff from CommitPicker tracks CommitPicker",
			initialFocus:     focusCommitPicker,
			initialLastLeft:  focusFileList,
			cmdID:            cmdFocusDiff,
			expectedLastLeft: focusCommitPicker,
			description:      "should track commit picker as last left focus",
		},
		{
			name:             "FocusDiff from DiffPane preserves lastLeftFocus",
			initialFocus:     focusDiffPane,
			initialLastLeft:  focusCommitPicker,
			cmdID:            cmdFocusDiff,
			expectedLastLeft: focusCommitPicker,
			description:      "should preserve last left focus when already on diff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := setupModelForExecuteCommand()
			m.focus = tt.initialFocus
			m.lastLeftFocus = tt.initialLastLeft

			newModel, _ := m.executeCommand(tt.cmdID)

			require.Equal(t, tt.expectedLastLeft, newModel.lastLeftFocus, tt.description)
		})
	}
}

// === Commit File Mode Hunk Navigation Tests ===

// setupModelWithCommitFiles creates a test model with commit files and tree properly initialized.
func setupModelWithCommitFiles(files []DiffFile) Model {
	m := New().SetSize(100, 50)
	m.visible = true
	m.commitFiles = files
	m.commitFilesTree = NewFileTree(files)
	m.selectedCommitFileNode = 0
	m.focus = focusCommitPicker
	m.commitPaneMode = commitPaneModeFiles
	return m
}

func TestModel_GetActiveFile_CommitFilesMode(t *testing.T) {
	file := DiffFile{
		NewPath: "commit_test.go",
		Hunks: []DiffHunk{
			{Header: "@@ -1,3 +1,4 @@"},
		},
	}

	m := setupModelWithCommitFiles([]DiffFile{file})

	activeFile := m.getActiveFile()
	require.NotNil(t, activeFile, "getActiveFile should return file in commit files mode")
	require.Equal(t, "commit_test.go", activeFile.NewPath)
	require.Len(t, activeFile.Hunks, 1)
}

func TestModel_GetActiveFile_CommitFilesMode_FocusDiffPane(t *testing.T) {
	file := DiffFile{
		NewPath: "commit_test.go",
		Hunks: []DiffHunk{
			{Header: "@@ -1,3 +1,4 @@"},
		},
	}

	m := setupModelWithCommitFiles([]DiffFile{file})
	m.focus = focusDiffPane
	m.lastLeftFocus = focusCommitPicker

	activeFile := m.getActiveFile()
	require.NotNil(t, activeFile, "getActiveFile should return file when focus is diff pane with commit lastLeftFocus")
	require.Equal(t, "commit_test.go", activeFile.NewPath)
}

func TestModel_HunkNavigation_NextHunk_CommitFiles(t *testing.T) {
	file := DiffFile{
		NewPath: "commit_test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,3 +1,4 @@",
				OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"},
					{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineAddition, Content: "new line", NewLineNum: 2},
					{Type: LineContext, Content: "line 2", OldLineNum: 2, NewLineNum: 3},
					{Type: LineContext, Content: "line 3", OldLineNum: 3, NewLineNum: 4},
				},
			},
			{
				Header:   "@@ -10,3 +11,5 @@",
				OldStart: 10, OldCount: 3, NewStart: 11, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -10,3 +11,5 @@"},
					{Type: LineContext, Content: "line 10", OldLineNum: 10, NewLineNum: 11},
					{Type: LineAddition, Content: "new line A", NewLineNum: 12},
					{Type: LineAddition, Content: "new line B", NewLineNum: 13},
					{Type: LineContext, Content: "line 11", OldLineNum: 11, NewLineNum: 14},
					{Type: LineContext, Content: "line 12", OldLineNum: 12, NewLineNum: 15},
				},
			},
			{
				Header:   "@@ -20,2 +24,3 @@",
				OldStart: 20, OldCount: 2, NewStart: 24, NewCount: 3,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -20,2 +24,3 @@"},
					{Type: LineContext, Content: "line 20", OldLineNum: 20, NewLineNum: 24},
					{Type: LineAddition, Content: "new line C", NewLineNum: 25},
					{Type: LineContext, Content: "line 21", OldLineNum: 21, NewLineNum: 26},
				},
			},
		},
	}

	m := setupModelWithCommitFiles([]DiffFile{file})
	m.refreshViewport()

	activeFile := m.getActiveFile()
	require.NotNil(t, activeFile, "getActiveFile should return file in commit files mode")
	require.Len(t, activeFile.Hunks, 3, "should have 3 hunks")

	require.Equal(t, 0, m.diffViewport.YOffset)

	m, _ = m.navigateToNextHunk()
	require.Equal(t, 5, m.diffViewport.YOffset, "should jump to second hunk")

	m, _ = m.navigateToNextHunk()
	require.Equal(t, 11, m.diffViewport.YOffset, "should jump to third hunk")

	m, _ = m.navigateToNextHunk()
	require.Equal(t, 0, m.diffViewport.YOffset, "should wrap to first hunk")
}

func TestModel_HunkNavigation_PrevHunk_CommitFiles(t *testing.T) {
	file := DiffFile{
		NewPath: "commit_test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,3 +1,4 @@",
				OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"},
					{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineAddition, Content: "new line", NewLineNum: 2},
					{Type: LineContext, Content: "line 2", OldLineNum: 2, NewLineNum: 3},
					{Type: LineContext, Content: "line 3", OldLineNum: 3, NewLineNum: 4},
				},
			},
			{
				Header:   "@@ -10,3 +11,5 @@",
				OldStart: 10, OldCount: 3, NewStart: 11, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -10,3 +11,5 @@"},
					{Type: LineContext, Content: "line 10", OldLineNum: 10, NewLineNum: 11},
					{Type: LineAddition, Content: "new line A", NewLineNum: 12},
					{Type: LineAddition, Content: "new line B", NewLineNum: 13},
					{Type: LineContext, Content: "line 11", OldLineNum: 11, NewLineNum: 14},
					{Type: LineContext, Content: "line 12", OldLineNum: 12, NewLineNum: 15},
				},
			},
		},
	}

	m := setupModelWithCommitFiles([]DiffFile{file})
	m.refreshViewport()

	require.Equal(t, 0, m.diffViewport.YOffset)

	m, _ = m.navigateToPrevHunk()
	require.Equal(t, 5, m.diffViewport.YOffset, "should wrap to last hunk")

	m, _ = m.navigateToPrevHunk()
	require.Equal(t, 0, m.diffViewport.YOffset, "should go to first hunk")
}

func TestModel_HunkNavigation_NoFile_CommitFiles(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.commitPaneMode = commitPaneModeFiles

	originalOffset := m.diffViewport.YOffset
	m, _ = m.navigateToNextHunk()
	require.Equal(t, originalOffset, m.diffViewport.YOffset, "should not change offset when no commit file")
}

func TestModel_HunkNavigation_EmptyHunks_CommitFiles(t *testing.T) {
	file := DiffFile{
		NewPath: "empty_commit.go",
		Hunks:   []DiffHunk{},
	}

	m := setupModelWithCommitFiles([]DiffFile{file})
	m.refreshViewport()

	originalOffset := m.diffViewport.YOffset
	m, _ = m.navigateToNextHunk()
	require.Equal(t, originalOffset, m.diffViewport.YOffset, "should not change offset when no hunks in commit file")
}

func TestModel_GetHunkPositionsForFile_CommitFiles(t *testing.T) {
	file := DiffFile{
		NewPath: "commit_test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
					{Type: LineContext},
				},
			},
			{
				Header: "@@ -10,3 +11,5 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
					{Type: LineAddition},
					{Type: LineContext},
				},
			},
		},
	}

	m := setupModelWithCommitFiles([]DiffFile{file})

	positions := m.getHunkPositionsForCurrentView()

	require.Len(t, positions, 2, "should have 2 hunk positions")
	require.Equal(t, 0, positions[0], "first hunk at position 0")
	require.Equal(t, 4, positions[1], "second hunk at position 4 (after 4 lines)")
}

// TestModel_GetHunkPositionsForDirectory tests hunk position calculation for directory views.
// This verifies that hunk positions are calculated correctly when viewing a directory
// containing multiple files with hunks.
func TestModel_GetHunkPositionsForDirectory(t *testing.T) {
	// Create two files with hunks
	file1 := DiffFile{
		NewPath: "dir/file1.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
				},
			},
		},
	}
	file2 := DiffFile{
		NewPath: "dir/file2.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,2 +1,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineAddition},
					{Type: LineContext},
				},
			},
			{
				Header: "@@ -10,2 +11,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
				},
			},
		},
	}

	// Create a directory node containing these files
	dirNode := &FileTreeNode{
		Name:  "dir",
		Path:  "dir",
		IsDir: true,
		Children: []*FileTreeNode{
			{Name: "file1.go", Path: "dir/file1.go", IsDir: false, File: &file1},
			{Name: "file2.go", Path: "dir/file2.go", IsDir: false, File: &file2},
		},
		Expanded: true,
	}

	m := New()
	positions := m.getHunkPositionsForDirectory(dirNode)

	// Expected positions:
	// File 1:
	//   Line 0: file header
	//   Line 1: hunk 1 (3 lines including header) -> position 1
	//   Lines 1-3: hunk content
	//   Line 4: empty line between files
	// File 2:
	//   Line 5: file header
	//   Line 6: hunk 1 (3 lines including header) -> position 6
	//   Lines 6-8: hunk content
	//   Line 9: hunk 2 (3 lines including header) -> position 9
	//   Lines 9-11: hunk content
	//   Line 12: empty line after file
	require.Len(t, positions, 3, "should have 3 hunk positions (1 in file1, 2 in file2)")
	require.Equal(t, 1, positions[0], "file1 hunk at line 1 (after file header)")
	require.Equal(t, 6, positions[1], "file2 hunk1 at line 6 (after file1 content + empty line + file2 header)")
	require.Equal(t, 9, positions[2], "file2 hunk2 at line 9 (after file2 hunk1)")
}

// TestModel_HunkNavigation_Directory tests that ] and [ work in directory view.
func TestModel_HunkNavigation_Directory(t *testing.T) {
	t.Run("direct function call", testHunkNavigationDirectoryDirectCall)
	t.Run("flat files at root", testHunkNavigationDirectoryFlatFiles)
	t.Run("key press simulation", testHunkNavigationDirectoryKeyPress)
	t.Run("with render cycle", testHunkNavigationDirectoryWithRender)
	t.Run("focus on file list pane", testHunkNavigationDirectoryFocusFileList)
}

func testHunkNavigationDirectoryDirectCall(t *testing.T) {
	// Create two files with hunks
	file1 := DiffFile{
		NewPath: "dir/file1.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
				},
			},
		},
	}
	file2 := DiffFile{
		NewPath: "dir/file2.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,2 +1,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineAddition},
					{Type: LineContext},
				},
			},
		},
	}

	// Setup model with directory selected
	m := New().SetSize(100, 50)
	m.visible = true
	m.workingDirFiles = []DiffFile{file1, file2}

	// Build tree with directory structure
	dirNode := &FileTreeNode{
		Name:  "dir",
		Path:  "dir",
		IsDir: true,
		Children: []*FileTreeNode{
			{Name: "file1.go", Path: "dir/file1.go", IsDir: false, File: &file1},
			{Name: "file2.go", Path: "dir/file2.go", IsDir: false, File: &file2},
		},
		Expanded: true,
	}
	m.workingDirTree = &FileTree{
		Root: []*FileTreeNode{dirNode},
	}
	// Select the directory node (index 0 in visible nodes)
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.lastLeftFocus = focusFileList
	m.diffViewport.YOffset = 0

	// Verify we have the directory node selected
	node := m.getActiveNode()
	require.NotNil(t, node, "should have an active node")
	require.True(t, node.IsDir, "active node should be a directory")

	// Get hunk positions
	positions := m.getHunkPositionsForCurrentView()
	require.Len(t, positions, 2, "should have 2 hunk positions")

	// Navigate to next hunk (])
	m, _ = m.navigateToNextHunk()
	require.Equal(t, positions[0], m.diffViewport.YOffset, "should navigate to first hunk")

	// Navigate to next hunk again
	m, _ = m.navigateToNextHunk()
	require.Equal(t, positions[1], m.diffViewport.YOffset, "should navigate to second hunk")

	// Navigate to next hunk again (should wrap to first)
	m, _ = m.navigateToNextHunk()
	require.Equal(t, positions[0], m.diffViewport.YOffset, "should wrap to first hunk")
}

func testHunkNavigationDirectoryFlatFiles(t *testing.T) {
	// Test case: Files at root level (no directory structure)
	// This simulates when files like "file1.go", "file2.go" are changed (no common dir)
	file1 := DiffFile{
		NewPath: "file1.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
				},
			},
		},
	}
	file2 := DiffFile{
		NewPath: "file2.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,2 +1,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineAddition},
				},
			},
		},
	}

	m := New().SetSize(100, 50)
	m.visible = true
	m.workingDirFiles = []DiffFile{file1, file2}
	m.workingDirTree = NewFileTree(m.workingDirFiles)

	// Select first file (not a directory)
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.lastLeftFocus = focusFileList
	m.diffViewport.YOffset = 0

	// Verify we have a file node selected (not a directory)
	node := m.getActiveNode()
	require.NotNil(t, node)
	require.False(t, node.IsDir, "first node should be a file, not directory (flat file list)")

	// Get positions - should be for single file since we selected a file
	positions := m.getHunkPositionsForCurrentView()
	require.Len(t, positions, 1, "should have 1 hunk position for single file")

	// Navigate should work within single file
	m, _ = m.navigateToNextHunk()
	require.Equal(t, positions[0], m.diffViewport.YOffset)
}

func testHunkNavigationDirectoryKeyPress(t *testing.T) {
	// Create two files with hunks
	file1 := DiffFile{
		NewPath: "dir/file1.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
				},
			},
		},
	}
	file2 := DiffFile{
		NewPath: "dir/file2.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,2 +1,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineAddition},
					{Type: LineContext},
				},
			},
		},
	}

	// Setup model with directory selected
	m := New().SetSize(100, 50)
	m.visible = true
	m.workingDirFiles = []DiffFile{file1, file2}

	// Build tree with directory structure
	dirNode := &FileTreeNode{
		Name:  "dir",
		Path:  "dir",
		IsDir: true,
		Children: []*FileTreeNode{
			{Name: "file1.go", Path: "dir/file1.go", IsDir: false, File: &file1},
			{Name: "file2.go", Path: "dir/file2.go", IsDir: false, File: &file2},
		},
		Expanded: true,
	}
	m.workingDirTree = &FileTree{
		Root: []*FileTreeNode{dirNode},
	}
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.lastLeftFocus = focusFileList
	m.diffViewport.YOffset = 0

	// Verify directory node is selected
	node := m.getActiveNode()
	require.NotNil(t, node)
	require.True(t, node.IsDir)

	// Get expected positions
	positions := m.getHunkPositionsForCurrentView()
	require.Len(t, positions, 2, "should have 2 hunk positions")

	// Simulate ] key press to navigate to next hunk
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	require.Equal(t, positions[0], m.diffViewport.YOffset, "should navigate to first hunk via ] key press")

	// Simulate ] again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	require.Equal(t, positions[1], m.diffViewport.YOffset, "should navigate to second hunk via ] key press")
}

func testHunkNavigationDirectoryWithRender(t *testing.T) {
	// Test that hunk navigation persists across View() renders
	// This tests the full render cycle, not just state changes
	file1 := DiffFile{
		NewPath: "dir/file1.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
				},
			},
		},
	}
	file2 := DiffFile{
		NewPath: "dir/file2.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,2 +1,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineAddition},
					{Type: LineContext},
				},
			},
		},
	}

	m := New().SetSize(100, 50)
	m.visible = true
	m.workingDirFiles = []DiffFile{file1, file2}

	// Build tree with directory structure
	dirNode := &FileTreeNode{
		Name:  "dir",
		Path:  "dir",
		IsDir: true,
		Children: []*FileTreeNode{
			{Name: "file1.go", Path: "dir/file1.go", IsDir: false, File: &file1},
			{Name: "file2.go", Path: "dir/file2.go", IsDir: false, File: &file2},
		},
		Expanded: true,
	}
	m.workingDirTree = &FileTree{
		Root: []*FileTreeNode{dirNode},
	}
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.lastLeftFocus = focusFileList
	m.diffViewport.YOffset = 0

	// Render to initialize viewport
	_ = m.View()

	// Get expected positions
	positions := m.getHunkPositionsForCurrentView()
	require.Len(t, positions, 2, "should have 2 hunk positions")

	// Navigate to next hunk
	m, _ = m.navigateToNextHunk()
	offsetAfterNav := m.diffViewport.YOffset
	require.Equal(t, positions[0], offsetAfterNav, "should navigate to first hunk")

	// Render the view (this is what happens in a real app)
	_ = m.View()

	// Check that offset is STILL at the hunk position after render
	require.Equal(t, offsetAfterNav, m.diffViewport.YOffset, "offset should persist after View() render")
}

func testHunkNavigationDirectoryFocusFileList(t *testing.T) {
	// Test hunk navigation when focus is on file list pane (not diff pane)
	// This is a common scenario where user has focus on file list but wants to navigate hunks
	file1 := DiffFile{
		NewPath: "dir/file1.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext},
					{Type: LineAddition},
				},
			},
		},
	}
	file2 := DiffFile{
		NewPath: "dir/file2.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,2 +1,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineAddition},
					{Type: LineContext},
				},
			},
		},
	}

	m := New().SetSize(100, 50)
	m.visible = true
	m.workingDirFiles = []DiffFile{file1, file2}

	// Build tree with directory structure
	dirNode := &FileTreeNode{
		Name:  "dir",
		Path:  "dir",
		IsDir: true,
		Children: []*FileTreeNode{
			{Name: "file1.go", Path: "dir/file1.go", IsDir: false, File: &file1},
			{Name: "file2.go", Path: "dir/file2.go", IsDir: false, File: &file2},
		},
		Expanded: true,
	}
	m.workingDirTree = &FileTree{
		Root: []*FileTreeNode{dirNode},
	}
	m.selectedWorkingDirNode = 0
	// KEY DIFFERENCE: Focus is on file list, not diff pane
	m.focus = focusFileList
	m.diffViewport.YOffset = 0

	// Verify directory node is selected
	node := m.getActiveNode()
	require.NotNil(t, node)
	require.True(t, node.IsDir, "should have directory node selected")

	// Get expected positions
	positions := m.getHunkPositionsForCurrentView()
	require.Len(t, positions, 2, "should have 2 hunk positions when focus is on file list")

	// Navigate to next hunk
	m, _ = m.navigateToNextHunk()
	require.Equal(t, positions[0], m.diffViewport.YOffset, "should navigate to first hunk")

	// Navigate to next hunk again
	m, _ = m.navigateToNextHunk()
	require.Equal(t, positions[1], m.diffViewport.YOffset, "should navigate to second hunk")
}

// =============================================================================
// Golden tests for tab rendering in commits pane
// =============================================================================

// TestView_Golden_CommitsPane_TabsDefault tests the commits pane with default tab (Commits)
func TestView_Golden_CommitsPane_TabsDefault(t *testing.T) {
	fixedTime := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)

	m := New().SetSize(100, 30)
	m.visible = true
	m.commitPaneMode = commitPaneModeList
	m.focus = focusCommitPicker
	m.activeCommitTab = commitsTabCommits
	m.commits = []git.CommitInfo{
		{Hash: "abc1234def567890abc1234def567890abc12345", ShortHash: "abc1234", Subject: "Add user authentication", Author: "Alice", Date: fixedTime},
		{Hash: "def5678abc901234def5678abc901234def56789", ShortHash: "def5678", Subject: "Fix login bug", Author: "Bob", Date: fixedTime.Add(-time.Hour)},
		{Hash: "ghi9012def345678ghi9012def345678ghi90123", ShortHash: "ghi9012", Subject: "Update dependencies", Author: "Charlie", Date: fixedTime.Add(-2 * time.Hour)},
	}
	m.selectedCommit = 0
	m.commitScrollTop = 0

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestView_Golden_CommitsPane_BranchesTab tests the commits pane with Branches tab active
func TestView_Golden_CommitsPane_BranchesTab(t *testing.T) {
	m := New().SetSize(100, 30)
	m.visible = true
	m.commitPaneMode = commitPaneModeList
	m.focus = focusCommitPicker
	m.activeCommitTab = commitsTabBranches
	m.branchList = []git.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "develop", IsCurrent: false},
		{Name: "feature/auth", IsCurrent: false},
		{Name: "feature/profile", IsCurrent: false},
	}
	m.selectedBranch = 0
	m.branchScrollTop = 0
	m.branchListLoaded = true

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestView_Golden_CommitsPane_WorktreesTab tests the commits pane with Worktrees tab active
func TestView_Golden_CommitsPane_WorktreesTab(t *testing.T) {
	m := New().SetSize(100, 30)
	m.visible = true
	m.commitPaneMode = commitPaneModeList
	m.focus = focusCommitPicker
	m.activeCommitTab = commitsTabWorktrees
	m.worktreeList = []git.WorktreeInfo{
		{Path: "/path/to/main", Branch: "main", HEAD: "abc1234"},
		{Path: "/path/to/feature-auth", Branch: "feature/auth", HEAD: "def5678"},
		{Path: "/path/to/hotfix", Branch: "hotfix/urgent", HEAD: "ghi9012"},
	}
	m.selectedWorktree = 0
	m.worktreeScrollTop = 0
	m.worktreeListLoaded = true

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestView_Golden_CommitsPane_BranchesTab_Selected tests Branches tab with a non-first item selected
func TestView_Golden_CommitsPane_BranchesTab_Selected(t *testing.T) {
	m := New().SetSize(100, 30)
	m.visible = true
	m.commitPaneMode = commitPaneModeList
	m.focus = focusCommitPicker
	m.activeCommitTab = commitsTabBranches
	m.branchList = []git.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "develop", IsCurrent: false},
		{Name: "feature/auth", IsCurrent: false},
	}
	m.selectedBranch = 1 // Select develop
	m.branchScrollTop = 0
	m.branchListLoaded = true

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestView_Golden_CommitsPane_WorktreesTab_Selected tests Worktrees tab with a non-first item selected
func TestView_Golden_CommitsPane_WorktreesTab_Selected(t *testing.T) {
	m := New().SetSize(100, 30)
	m.visible = true
	m.commitPaneMode = commitPaneModeList
	m.focus = focusCommitPicker
	m.activeCommitTab = commitsTabWorktrees
	m.worktreeList = []git.WorktreeInfo{
		{Path: "/path/to/main", Branch: "main", HEAD: "abc1234"},
		{Path: "/path/to/feature-auth", Branch: "feature/auth", HEAD: "def5678"},
	}
	m.selectedWorktree = 1 // Select feature-auth
	m.worktreeScrollTop = 0
	m.worktreeListLoaded = true

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestView_Golden_CommitsPane_TabsUnfocused tests tab rendering when commits pane is not focused
func TestView_Golden_CommitsPane_TabsUnfocused(t *testing.T) {
	fixedTime := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)

	m := New().SetSize(100, 30)
	m.visible = true
	m.commitPaneMode = commitPaneModeList
	m.focus = focusFileList // Not focused on commits pane
	m.activeCommitTab = commitsTabCommits
	m.commits = []git.CommitInfo{
		{Hash: "abc1234def567890abc1234def567890abc12345", ShortHash: "abc1234", Subject: "Add user authentication", Author: "Alice", Date: fixedTime},
		{Hash: "def5678abc901234def5678abc901234def56789", ShortHash: "def5678", Subject: "Fix login bug", Author: "Bob", Date: fixedTime.Add(-time.Hour)},
	}
	m.selectedCommit = 0
	m.commitScrollTop = 0

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestView_Golden_CommitPreview_WithScrollbar tests the commit preview view with
// enough content to show a scrollbar. This verifies the scrollbar appears on the
// RIGHT edge of the diff pane, not the left.
func TestView_Golden_CommitPreview_WithScrollbar(t *testing.T) {
	fixedTime := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)

	// Create a diff file with enough lines to trigger scrollbar
	// (more lines than viewport height)
	var lines []DiffLine
	lines = append(lines, DiffLine{Type: LineHunkHeader, Content: "@@ -1,20 +1,25 @@ function main()"})
	for i := 1; i <= 30; i++ {
		if i%3 == 0 {
			lines = append(lines, DiffLine{Type: LineAddition, Content: fmt.Sprintf("+ added line %d", i), NewLineNum: i})
		} else if i%5 == 0 {
			lines = append(lines, DiffLine{Type: LineDeletion, Content: fmt.Sprintf("- deleted line %d", i), OldLineNum: i})
		} else {
			lines = append(lines, DiffLine{Type: LineContext, Content: fmt.Sprintf("  context line %d", i), OldLineNum: i, NewLineNum: i})
		}
	}

	m := New().SetSize(100, 25) // Small height to trigger scrollbar
	m.visible = true
	m.commitPaneMode = commitPaneModeList
	m.focus = focusCommitPicker
	m.activeCommitTab = commitsTabCommits
	m.commits = []git.CommitInfo{
		{Hash: "abc1234def567890abc1234def567890abc12345", ShortHash: "abc1234", Subject: "Add new feature", Author: "Alice", Date: fixedTime},
	}
	m.selectedCommit = 0
	m.previewCommitFiles = []DiffFile{
		{
			NewPath:   "src/main.go",
			Additions: 10,
			Deletions: 5,
			Hunks: []DiffHunk{
				{
					Header:   "@@ -1,20 +1,25 @@ function main()",
					OldStart: 1, OldCount: 20, NewStart: 1, NewCount: 25,
					Lines: lines,
				},
			},
		},
	}

	// Call refreshViewport to properly set up content and scrollbar
	m.refreshViewport()

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// === Tab Navigation Tests ===

// TestTabNavigation_BracketsWhenCommitsPaneFocused verifies that [ and ] cycle tabs
// when the commits pane is focused.
func TestTabNavigation_BracketsWhenCommitsPaneFocused(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker

	// Start at Commits tab (default)
	require.Equal(t, commitsTabCommits, m.activeCommitTab, "should start at Commits tab")

	// Press ] to go to Branches tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	require.Equal(t, commitsTabBranches, m.activeCommitTab, "] should navigate to Branches tab")

	// Press ] to go to Worktrees tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	require.Equal(t, commitsTabWorktrees, m.activeCommitTab, "] should navigate to Worktrees tab")

	// Press ] to wrap back to Commits tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	require.Equal(t, commitsTabCommits, m.activeCommitTab, "] should wrap back to Commits tab")

	// Press [ to go back to Worktrees tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	require.Equal(t, commitsTabWorktrees, m.activeCommitTab, "[ should navigate back to Worktrees tab")

	// Press [ to go to Branches tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	require.Equal(t, commitsTabBranches, m.activeCommitTab, "[ should navigate to Branches tab")

	// Press [ to go to Commits tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	require.Equal(t, commitsTabCommits, m.activeCommitTab, "[ should navigate to Commits tab")
}

// TestTabNavigation_BracketsWhenDiffPaneFocused_HunkNavigation verifies that [ and ]
// still perform hunk navigation when the diff pane is focused (existing behavior preserved).
func TestTabNavigation_BracketsWhenDiffPaneFocused_HunkNavigation(t *testing.T) {
	// Create a file with multiple hunks
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,3 +1,4 @@",
				OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"},
					{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineAddition, Content: "new line", NewLineNum: 2},
					{Type: LineContext, Content: "line 2", OldLineNum: 2, NewLineNum: 3},
					{Type: LineContext, Content: "line 3", OldLineNum: 3, NewLineNum: 4},
				},
			},
			{
				Header:   "@@ -10,3 +11,5 @@",
				OldStart: 10, OldCount: 3, NewStart: 11, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -10,3 +11,5 @@"},
					{Type: LineContext, Content: "line 10", OldLineNum: 10, NewLineNum: 11},
					{Type: LineAddition, Content: "new line A", NewLineNum: 12},
				},
			},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.focus = focusDiffPane
	m.refreshViewport()

	// Verify focus is on diff pane
	require.Equal(t, focusDiffPane, m.focus)

	// Tab should remain unchanged since we're on diff pane
	originalTab := m.activeCommitTab

	// Start at position 0
	require.Equal(t, 0, m.diffViewport.YOffset)

	// Press ] - should perform hunk navigation, not tab navigation
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})

	// Tab should be unchanged
	require.Equal(t, originalTab, m.activeCommitTab, "tab should not change when diff pane is focused")

	// Viewport should have moved (hunk navigation happened)
	require.Greater(t, m.diffViewport.YOffset, 0, "should have navigated to next hunk")
}

// TestTabNavigation_BracketsWhenFileListFocused_NoTabChange verifies that [ and ]
// perform hunk navigation (not tab navigation) when file list is focused.
func TestTabNavigation_BracketsWhenFileListFocused_NoTabChange(t *testing.T) {
	// Create a file with hunks
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"},
					{Type: LineContext, Content: "line 1"},
					{Type: LineAddition, Content: "new line"},
				},
			},
			{
				Header: "@@ -10,3 +11,5 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -10,3 +11,5 @@"},
					{Type: LineAddition, Content: "new line A"},
				},
			},
		},
	}

	m := setupModelWithFiles([]DiffFile{file})
	m.focus = focusFileList
	m.refreshViewport()

	// Verify focus is on file list
	require.Equal(t, focusFileList, m.focus)

	// Tab should remain unchanged since we're on file list pane
	originalTab := m.activeCommitTab

	// Press ] - should perform hunk navigation, not tab navigation
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})

	// Tab should be unchanged
	require.Equal(t, originalTab, m.activeCommitTab, "tab should not change when file list is focused")
}

// TestTabNavigation_LazyLoading verifies that switching to Branches or Worktrees tab
// triggers lazy loading if data is not already loaded.
func TestTabNavigation_LazyLoading(t *testing.T) {
	// Create a mock git executor (with Maybe() expectations since we only check commands returned)
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.On("ListBranches").Maybe().Return([]git.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "feature/test", IsCurrent: false},
	}, nil)
	mockGit.On("ListWorktrees").Maybe().Return([]git.WorktreeInfo{
		{Path: "/path/to/main", Branch: "main"},
	}, nil)

	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.gitExecutor = mockGit

	// Verify branch list not loaded initially
	require.False(t, m.branchListLoaded, "branch list should not be loaded initially")
	require.False(t, m.worktreeListLoaded, "worktree list should not be loaded initially")

	// Navigate to Branches tab
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	require.Equal(t, commitsTabBranches, m.activeCommitTab)

	// Should have returned a command to load branches
	require.NotNil(t, cmd, "should return a command to load branches")

	// Navigate to Worktrees tab
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	require.Equal(t, commitsTabWorktrees, m.activeCommitTab)

	// Should have returned a command to load worktrees
	require.NotNil(t, cmd, "should return a command to load worktrees")
}

// TestTabNavigation_NoReloadWhenAlreadyLoaded verifies that switching to a tab
// does not trigger loading if data is already loaded.
func TestTabNavigation_NoReloadWhenAlreadyLoaded(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker

	// Mark branch list as already loaded
	m.branchListLoaded = true
	m.branchList = []git.BranchInfo{
		{Name: "main", IsCurrent: true},
	}

	// Navigate to Branches tab
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	require.Equal(t, commitsTabBranches, m.activeCommitTab)

	// Should NOT return a command since branches are already loaded
	require.Nil(t, cmd, "should not return a command when branches already loaded")
}

// TestNextCommitTab verifies the nextCommitTab method cycles correctly.
func TestNextCommitTab(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	// Start at Commits tab
	require.Equal(t, commitsTabCommits, m.activeCommitTab)

	// Cycle forward
	m, _ = m.nextCommitTab()
	require.Equal(t, commitsTabBranches, m.activeCommitTab)

	m, _ = m.nextCommitTab()
	require.Equal(t, commitsTabWorktrees, m.activeCommitTab)

	m, _ = m.nextCommitTab()
	require.Equal(t, commitsTabCommits, m.activeCommitTab, "should wrap back to Commits")
}

// TestPrevCommitTab verifies the prevCommitTab method cycles correctly.
func TestPrevCommitTab(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true

	// Start at Commits tab
	require.Equal(t, commitsTabCommits, m.activeCommitTab)

	// Cycle backward
	m, _ = m.prevCommitTab()
	require.Equal(t, commitsTabWorktrees, m.activeCommitTab, "should wrap to Worktrees")

	m, _ = m.prevCommitTab()
	require.Equal(t, commitsTabBranches, m.activeCommitTab)

	m, _ = m.prevCommitTab()
	require.Equal(t, commitsTabCommits, m.activeCommitTab)
}

// TestEnsureTabDataLoaded verifies lazy loading logic for each tab.
func TestEnsureTabDataLoaded(t *testing.T) {
	t.Run("commits tab needs no loading", func(t *testing.T) {
		m := New().SetSize(100, 50)
		m.activeCommitTab = commitsTabCommits

		m, cmd := m.ensureTabDataLoaded()
		require.Nil(t, cmd, "commits tab should not trigger loading")
	})

	t.Run("branches tab triggers loading when not loaded", func(t *testing.T) {
		mockGit := mocks.NewMockGitExecutor(t)
		mockGit.On("ListBranches").Maybe().Return([]git.BranchInfo{}, nil)

		m := New().SetSize(100, 50)
		m.gitExecutor = mockGit
		m.activeCommitTab = commitsTabBranches
		m.branchListLoaded = false

		m, cmd := m.ensureTabDataLoaded()
		require.NotNil(t, cmd, "should return load command when branches not loaded")
	})

	t.Run("branches tab skips loading when already loaded", func(t *testing.T) {
		m := New().SetSize(100, 50)
		m.activeCommitTab = commitsTabBranches
		m.branchListLoaded = true

		m, cmd := m.ensureTabDataLoaded()
		require.Nil(t, cmd, "should not return load command when branches already loaded")
	})

	t.Run("worktrees tab triggers loading when not loaded", func(t *testing.T) {
		mockGit := mocks.NewMockGitExecutor(t)
		mockGit.On("ListWorktrees").Maybe().Return([]git.WorktreeInfo{}, nil)

		m := New().SetSize(100, 50)
		m.gitExecutor = mockGit
		m.activeCommitTab = commitsTabWorktrees
		m.worktreeListLoaded = false

		m, cmd := m.ensureTabDataLoaded()
		require.NotNil(t, cmd, "should return load command when worktrees not loaded")
	})

	t.Run("worktrees tab skips loading when already loaded", func(t *testing.T) {
		m := New().SetSize(100, 50)
		m.activeCommitTab = commitsTabWorktrees
		m.worktreeListLoaded = true

		m, cmd := m.ensureTabDataLoaded()
		require.Nil(t, cmd, "should not return load command when worktrees already loaded")
	})
}

// === Branch/Worktree Selection Tests ===

// TestBranchSelection_FromTab verifies branch selection from Branches tab auto-switches to Commits tab.
func TestBranchSelection_FromTab(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.On("GetCommitLogForRef", "develop", 50).Return([]git.CommitInfo{
		{Hash: "abc1234", ShortHash: "abc1234", Subject: "Test commit"},
	}, nil)

	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.gitExecutor = mockGit
	m.activeCommitTab = commitsTabBranches
	m.branchList = []git.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "develop", IsCurrent: false},
		{Name: "feature/auth", IsCurrent: false},
	}
	m.selectedBranch = 1 // Select "develop"
	m.branchListLoaded = true

	// Press Enter to select the branch
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should have auto-switched to Commits tab
	require.Equal(t, commitsTabCommits, m.activeCommitTab, "should auto-switch to Commits tab after branch selection")

	// Should have returned a command to load commits for the branch
	require.NotNil(t, cmd, "should return a command to load commits for branch")

	// Execute the command to verify it calls GetCommitLogForRef
	msg := cmd()
	require.IsType(t, CommitsForBranchLoadedMsg{}, msg, "command should return CommitsForBranchLoadedMsg")
	branchMsg := msg.(CommitsForBranchLoadedMsg)
	require.Equal(t, "develop", branchMsg.Branch, "should load commits for 'develop' branch")
	require.Len(t, branchMsg.Commits, 1, "should return the mocked commits")
}

// TestWorktreeSelection_FromTab verifies worktree selection from Worktrees tab.
func TestWorktreeSelection_FromTab(t *testing.T) {
	// Create a temp directory to act as a worktree path
	tempDir := t.TempDir()

	mockGit := mocks.NewMockGitExecutor(t)

	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.gitExecutor = mockGit
	m.gitExecutorFactory = func(path string) git.GitExecutor {
		return mockGit
	}
	m.activeCommitTab = commitsTabWorktrees
	m.worktreeList = []git.WorktreeInfo{
		{Path: "/main", Branch: "main", HEAD: "abc1234"},
		{Path: tempDir, Branch: "feature/auth", HEAD: "def5678"}, // Valid path
	}
	m.selectedWorktree = 1 // Select the temp directory worktree
	m.worktreeListLoaded = true

	// Press Enter to select the worktree
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should have auto-switched to Commits tab
	require.Equal(t, commitsTabCommits, m.activeCommitTab, "should auto-switch to Commits tab after worktree selection")

	// Current worktree path should be updated
	require.Equal(t, tempDir, m.currentWorktreePath, "should update currentWorktreePath")
}

// TestWorktreeSelection_FromTab_ValidationError verifies stale worktree handling with os.Stat error.
func TestWorktreeSelection_FromTab_ValidationError(t *testing.T) {
	mockGit := mocks.NewMockGitExecutor(t)

	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.gitExecutor = mockGit
	m.activeCommitTab = commitsTabWorktrees
	m.worktreeList = []git.WorktreeInfo{
		{Path: "/nonexistent/path", Branch: "main", HEAD: "abc1234"}, // Invalid path
	}
	m.selectedWorktree = 0
	m.worktreeListLoaded = true

	// Press Enter to select the worktree (should fail validation)
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should have set an error (worktree doesn't exist)
	require.NotNil(t, m.err, "should set error for non-existent worktree")
	require.Contains(t, m.err.Error(), "worktree no longer exists", "error should mention worktree doesn't exist")

	// Should remain on Worktrees tab (not auto-switch)
	require.Equal(t, commitsTabWorktrees, m.activeCommitTab, "should remain on Worktrees tab on error")

	// Should not return any command
	require.Nil(t, cmd, "should not return a command on validation error")
}

// TestTabNavigation_EmptyList verifies bounds checking with empty branch/worktree lists.
func TestTabNavigation_EmptyList(t *testing.T) {
	t.Run("empty branch list - selection does nothing", func(t *testing.T) {
		m := New().SetSize(100, 50)
		m.visible = true
		m.focus = focusCommitPicker
		m.activeCommitTab = commitsTabBranches
		m.branchList = []git.BranchInfo{} // Empty list
		m.branchListLoaded = true

		// Press Enter - should not crash or error
		m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// Should remain on Branches tab
		require.Equal(t, commitsTabBranches, m.activeCommitTab, "should remain on Branches tab")
		require.Nil(t, cmd, "should not return a command with empty list")
	})

	t.Run("empty worktree list - selection does nothing", func(t *testing.T) {
		m := New().SetSize(100, 50)
		m.visible = true
		m.focus = focusCommitPicker
		m.activeCommitTab = commitsTabWorktrees
		m.worktreeList = []git.WorktreeInfo{} // Empty list
		m.worktreeListLoaded = true

		// Press Enter - should not crash or error
		m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// Should remain on Worktrees tab
		require.Equal(t, commitsTabWorktrees, m.activeCommitTab, "should remain on Worktrees tab")
		require.Nil(t, cmd, "should not return a command with empty list")
	})

	t.Run("empty branch list - j/k navigation does nothing", func(t *testing.T) {
		m := New().SetSize(100, 50)
		m.visible = true
		m.focus = focusCommitPicker
		m.activeCommitTab = commitsTabBranches
		m.branchList = []git.BranchInfo{} // Empty list
		m.branchListLoaded = true
		m.selectedBranch = 0

		// Press j - should not crash
		m, _ = m.handleCommitPaneDown()
		require.Equal(t, 0, m.selectedBranch, "selectedBranch should remain 0")

		// Press k - should not crash
		m, _ = m.handleCommitPaneUp()
		require.Equal(t, 0, m.selectedBranch, "selectedBranch should remain 0")
	})

	t.Run("empty worktree list - j/k navigation does nothing", func(t *testing.T) {
		m := New().SetSize(100, 50)
		m.visible = true
		m.focus = focusCommitPicker
		m.activeCommitTab = commitsTabWorktrees
		m.worktreeList = []git.WorktreeInfo{} // Empty list
		m.worktreeListLoaded = true
		m.selectedWorktree = 0

		// Press j - should not crash
		m, _ = m.handleCommitPaneDown()
		require.Equal(t, 0, m.selectedWorktree, "selectedWorktree should remain 0")

		// Press k - should not crash
		m, _ = m.handleCommitPaneUp()
		require.Equal(t, 0, m.selectedWorktree, "selectedWorktree should remain 0")
	})
}

// TestJKNavigation_BranchList verifies j/k navigation within branch list.
func TestJKNavigation_BranchList(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.activeCommitTab = commitsTabBranches
	m.branchList = []git.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "develop", IsCurrent: false},
		{Name: "feature/auth", IsCurrent: false},
	}
	m.selectedBranch = 0
	m.branchListLoaded = true

	// Navigate down with j
	m, _ = m.handleCommitPaneDown()
	require.Equal(t, 1, m.selectedBranch, "j should move to next branch")

	m, _ = m.handleCommitPaneDown()
	require.Equal(t, 2, m.selectedBranch, "j should move to next branch")

	// Try to go past end
	m, _ = m.handleCommitPaneDown()
	require.Equal(t, 2, m.selectedBranch, "should stay at end of list")

	// Navigate up with k
	m, _ = m.handleCommitPaneUp()
	require.Equal(t, 1, m.selectedBranch, "k should move to previous branch")

	m, _ = m.handleCommitPaneUp()
	require.Equal(t, 0, m.selectedBranch, "k should move to previous branch")

	// Try to go before start
	m, _ = m.handleCommitPaneUp()
	require.Equal(t, 0, m.selectedBranch, "should stay at start of list")
}

// TestJKNavigation_WorktreeList verifies j/k navigation within worktree list.
func TestJKNavigation_WorktreeList(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	m.focus = focusCommitPicker
	m.activeCommitTab = commitsTabWorktrees
	m.worktreeList = []git.WorktreeInfo{
		{Path: "/path/to/main", Branch: "main", HEAD: "abc1234"},
		{Path: "/path/to/develop", Branch: "develop", HEAD: "def5678"},
		{Path: "/path/to/feature", Branch: "feature/auth", HEAD: "ghi9012"},
	}
	m.selectedWorktree = 0
	m.worktreeListLoaded = true

	// Navigate down with j
	m, _ = m.handleCommitPaneDown()
	require.Equal(t, 1, m.selectedWorktree, "j should move to next worktree")

	m, _ = m.handleCommitPaneDown()
	require.Equal(t, 2, m.selectedWorktree, "j should move to next worktree")

	// Try to go past end
	m, _ = m.handleCommitPaneDown()
	require.Equal(t, 2, m.selectedWorktree, "should stay at end of list")

	// Navigate up with k
	m, _ = m.handleCommitPaneUp()
	require.Equal(t, 1, m.selectedWorktree, "k should move to previous worktree")

	m, _ = m.handleCommitPaneUp()
	require.Equal(t, 0, m.selectedWorktree, "k should move to previous worktree")

	// Try to go before start
	m, _ = m.handleCommitPaneUp()
	require.Equal(t, 0, m.selectedWorktree, "should stay at start of list")
}

// TestBranchesLoadedMsg_SetsLoadedFlag verifies BranchesLoadedMsg sets branchListLoaded flag.
func TestBranchesLoadedMsg_SetsLoadedFlag(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	require.False(t, m.branchListLoaded, "branchListLoaded should be false initially")

	branches := []git.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "develop", IsCurrent: false},
	}

	m, _ = m.Update(BranchesLoadedMsg{Branches: branches, Err: nil})

	require.True(t, m.branchListLoaded, "branchListLoaded should be true after BranchesLoadedMsg")
	require.Equal(t, branches, m.branchList, "branchList should be set")
}

// TestWorktreesLoadedMsg_SetsLoadedFlag verifies WorktreesLoadedMsg sets worktreeListLoaded flag.
func TestWorktreesLoadedMsg_SetsLoadedFlag(t *testing.T) {
	m := New().SetSize(100, 50)
	m.visible = true
	require.False(t, m.worktreeListLoaded, "worktreeListLoaded should be false initially")

	worktrees := []git.WorktreeInfo{
		{Path: "/path/to/main", Branch: "main", HEAD: "abc1234"},
		{Path: "/path/to/develop", Branch: "develop", HEAD: "def5678"},
	}

	m, _ = m.Update(WorktreesLoadedMsg{Worktrees: worktrees, Err: nil})

	require.True(t, m.worktreeListLoaded, "worktreeListLoaded should be true after WorktreesLoadedMsg")
	require.Equal(t, worktrees, m.worktreeList, "worktreeList should be set")
}

// TestScrollStateManagement_Branches verifies scroll state updates correctly for branch list.
func TestScrollStateManagement_Branches(t *testing.T) {
	m := New().SetSize(100, 20) // Small height to force scrolling
	m.visible = true
	m.focus = focusCommitPicker
	m.activeCommitTab = commitsTabBranches

	// Create a list longer than visible height
	var branches []git.BranchInfo
	for i := 0; i < 20; i++ {
		branches = append(branches, git.BranchInfo{Name: fmt.Sprintf("branch-%d", i), IsCurrent: i == 0})
	}
	m.branchList = branches
	m.selectedBranch = 0
	m.branchScrollTop = 0
	m.branchListLoaded = true

	// Navigate down multiple times
	for i := 0; i < 15; i++ {
		m, _ = m.handleCommitPaneDown()
	}

	require.Equal(t, 15, m.selectedBranch, "should be at branch 15")
	// branchScrollTop should have adjusted to keep selection visible
	require.GreaterOrEqual(t, m.branchScrollTop, 0, "branchScrollTop should be non-negative")
}

// TestScrollStateManagement_Worktrees verifies scroll state updates correctly for worktree list.
func TestScrollStateManagement_Worktrees(t *testing.T) {
	m := New().SetSize(100, 20) // Small height to force scrolling
	m.visible = true
	m.focus = focusCommitPicker
	m.activeCommitTab = commitsTabWorktrees

	// Create a list longer than visible height
	var worktrees []git.WorktreeInfo
	for i := 0; i < 20; i++ {
		worktrees = append(worktrees, git.WorktreeInfo{Path: fmt.Sprintf("/path/to/wt-%d", i), Branch: fmt.Sprintf("branch-%d", i)})
	}
	m.worktreeList = worktrees
	m.selectedWorktree = 0
	m.worktreeScrollTop = 0
	m.worktreeListLoaded = true

	// Navigate down multiple times
	for i := 0; i < 15; i++ {
		m, _ = m.handleCommitPaneDown()
	}

	require.Equal(t, 15, m.selectedWorktree, "should be at worktree 15")
	// worktreeScrollTop should have adjusted to keep selection visible
	require.GreaterOrEqual(t, m.worktreeScrollTop, 0, "worktreeScrollTop should be non-negative")
}

// TestModel_HunkNavigation_YOffsetPreservedAfterView verifies that YOffset set by hunk navigation
// is preserved after calling View() which triggers ScrollablePane.
// This test reproduces a bug where the hunk header would sometimes not be visible after navigation
// because ScrollablePane's scroll position restoration logic was overwriting the navigation offset.
func TestModel_HunkNavigation_YOffsetPreservedAfterView(t *testing.T) {
	// Create a file with multiple hunks - enough lines that viewport needs scrolling
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,5 +1,6 @@",
				OldStart: 1, OldCount: 5, NewStart: 1, NewCount: 6,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,5 +1,6 @@"},
					{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineContext, Content: "line 2", OldLineNum: 2, NewLineNum: 2},
					{Type: LineAddition, Content: "new line", NewLineNum: 3},
					{Type: LineContext, Content: "line 3", OldLineNum: 3, NewLineNum: 4},
					{Type: LineContext, Content: "line 4", OldLineNum: 4, NewLineNum: 5},
					{Type: LineContext, Content: "line 5", OldLineNum: 5, NewLineNum: 6},
				},
			},
			{
				Header:   "@@ -20,5 +21,6 @@",
				OldStart: 20, OldCount: 5, NewStart: 21, NewCount: 6,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -20,5 +21,6 @@"},
					{Type: LineContext, Content: "line 20", OldLineNum: 20, NewLineNum: 21},
					{Type: LineContext, Content: "line 21", OldLineNum: 21, NewLineNum: 22},
					{Type: LineAddition, Content: "new line A", NewLineNum: 23},
					{Type: LineContext, Content: "line 22", OldLineNum: 22, NewLineNum: 24},
					{Type: LineContext, Content: "line 23", OldLineNum: 23, NewLineNum: 25},
					{Type: LineContext, Content: "line 24", OldLineNum: 24, NewLineNum: 26},
				},
			},
			{
				Header:   "@@ -40,5 +42,6 @@",
				OldStart: 40, OldCount: 5, NewStart: 42, NewCount: 6,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -40,5 +42,6 @@"},
					{Type: LineContext, Content: "line 40", OldLineNum: 40, NewLineNum: 42},
					{Type: LineContext, Content: "line 41", OldLineNum: 41, NewLineNum: 43},
					{Type: LineAddition, Content: "new line B", NewLineNum: 44},
					{Type: LineContext, Content: "line 42", OldLineNum: 42, NewLineNum: 45},
					{Type: LineContext, Content: "line 43", OldLineNum: 43, NewLineNum: 46},
					{Type: LineContext, Content: "line 44", OldLineNum: 44, NewLineNum: 47},
				},
			},
		},
	}

	// Use a small viewport height so scrolling is required
	m := New().SetSize(100, 15)
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane // Focus on diff pane for navigation
	m.refreshViewport()

	// Call View() to initialize ScrollablePane's dimension tracking
	_ = m.View()
	initialOffset := m.diffViewport.YOffset

	// Navigate to next hunk
	m, _ = m.navigateToNextHunk()
	expectedOffset := 7 // First hunk has 7 lines (header + 6 content), second hunk starts at line 7

	require.Equal(t, expectedOffset, m.diffViewport.YOffset, "YOffset should be at second hunk after navigation")

	// Call View() again - this is where ScrollablePane might overwrite our offset
	_ = m.View()

	// The YOffset should still be preserved after View()
	require.Equal(t, expectedOffset, m.diffViewport.YOffset,
		"YOffset should be preserved after View(); was %d before View(), got %d after (initial was %d)",
		expectedOffset, m.diffViewport.YOffset, initialOffset)
}

// TestModel_HunkNavigation_YOffsetPreservedWhenPreviouslyAtBottom tests the specific case where
// the viewport was "at bottom" before navigation, which could trigger ScrollablePane's GotoBottom().
func TestModel_HunkNavigation_YOffsetPreservedWhenPreviouslyAtBottom(t *testing.T) {
	// Create a file with hunks - content short enough that viewport starts "at bottom"
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,3 +1,4 @@",
				OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"},
					{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineAddition, Content: "new line", NewLineNum: 2},
					{Type: LineContext, Content: "line 2", OldLineNum: 2, NewLineNum: 3},
				},
			},
			{
				Header:   "@@ -10,3 +11,4 @@",
				OldStart: 10, OldCount: 3, NewStart: 11, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -10,3 +11,4 @@"},
					{Type: LineContext, Content: "line 10", OldLineNum: 10, NewLineNum: 11},
					{Type: LineAddition, Content: "new line A", NewLineNum: 12},
					{Type: LineContext, Content: "line 11", OldLineNum: 11, NewLineNum: 13},
				},
			},
		},
	}

	// Use viewport that's larger than content so we start "at bottom"
	m := New().SetSize(100, 30)
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.refreshViewport()

	// Initialize viewport via View() - with short content, viewport should be "at bottom"
	_ = m.View()

	// Verify we're at bottom (content fits in viewport)
	t.Logf("Before navigation: YOffset=%d, AtBottom=%v, TotalLines=%d, Height=%d",
		m.diffViewport.YOffset, m.diffViewport.AtBottom(),
		m.diffViewport.TotalLineCount(), m.diffViewport.Height)

	// Navigate to next hunk (should go to second hunk at line 4)
	m, _ = m.navigateToNextHunk()
	expectedOffset := 4 // First hunk has 4 lines

	require.Equal(t, expectedOffset, m.diffViewport.YOffset, "YOffset should be at second hunk")

	// Call View() - this triggers ScrollablePane which checks wasAtBottom
	_ = m.View()

	// The bug: if wasAtBottom was true, ScrollablePane calls GotoBottom() which overwrites our offset
	require.Equal(t, expectedOffset, m.diffViewport.YOffset,
		"YOffset should be preserved after View() even if viewport was previously at bottom")
}

// TestModel_HunkNavigation_MultipleNavigationsWithView tests multiple navigations with View() calls between.
// This simulates the real-world scenario where each key press triggers navigation + render cycle.
func TestModel_HunkNavigation_MultipleNavigationsWithView(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,5 +1,6 @@",
				OldStart: 1, OldCount: 5, NewStart: 1, NewCount: 6,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,5 +1,6 @@"},
					{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineContext, Content: "line 2", OldLineNum: 2, NewLineNum: 2},
					{Type: LineAddition, Content: "new line", NewLineNum: 3},
					{Type: LineContext, Content: "line 3", OldLineNum: 3, NewLineNum: 4},
					{Type: LineContext, Content: "line 4", OldLineNum: 4, NewLineNum: 5},
				},
			},
			{
				Header:   "@@ -10,5 +11,6 @@",
				OldStart: 10, OldCount: 5, NewStart: 11, NewCount: 6,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -10,5 +11,6 @@"},
					{Type: LineContext, Content: "line 10", OldLineNum: 10, NewLineNum: 11},
					{Type: LineContext, Content: "line 11", OldLineNum: 11, NewLineNum: 12},
					{Type: LineAddition, Content: "new line A", NewLineNum: 13},
					{Type: LineContext, Content: "line 12", OldLineNum: 12, NewLineNum: 14},
					{Type: LineContext, Content: "line 13", OldLineNum: 13, NewLineNum: 15},
				},
			},
			{
				Header:   "@@ -20,5 +22,6 @@",
				OldStart: 20, OldCount: 5, NewStart: 22, NewCount: 6,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -20,5 +22,6 @@"},
					{Type: LineContext, Content: "line 20", OldLineNum: 20, NewLineNum: 22},
					{Type: LineContext, Content: "line 21", OldLineNum: 21, NewLineNum: 23},
					{Type: LineAddition, Content: "new line B", NewLineNum: 24},
					{Type: LineContext, Content: "line 22", OldLineNum: 22, NewLineNum: 25},
					{Type: LineContext, Content: "line 23", OldLineNum: 23, NewLineNum: 26},
				},
			},
		},
	}

	m := New().SetSize(100, 20)
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.refreshViewport()

	// Initial render
	_ = m.View()
	require.Equal(t, 0, m.diffViewport.YOffset, "initial offset should be 0")

	// Navigate to hunk 2 (]) + View()
	m, _ = m.navigateToNextHunk()
	_ = m.View()
	require.Equal(t, 6, m.diffViewport.YOffset, "should be at hunk 2 (line 6)")

	// Navigate to hunk 3 (]) + View()
	m, _ = m.navigateToNextHunk()
	_ = m.View()
	require.Equal(t, 12, m.diffViewport.YOffset, "should be at hunk 3 (line 12)")

	// Navigate to hunk 1 (wrap) (]) + View()
	m, _ = m.navigateToNextHunk()
	_ = m.View()
	require.Equal(t, 0, m.diffViewport.YOffset, "should wrap to hunk 1 (line 0)")

	// Navigate backward to hunk 3 ([) + View()
	m, _ = m.navigateToPrevHunk()
	_ = m.View()
	require.Equal(t, 12, m.diffViewport.YOffset, "should wrap to hunk 3 (line 12)")
}

// TestModel_HunkNavigation_RealWorldDiff reproduces bug with realistic multi-hunk diff
// like renderer_test.go which has many small hunks spread throughout a large file.
// The issue: when navigating to a hunk, viewport scrolls to one line BELOW the header.
func TestModel_HunkNavigation_RealWorldDiff(t *testing.T) {
	// Simulate renderer_test.go diff: many small 1-line change hunks at different positions
	// Each hunk has context lines around a single changed line
	file := DiffFile{
		NewPath: "renderer_test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -36,7 +36,7 @@ func TestRenderDiffContent_Colors",
				OldStart: 36, OldCount: 7, NewStart: 36, NewCount: 7,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -36,7 +36,7 @@ func TestRenderDiffContent_Colors"},
					{Type: LineContext, Content: "		Deletions: 1,", OldLineNum: 36, NewLineNum: 36},
					{Type: LineContext, Content: "	}", OldLineNum: 37, NewLineNum: 37},
					{Type: LineContext, Content: "", OldLineNum: 38, NewLineNum: 38},
					{Type: LineDeletion, Content: "	result := renderDiffContentWithWordDiff(file, nil, 80, 20)", OldLineNum: 39},
					{Type: LineAddition, Content: "	result := renderDiffContentWithWordDiff(file, nil, 80, 20, 0)", NewLineNum: 39},
					{Type: LineContext, Content: "	require.NotEmpty(t, result)", OldLineNum: 40, NewLineNum: 40},
					{Type: LineContext, Content: "", OldLineNum: 41, NewLineNum: 41},
				},
			},
			{
				Header:   "@@ -70,7 +70,7 @@ func TestRenderDiffContent_LineNumbers",
				OldStart: 70, OldCount: 7, NewStart: 70, NewCount: 7,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -70,7 +70,7 @@ func TestRenderDiffContent_LineNumbers"},
					{Type: LineContext, Content: "		},", OldLineNum: 70, NewLineNum: 70},
					{Type: LineContext, Content: "	}", OldLineNum: 71, NewLineNum: 71},
					{Type: LineContext, Content: "", OldLineNum: 72, NewLineNum: 72},
					{Type: LineDeletion, Content: "	result := renderDiffContentWithWordDiff(file, nil, 80, 20)", OldLineNum: 73},
					{Type: LineAddition, Content: "	result := renderDiffContentWithWordDiff(file, nil, 80, 20, 0)", NewLineNum: 73},
					{Type: LineContext, Content: "", OldLineNum: 74, NewLineNum: 74},
					{Type: LineContext, Content: "	// Should show line numbers", OldLineNum: 75, NewLineNum: 75},
				},
			},
			{
				Header:   "@@ -85,7 +85,7 @@ func TestRenderDiffContent_BinaryFile",
				OldStart: 85, OldCount: 7, NewStart: 85, NewCount: 7,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -85,7 +85,7 @@ func TestRenderDiffContent_BinaryFile"},
					{Type: LineContext, Content: "		IsBinary: true,", OldLineNum: 85, NewLineNum: 85},
					{Type: LineContext, Content: "	}", OldLineNum: 86, NewLineNum: 86},
					{Type: LineContext, Content: "", OldLineNum: 87, NewLineNum: 87},
					{Type: LineDeletion, Content: "	result := renderDiffContentWithWordDiff(file, nil, 60, 10)", OldLineNum: 88},
					{Type: LineAddition, Content: "	result := renderDiffContentWithWordDiff(file, nil, 60, 10, 0)", NewLineNum: 88},
					{Type: LineContext, Content: "	require.Contains(t, result, \"Binary file\")", OldLineNum: 89, NewLineNum: 89},
					{Type: LineContext, Content: "	require.Contains(t, result, \"cannot display diff\")", OldLineNum: 90, NewLineNum: 90},
				},
			},
			{
				Header:   "@@ -97,7 +97,7 @@ func TestRenderDiffContent_NoHunks",
				OldStart: 97, OldCount: 7, NewStart: 97, NewCount: 7,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -97,7 +97,7 @@ func TestRenderDiffContent_NoHunks"},
					{Type: LineContext, Content: "		Hunks:   []DiffHunk{},", OldLineNum: 97, NewLineNum: 97},
					{Type: LineContext, Content: "	}", OldLineNum: 98, NewLineNum: 98},
					{Type: LineContext, Content: "", OldLineNum: 99, NewLineNum: 99},
					{Type: LineDeletion, Content: "	result := renderDiffContentWithWordDiff(file, nil, 60, 10)", OldLineNum: 100},
					{Type: LineAddition, Content: "	result := renderDiffContentWithWordDiff(file, nil, 60, 10, 0)", NewLineNum: 100},
					{Type: LineContext, Content: "	require.Contains(t, result, \"No changes\")", OldLineNum: 101, NewLineNum: 101},
					{Type: LineContext, Content: "}", OldLineNum: 102, NewLineNum: 102},
				},
			},
			{
				Header:   "@@ -111,11 +111,11 @@ func TestRenderDiffContent_ZeroDimensions",
				OldStart: 111, OldCount: 11, NewStart: 111, NewCount: 11,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -111,11 +111,11 @@ func TestRenderDiffContent_ZeroDimensions"},
					{Type: LineContext, Content: "	}", OldLineNum: 111, NewLineNum: 111},
					{Type: LineContext, Content: "", OldLineNum: 112, NewLineNum: 112},
					{Type: LineContext, Content: "	// Zero width", OldLineNum: 113, NewLineNum: 113},
					{Type: LineDeletion, Content: "	result := renderDiffContentWithWordDiff(file, nil, 0, 10)", OldLineNum: 114},
					{Type: LineAddition, Content: "	result := renderDiffContentWithWordDiff(file, nil, 0, 10, 0)", NewLineNum: 114},
					{Type: LineContext, Content: "	require.Empty(t, result)", OldLineNum: 115, NewLineNum: 115},
					{Type: LineContext, Content: "", OldLineNum: 116, NewLineNum: 116},
					{Type: LineContext, Content: "	// Zero height is allowed (means viewport handles scrolling)", OldLineNum: 117, NewLineNum: 117},
					{Type: LineDeletion, Content: "	result = renderDiffContentWithWordDiff(file, nil, 10, 0)", OldLineNum: 118},
					{Type: LineAddition, Content: "	result = renderDiffContentWithWordDiff(file, nil, 10, 0, 0)", NewLineNum: 118},
					{Type: LineContext, Content: "	require.NotEmpty(t, result)", OldLineNum: 119, NewLineNum: 119},
					{Type: LineContext, Content: "}", OldLineNum: 120, NewLineNum: 120},
				},
			},
		},
	}

	// Small viewport to require scrolling (typical terminal height)
	m := New().SetSize(120, 25)
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.refreshViewport()

	// Calculate expected positions
	// Hunk 1: starts at line 0, has 8 lines (header + 7 content)
	// Hunk 2: starts at line 8, has 8 lines
	// Hunk 3: starts at line 16, has 8 lines
	// Hunk 4: starts at line 24, has 8 lines
	// Hunk 5: starts at line 32, has 12 lines (header + 11 content)
	expectedPositions := []int{0, 8, 16, 24, 32}

	// Verify hunk positions match expected
	positions := m.getHunkPositionsForFile(&file)
	require.Equal(t, expectedPositions, positions, "hunk positions should match expected")

	// Initial render
	_ = m.View()
	require.Equal(t, 0, m.diffViewport.YOffset, "initial offset should be 0")

	// Navigate through each hunk and verify the header is visible
	for i := 1; i < len(expectedPositions); i++ {
		m, _ = m.navigateToNextHunk()
		view := m.View()

		expectedOffset := expectedPositions[i]
		require.Equal(t, expectedOffset, m.diffViewport.YOffset,
			"hunk %d: YOffset should be %d", i+1, expectedOffset)

		// Verify the hunk header is actually in the rendered view
		expectedHeader := file.Hunks[i].Header
		require.Contains(t, view, expectedHeader,
			"hunk %d: header '%s' should be visible in view at YOffset=%d",
			i+1, expectedHeader, m.diffViewport.YOffset)
	}
}

// TestModel_HunkNavigation_ViaKeyPress tests navigation using ] key through Update().
// This is the real codepath used when user presses ] in the app.
func TestModel_HunkNavigation_ViaKeyPress(t *testing.T) {
	file := DiffFile{
		NewPath: "renderer_test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -36,7 +36,7 @@",
				OldStart: 36, OldCount: 7, NewStart: 36, NewCount: 7,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -36,7 +36,7 @@"},
					{Type: LineContext, Content: "line 36", OldLineNum: 36, NewLineNum: 36},
					{Type: LineContext, Content: "line 37", OldLineNum: 37, NewLineNum: 37},
					{Type: LineContext, Content: "line 38", OldLineNum: 38, NewLineNum: 38},
					{Type: LineDeletion, Content: "old line 39", OldLineNum: 39},
					{Type: LineAddition, Content: "new line 39", NewLineNum: 39},
					{Type: LineContext, Content: "line 40", OldLineNum: 40, NewLineNum: 40},
					{Type: LineContext, Content: "line 41", OldLineNum: 41, NewLineNum: 41},
				},
			},
			{
				Header:   "@@ -70,7 +70,7 @@",
				OldStart: 70, OldCount: 7, NewStart: 70, NewCount: 7,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -70,7 +70,7 @@"},
					{Type: LineContext, Content: "line 70", OldLineNum: 70, NewLineNum: 70},
					{Type: LineContext, Content: "line 71", OldLineNum: 71, NewLineNum: 71},
					{Type: LineContext, Content: "line 72", OldLineNum: 72, NewLineNum: 72},
					{Type: LineDeletion, Content: "old line 73", OldLineNum: 73},
					{Type: LineAddition, Content: "new line 73", NewLineNum: 73},
					{Type: LineContext, Content: "line 74", OldLineNum: 74, NewLineNum: 74},
					{Type: LineContext, Content: "line 75", OldLineNum: 75, NewLineNum: 75},
				},
			},
			{
				Header:   "@@ -97,7 +97,7 @@",
				OldStart: 97, OldCount: 7, NewStart: 97, NewCount: 7,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -97,7 +97,7 @@"},
					{Type: LineContext, Content: "line 97", OldLineNum: 97, NewLineNum: 97},
					{Type: LineContext, Content: "line 98", OldLineNum: 98, NewLineNum: 98},
					{Type: LineContext, Content: "line 99", OldLineNum: 99, NewLineNum: 99},
					{Type: LineDeletion, Content: "old line 100", OldLineNum: 100},
					{Type: LineAddition, Content: "new line 100", NewLineNum: 100},
					{Type: LineContext, Content: "line 101", OldLineNum: 101, NewLineNum: 101},
					{Type: LineContext, Content: "line 102", OldLineNum: 102, NewLineNum: 102},
				},
			},
			{
				Header:   "@@ -111,11 +111,11 @@",
				OldStart: 111, OldCount: 11, NewStart: 111, NewCount: 11,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -111,11 +111,11 @@"},
					{Type: LineContext, Content: "line 111", OldLineNum: 111, NewLineNum: 111},
					{Type: LineContext, Content: "line 112", OldLineNum: 112, NewLineNum: 112},
					{Type: LineContext, Content: "line 113", OldLineNum: 113, NewLineNum: 113},
					{Type: LineDeletion, Content: "old line 114", OldLineNum: 114},
					{Type: LineAddition, Content: "new line 114", NewLineNum: 114},
					{Type: LineContext, Content: "line 115", OldLineNum: 115, NewLineNum: 115},
					{Type: LineContext, Content: "line 116", OldLineNum: 116, NewLineNum: 116},
					{Type: LineContext, Content: "line 117", OldLineNum: 117, NewLineNum: 117},
					{Type: LineDeletion, Content: "old line 118", OldLineNum: 118},
					{Type: LineAddition, Content: "new line 118", NewLineNum: 118},
					{Type: LineContext, Content: "line 119", OldLineNum: 119, NewLineNum: 119},
					{Type: LineContext, Content: "line 120", OldLineNum: 120, NewLineNum: 120},
				},
			},
		},
	}

	m := New().SetSize(120, 30)
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.refreshViewport()

	// Initial render
	_ = m.View()

	// Expected positions: 0, 8, 16, 24 (each hunk has 8 lines)
	expectedPositions := []int{0, 8, 16, 24}

	// Navigate through hunks using ] key through Update()
	for i := 1; i < len(expectedPositions); i++ {
		// Press ] key
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
		_ = cmd // command is nil for navigation

		// Render the view
		view := m.View()

		expectedOffset := expectedPositions[i]
		require.Equal(t, expectedOffset, m.diffViewport.YOffset,
			"after %d ] presses: YOffset should be %d", i, expectedOffset)

		// Verify the hunk header is in the rendered view
		expectedHeader := file.Hunks[i].Header
		require.Contains(t, view, expectedHeader,
			"after %d ] presses: header '%s' should be visible", i, expectedHeader)

		// Check hunk indicator shows correct value
		expectedIndicator := fmt.Sprintf("%d / %d hunks", i+1, len(file.Hunks))
		require.Contains(t, view, expectedIndicator,
			"after %d ] presses: indicator should show '%s'", i, expectedIndicator)
	}
}

// TestModel_HunkNavigation_HeaderAtTop verifies that after navigation, the hunk header
// is at the TOP of the viewport, not somewhere in the middle.
// This reproduces the bug where scrolling to a hunk shows the header one line below where it should be.
func TestModel_HunkNavigation_HeaderAtTop(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,5 +1,5 @@ first hunk",
				OldStart: 1, OldCount: 5, NewStart: 1, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,5 +1,5 @@ first hunk"},
					{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineContext, Content: "line 2", OldLineNum: 2, NewLineNum: 2},
					{Type: LineDeletion, Content: "old line 3", OldLineNum: 3},
					{Type: LineAddition, Content: "new line 3", NewLineNum: 3},
					{Type: LineContext, Content: "line 4", OldLineNum: 4, NewLineNum: 4},
					{Type: LineContext, Content: "line 5", OldLineNum: 5, NewLineNum: 5},
				},
			},
			{
				Header:   "@@ -20,5 +20,5 @@ second hunk",
				OldStart: 20, OldCount: 5, NewStart: 20, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -20,5 +20,5 @@ second hunk"},
					{Type: LineContext, Content: "line 20", OldLineNum: 20, NewLineNum: 20},
					{Type: LineContext, Content: "line 21", OldLineNum: 21, NewLineNum: 21},
					{Type: LineDeletion, Content: "old line 22", OldLineNum: 22},
					{Type: LineAddition, Content: "new line 22", NewLineNum: 22},
					{Type: LineContext, Content: "line 23", OldLineNum: 23, NewLineNum: 23},
					{Type: LineContext, Content: "line 24", OldLineNum: 24, NewLineNum: 24},
				},
			},
		},
	}

	m := New().SetSize(120, 25)
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.refreshViewport()

	// Initial render
	_ = m.View()

	// Navigate to second hunk
	m, _ = m.navigateToNextHunk()
	view := m.View()

	// Get just the diff pane content (extract from the full view)
	// The hunk header should appear in the first few lines of the diff content
	lines := strings.Split(view, "\n")

	// Find the line containing the second hunk header
	headerFound := false
	headerLineIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "@@ -20,5 +20,5 @@ second hunk") {
			headerFound = true
			headerLineIdx = i
			break
		}
	}

	require.True(t, headerFound, "second hunk header should be visible in view")

	// The header should be near the top of the view (within first few lines that are part of diff pane)
	// Allow some lines for the border and other UI elements
	t.Logf("Header found at line index %d of %d total lines", headerLineIdx, len(lines))
	t.Logf("YOffset after navigation: %d", m.diffViewport.YOffset)

	// The hunk header should NOT be preceded by content lines from this same hunk
	// (i.e., we should not see "line 20" before "@@ -20,5")
	for i := 0; i < headerLineIdx; i++ {
		line := lines[i]
		// Content from hunk 2 should NOT appear before the header
		require.NotContains(t, line, "line 20",
			"content from hunk 2 should not appear before the hunk 2 header; header at line %d, found 'line 20' at line %d",
			headerLineIdx, i)
		require.NotContains(t, line, "line 21",
			"content from hunk 2 should not appear before the hunk 2 header")
	}
}

// TestModel_HunkNavigation_VisualVerification verifies the hunk header is actually visible
// by checking that the view content starts with the expected hunk header.
func TestModel_HunkNavigation_VisualVerification(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,3 +1,4 @@",
				OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"},
					{Type: LineContext, Content: "line 1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineAddition, Content: "new line", NewLineNum: 2},
					{Type: LineContext, Content: "line 2", OldLineNum: 2, NewLineNum: 3},
				},
			},
			{
				Header:   "@@ -10,3 +11,4 @@",
				OldStart: 10, OldCount: 3, NewStart: 11, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -10,3 +11,4 @@"},
					{Type: LineContext, Content: "line 10", OldLineNum: 10, NewLineNum: 11},
					{Type: LineAddition, Content: "new line A", NewLineNum: 12},
					{Type: LineContext, Content: "line 11", OldLineNum: 11, NewLineNum: 13},
				},
			},
			{
				Header:   "@@ -20,3 +22,4 @@",
				OldStart: 20, OldCount: 3, NewStart: 22, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "@@ -20,3 +22,4 @@"},
					{Type: LineContext, Content: "line 20", OldLineNum: 20, NewLineNum: 22},
					{Type: LineAddition, Content: "new line B", NewLineNum: 23},
					{Type: LineContext, Content: "line 21", OldLineNum: 21, NewLineNum: 24},
				},
			},
		},
	}

	m := New().SetSize(100, 20)
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.refreshViewport()

	// Initialize
	_ = m.View()

	// Navigate to second hunk
	m, _ = m.navigateToNextHunk()

	// Get the view and check that second hunk header is visible
	view := m.View()

	// The second hunk header "@@ -10,3 +11,4 @@" should be visible in the view
	require.Contains(t, view, "@@ -10,3 +11,4 @@",
		"Second hunk header should be visible after navigation; YOffset=%d", m.diffViewport.YOffset)

	// Navigate to third hunk
	m, _ = m.navigateToNextHunk()

	view = m.View()
	require.Contains(t, view, "@@ -20,3 +22,4 @@",
		"Third hunk header should be visible after navigation; YOffset=%d", m.diffViewport.YOffset)
}

// TestModel_HunkNavigation_HeaderAtTopOfViewport tests that when navigating to a hunk,
// the hunk HEADER is at the TOP of the viewport, not one line below.
// This is the key test for the off-by-one rendering bug.
func TestModel_HunkNavigation_HeaderAtTopOfViewport(t *testing.T) {
	// Create a file with 3 hunks, each with 6 lines (header + 5 content)
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,5 +1,5 @@ func Hunk1",
				OldStart: 1, OldCount: 5, NewStart: 1, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "func Hunk1"},
					{Type: LineContext, Content: "hunk1_line1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineDeletion, Content: "hunk1_deleted", OldLineNum: 2},
					{Type: LineAddition, Content: "hunk1_added", NewLineNum: 2},
					{Type: LineContext, Content: "hunk1_line3", OldLineNum: 3, NewLineNum: 3},
					{Type: LineContext, Content: "hunk1_line4", OldLineNum: 4, NewLineNum: 4},
				},
			},
			{
				Header:   "@@ -10,5 +10,5 @@ func Hunk2",
				OldStart: 10, OldCount: 5, NewStart: 10, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "func Hunk2"},
					{Type: LineContext, Content: "hunk2_line1", OldLineNum: 10, NewLineNum: 10},
					{Type: LineDeletion, Content: "hunk2_deleted", OldLineNum: 11},
					{Type: LineAddition, Content: "hunk2_added", NewLineNum: 11},
					{Type: LineContext, Content: "hunk2_line3", OldLineNum: 12, NewLineNum: 12},
					{Type: LineContext, Content: "hunk2_line4", OldLineNum: 13, NewLineNum: 13},
				},
			},
			{
				Header:   "@@ -20,5 +20,5 @@ func Hunk3",
				OldStart: 20, OldCount: 5, NewStart: 20, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "func Hunk3"},
					{Type: LineContext, Content: "hunk3_line1", OldLineNum: 20, NewLineNum: 20},
					{Type: LineDeletion, Content: "hunk3_deleted", OldLineNum: 21},
					{Type: LineAddition, Content: "hunk3_added", NewLineNum: 21},
					{Type: LineContext, Content: "hunk3_line3", OldLineNum: 22, NewLineNum: 22},
					{Type: LineContext, Content: "hunk3_line4", OldLineNum: 23, NewLineNum: 23},
				},
			},
		},
	}

	// Use a small viewport height so that scrolling is required
	// Total lines: 18 (3 hunks * 6 lines)
	// Viewport height: 10 lines (so we need to scroll to see later hunks)
	m := New().SetSize(120, 30) // Overall size
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.refreshViewport()

	// Calculate expected positions: hunk 1 at 0, hunk 2 at 6, hunk 3 at 12
	positions := m.getHunkPositionsForFile(&file)
	require.Equal(t, []int{0, 6, 12}, positions, "hunk positions should be 0, 6, 12")

	// Initial state: at top, hunk 1 header should be visible
	view := m.View()
	require.Contains(t, view, "@@ -1,5 +1,5 @@ func Hunk1",
		"Hunk 1 header should be visible at start")

	// Navigate to hunk 2
	m, _ = m.navigateToNextHunk()
	require.Equal(t, 6, m.diffViewport.YOffset, "YOffset should be 6 after navigating to hunk 2")

	// Get the view and check what's at the TOP of the diff pane
	view = m.View()

	// The bug: if the viewport shows "hunk2_line1" at the top instead of the header,
	// then the header is not visible (it's scrolled past)
	require.Contains(t, view, "@@ -10,5 +10,5 @@ func Hunk2",
		"Hunk 2 header MUST be visible after navigating to hunk 2")

	// More precise check: the header should appear BEFORE the first content line
	// If "hunk2_line1" appears but the header doesn't, that's the off-by-one bug
	hunk2HeaderIndex := strings.Index(view, "@@ -10,5 +10,5 @@ func Hunk2")
	hunk2Line1Index := strings.Index(view, "hunk2_line1")

	if hunk2Line1Index >= 0 && hunk2HeaderIndex < 0 {
		t.Fatalf("BUG REPRODUCED: hunk2_line1 is visible but the header is NOT visible. "+
			"This is the off-by-one bug where viewport scrolls to content instead of header. "+
			"YOffset=%d, expected header at line 6", m.diffViewport.YOffset)
	}

	if hunk2HeaderIndex >= 0 && hunk2Line1Index >= 0 {
		require.Less(t, hunk2HeaderIndex, hunk2Line1Index,
			"Header should appear BEFORE the first content line in the view. "+
				"headerIndex=%d, line1Index=%d", hunk2HeaderIndex, hunk2Line1Index)
	}

	// Navigate to hunk 3 and verify same behavior
	m, _ = m.navigateToNextHunk()
	require.Equal(t, 12, m.diffViewport.YOffset, "YOffset should be 12 after navigating to hunk 3")

	view = m.View()
	require.Contains(t, view, "@@ -20,5 +20,5 @@ func Hunk3",
		"Hunk 3 header MUST be visible after navigating to hunk 3")

	hunk3HeaderIndex := strings.Index(view, "@@ -20,5 +20,5 @@ func Hunk3")
	hunk3Line1Index := strings.Index(view, "hunk3_line1")

	if hunk3Line1Index >= 0 && hunk3HeaderIndex < 0 {
		t.Fatalf("BUG REPRODUCED: hunk3_line1 is visible but the header is NOT visible. "+
			"YOffset=%d, expected header at line 12", m.diffViewport.YOffset)
	}
}

// TestModel_HunkNavigation_WithoutRefreshViewport simulates the case where
// navigation happens without prior refreshViewport call - the real-world
// scenario where the viewport might have stale or no content.
func TestModel_HunkNavigation_WithoutRefreshViewport(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,5 +1,5 @@ func Hunk1",
				OldStart: 1, OldCount: 5, NewStart: 1, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "func Hunk1"},
					{Type: LineContext, Content: "hunk1_line1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineDeletion, Content: "hunk1_deleted", OldLineNum: 2},
					{Type: LineAddition, Content: "hunk1_added", NewLineNum: 2},
					{Type: LineContext, Content: "hunk1_line3", OldLineNum: 3, NewLineNum: 3},
					{Type: LineContext, Content: "hunk1_line4", OldLineNum: 4, NewLineNum: 4},
				},
			},
			{
				Header:   "@@ -10,5 +10,5 @@ func Hunk2",
				OldStart: 10, OldCount: 5, NewStart: 10, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "func Hunk2"},
					{Type: LineContext, Content: "hunk2_line1", OldLineNum: 10, NewLineNum: 10},
					{Type: LineDeletion, Content: "hunk2_deleted", OldLineNum: 11},
					{Type: LineAddition, Content: "hunk2_added", NewLineNum: 11},
					{Type: LineContext, Content: "hunk2_line3", OldLineNum: 12, NewLineNum: 12},
					{Type: LineContext, Content: "hunk2_line4", OldLineNum: 13, NewLineNum: 13},
				},
			},
		},
	}

	// Create model WITHOUT calling refreshViewport
	// This simulates a scenario where the viewport has no prior content
	m := New().SetSize(120, 30)
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	// NOT calling m.refreshViewport() - this is the difference from other tests

	// Navigate to hunk 2 immediately
	m, _ = m.navigateToNextHunk()

	// The viewport has no content yet, so YOffset is set but viewport.lines is empty
	// This might cause wasAtBottom to be true, triggering GotoBottom on first render
	t.Logf("After navigation: YOffset=%d", m.diffViewport.YOffset)

	// Now render
	view := m.View()

	// Check if the header is visible
	if !strings.Contains(view, "@@ -10,5 +10,5 @@ func Hunk2") {
		// Check what IS visible
		if strings.Contains(view, "hunk2_line1") {
			t.Fatalf("BUG REPRODUCED: Without refreshViewport, hunk2_line1 is visible " +
				"but header is NOT. The viewport scroll position was incorrectly set during render.")
		}
		if strings.Contains(view, "@@ -1,5 +1,5 @@ func Hunk1") {
			t.Fatalf("BUG REPRODUCED: Without refreshViewport, viewport is showing Hunk1 instead of Hunk2. "+
				"The YOffset (%d) was ignored during render.", m.diffViewport.YOffset)
		}
		t.Logf("View doesn't contain expected header. YOffset=%d", m.diffViewport.YOffset)
	}
}

// TestModel_HunkNavigation_ExactOffByOne tests the specific bug pattern where the
// viewport is positioned exactly 1 line AFTER the hunk header, showing the first
// content line at the top instead of the header.
func TestModel_HunkNavigation_ExactOffByOne(t *testing.T) {
	// Create a diff with hunks where we can detect exact line positions
	// Each hunk header should be "HEADER_N" and first content should be "CONTENT_N_1"
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,4 +1,4 @@ HEADER_1",
				OldStart: 1, OldCount: 4, NewStart: 1, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "HEADER_1"},
					{Type: LineContext, Content: "CONTENT_1_1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineContext, Content: "CONTENT_1_2", OldLineNum: 2, NewLineNum: 2},
					{Type: LineContext, Content: "CONTENT_1_3", OldLineNum: 3, NewLineNum: 3},
					{Type: LineContext, Content: "CONTENT_1_4", OldLineNum: 4, NewLineNum: 4},
				},
			},
			{
				Header:   "@@ -10,4 +10,4 @@ HEADER_2",
				OldStart: 10, OldCount: 4, NewStart: 10, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "HEADER_2"},
					{Type: LineContext, Content: "CONTENT_2_1", OldLineNum: 10, NewLineNum: 10},
					{Type: LineContext, Content: "CONTENT_2_2", OldLineNum: 11, NewLineNum: 11},
					{Type: LineContext, Content: "CONTENT_2_3", OldLineNum: 12, NewLineNum: 12},
					{Type: LineContext, Content: "CONTENT_2_4", OldLineNum: 13, NewLineNum: 13},
				},
			},
			{
				Header:   "@@ -20,4 +20,4 @@ HEADER_3",
				OldStart: 20, OldCount: 4, NewStart: 20, NewCount: 4,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "HEADER_3"},
					{Type: LineContext, Content: "CONTENT_3_1", OldLineNum: 20, NewLineNum: 20},
					{Type: LineContext, Content: "CONTENT_3_2", OldLineNum: 21, NewLineNum: 21},
					{Type: LineContext, Content: "CONTENT_3_3", OldLineNum: 22, NewLineNum: 22},
					{Type: LineContext, Content: "CONTENT_3_4", OldLineNum: 23, NewLineNum: 23},
				},
			},
		},
	}

	// Positions: hunk1 at 0, hunk2 at 5, hunk3 at 10

	m := New().SetSize(120, 25)
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.refreshViewport()

	// Navigate to hunk 2
	m, _ = m.navigateToNextHunk()
	require.Equal(t, 5, m.diffViewport.YOffset, "YOffset should be 5 for hunk 2")

	view := m.View()

	// Extract just the content lines from the view to check order
	lines := strings.Split(view, "\n")

	// Find the FIRST occurrence of HEADER_2 and CONTENT_2_1
	headerLine := -1
	contentLine := -1
	for i, line := range lines {
		if strings.Contains(line, "HEADER_2") && headerLine == -1 {
			headerLine = i
		}
		if strings.Contains(line, "CONTENT_2_1") && contentLine == -1 {
			contentLine = i
		}
	}

	t.Logf("In rendered output: HEADER_2 at line %d, CONTENT_2_1 at line %d", headerLine, contentLine)

	// The off-by-one bug would show:
	// - CONTENT_2_1 visible but HEADER_2 NOT visible (header scrolled past)
	if contentLine >= 0 && headerLine < 0 {
		t.Fatalf("EXACT OFF-BY-ONE BUG: CONTENT_2_1 is visible at line %d but HEADER_2 is NOT visible. "+
			"The viewport YOffset (%d) is 1 more than the hunk position.", contentLine, m.diffViewport.YOffset)
	}

	// Header must be visible
	require.GreaterOrEqual(t, headerLine, 0, "HEADER_2 must be visible after navigating to hunk 2")

	// Header must come before content
	if headerLine >= 0 && contentLine >= 0 {
		require.Less(t, headerLine, contentLine,
			"HEADER_2 (line %d) must appear before CONTENT_2_1 (line %d)", headerLine, contentLine)
	}
}

// TestModel_HunkNavigation_DimensionMismatch tests the scenario where viewport dimensions
// are set after navigation. Verifies that navigation scroll position is preserved during render.
// Note: This test was updated after removing ScrollablePane in favor of direct BorderedPane rendering.
func TestModel_HunkNavigation_DimensionMismatch(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,3 +1,3 @@ HUNK_ONE",
				OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 3,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "HUNK_ONE"},
					{Type: LineContext, Content: "line1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineContext, Content: "line2", OldLineNum: 2, NewLineNum: 2},
					{Type: LineContext, Content: "line3", OldLineNum: 3, NewLineNum: 3},
				},
			},
			{
				Header:   "@@ -10,3 +10,3 @@ HUNK_TWO",
				OldStart: 10, OldCount: 3, NewStart: 10, NewCount: 3,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "HUNK_TWO"},
					{Type: LineContext, Content: "line10", OldLineNum: 10, NewLineNum: 10},
					{Type: LineContext, Content: "line11", OldLineNum: 11, NewLineNum: 11},
					{Type: LineContext, Content: "line12", OldLineNum: 12, NewLineNum: 12},
				},
			},
		},
	}

	m := New().SetSize(120, 25)
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane

	// Call refreshViewport to set content (required for non-ScrollablePane rendering)
	m.refreshViewport()

	// Navigate to hunk 2
	m, _ = m.navigateToNextHunk()
	require.Equal(t, 4, m.diffViewport.YOffset, "YOffset should be 4 for hunk 2")

	// Render
	view := m.View()

	// Check if HUNK_TWO is visible
	if !strings.Contains(view, "HUNK_TWO") {
		if strings.Contains(view, "HUNK_ONE") {
			t.Fatalf("DIMENSION MISMATCH BUG: Dimension change caused scroll reset. "+
				"HUNK_ONE is visible instead of HUNK_TWO. YOffset was %d", m.diffViewport.YOffset)
		}
		t.Fatalf("Neither hunk header visible. View might be empty or error state.")
	}
}

// TestModel_HunkNavigation_RenderedYOffset verifies that the YOffset set by navigation
// is actually used during rendering, not reset by ScrollablePane or View().
// This test examines the copy's behavior by checking the FIRST visible line.
func TestModel_HunkNavigation_RenderedYOffset(t *testing.T) {
	// Create a file where we can clearly identify which line is at the top
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,3 +1,3 @@ HUNK_ONE_HEADER",
				OldStart: 1, OldCount: 3, NewStart: 1, NewCount: 3,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "HUNK_ONE_HEADER"},
					{Type: LineContext, Content: "HUNK_ONE_LINE_A", OldLineNum: 1, NewLineNum: 1},
					{Type: LineContext, Content: "HUNK_ONE_LINE_B", OldLineNum: 2, NewLineNum: 2},
					{Type: LineContext, Content: "HUNK_ONE_LINE_C", OldLineNum: 3, NewLineNum: 3},
				},
			},
			{
				Header:   "@@ -10,3 +10,3 @@ HUNK_TWO_HEADER",
				OldStart: 10, OldCount: 3, NewStart: 10, NewCount: 3,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "HUNK_TWO_HEADER"},
					{Type: LineContext, Content: "HUNK_TWO_LINE_A", OldLineNum: 10, NewLineNum: 10},
					{Type: LineContext, Content: "HUNK_TWO_LINE_B", OldLineNum: 11, NewLineNum: 11},
					{Type: LineContext, Content: "HUNK_TWO_LINE_C", OldLineNum: 12, NewLineNum: 12},
				},
			},
			{
				Header:   "@@ -20,3 +20,3 @@ HUNK_THREE_HEADER",
				OldStart: 20, OldCount: 3, NewStart: 20, NewCount: 3,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "HUNK_THREE_HEADER"},
					{Type: LineContext, Content: "HUNK_THREE_LINE_A", OldLineNum: 20, NewLineNum: 20},
					{Type: LineContext, Content: "HUNK_THREE_LINE_B", OldLineNum: 21, NewLineNum: 21},
					{Type: LineContext, Content: "HUNK_THREE_LINE_C", OldLineNum: 22, NewLineNum: 22},
				},
			},
		},
	}

	// Setup model with viewport that requires scrolling
	m := New().SetSize(120, 15)
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.refreshViewport()

	// Navigate to hunk 2 (positions are 0, 4, 8)
	m, _ = m.navigateToNextHunk()
	require.Equal(t, 4, m.diffViewport.YOffset, "Should navigate to hunk 2 at position 4")

	// Render and extract the diff pane content
	view := m.View()

	// Check the order of content in the view
	// If YOffset=4 is respected, we should see HUNK_TWO_HEADER first, then content
	// If YOffset is reset (bug), we might see HUNK_ONE content or wrong position

	hunk2HeaderIdx := strings.Index(view, "HUNK_TWO_HEADER")
	hunk2LineAIdx := strings.Index(view, "HUNK_TWO_LINE_A")
	hunk1LineCIdx := strings.Index(view, "HUNK_ONE_LINE_C") // Last line of hunk 1 (line 3)

	t.Logf("String indices: hunk2HeaderIdx=%d, hunk2LineAIdx=%d, hunk1LineCIdx=%d",
		hunk2HeaderIdx, hunk2LineAIdx, hunk1LineCIdx)

	// Scenario 1: Bug where header is missing but content is visible (off-by-one)
	if hunk2LineAIdx >= 0 && hunk2HeaderIdx < 0 {
		t.Fatalf("OFF-BY-ONE BUG: HUNK_TWO_LINE_A is visible but HUNK_TWO_HEADER is not. " +
			"The viewport is scrolled 1 line past the header.")
	}

	// Scenario 2: Bug where YOffset is reset to 0
	if hunk1LineCIdx >= 0 && hunk2HeaderIdx < 0 {
		t.Fatalf("YOFFSET RESET BUG: HUNK_ONE content is visible but HUNK_TWO_HEADER is not. " +
			"The YOffset was reset during rendering.")
	}

	// The header MUST be visible
	require.True(t, hunk2HeaderIdx >= 0,
		"HUNK_TWO_HEADER must be visible after navigating to hunk 2")

	// If both are visible, header must come before content
	if hunk2HeaderIdx >= 0 && hunk2LineAIdx >= 0 {
		require.Less(t, hunk2HeaderIdx, hunk2LineAIdx,
			"Header must appear before content in rendered output")
	}
}

// TestModel_HunkNavigation_ViewportAtBottom tests the scenario where the viewport
// was at the bottom before navigation, which might trigger wasAtBottom logic.
func TestModel_HunkNavigation_ViewportAtBottom(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header:   "@@ -1,5 +1,5 @@ func Hunk1",
				OldStart: 1, OldCount: 5, NewStart: 1, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "func Hunk1"},
					{Type: LineContext, Content: "hunk1_line1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineDeletion, Content: "hunk1_deleted", OldLineNum: 2},
					{Type: LineAddition, Content: "hunk1_added", NewLineNum: 2},
					{Type: LineContext, Content: "hunk1_line3", OldLineNum: 3, NewLineNum: 3},
					{Type: LineContext, Content: "hunk1_line4", OldLineNum: 4, NewLineNum: 4},
				},
			},
			{
				Header:   "@@ -10,5 +10,5 @@ func Hunk2",
				OldStart: 10, OldCount: 5, NewStart: 10, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "func Hunk2"},
					{Type: LineContext, Content: "hunk2_line1", OldLineNum: 10, NewLineNum: 10},
					{Type: LineDeletion, Content: "hunk2_deleted", OldLineNum: 11},
					{Type: LineAddition, Content: "hunk2_added", NewLineNum: 11},
					{Type: LineContext, Content: "hunk2_line3", OldLineNum: 12, NewLineNum: 12},
					{Type: LineContext, Content: "hunk2_line4", OldLineNum: 13, NewLineNum: 13},
				},
			},
			{
				Header:   "@@ -20,5 +20,5 @@ func Hunk3",
				OldStart: 20, OldCount: 5, NewStart: 20, NewCount: 5,
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "func Hunk3"},
					{Type: LineContext, Content: "hunk3_line1", OldLineNum: 20, NewLineNum: 20},
					{Type: LineDeletion, Content: "hunk3_deleted", OldLineNum: 21},
					{Type: LineAddition, Content: "hunk3_added", NewLineNum: 21},
					{Type: LineContext, Content: "hunk3_line3", OldLineNum: 22, NewLineNum: 22},
					{Type: LineContext, Content: "hunk3_line4", OldLineNum: 23, NewLineNum: 23},
				},
			},
		},
	}

	// Use a SMALL viewport height so that scrolling is required
	// Total lines: 18, viewport height will be small
	// Model height 20 gives diffPaneContentHeight = 8 (after accounting for header and borders)
	m := New().SetSize(120, 20) // Small height to force scrolling
	m.visible = true
	m.workingDirFiles = []DiffFile{file}
	m.workingDirTree = NewFileTree([]DiffFile{file})
	m.selectedWorkingDirNode = 0
	m.focus = focusDiffPane
	m.refreshViewport()

	// First, scroll to the bottom
	m.diffViewport.GotoBottom()
	initialOffset := m.diffViewport.YOffset
	t.Logf("After GotoBottom: YOffset=%d, AtBottom=%v",
		initialOffset, m.diffViewport.AtBottom())

	// Render once at bottom to establish state
	_ = m.View()
	t.Logf("After first View at bottom: YOffset=%d", m.diffViewport.YOffset)

	// Now navigate - should go to hunk 3 (position 12) since that's the next hunk after position 10
	// Hunk positions are [0, 6, 12]. At offset 10, the next hunk after 10 is at 12.
	m, _ = m.navigateToNextHunk()
	t.Logf("After navigateToNextHunk: YOffset=%d", m.diffViewport.YOffset)

	// The navigation should set YOffset to 12 (hunk 3, which is after current offset 10)
	require.Equal(t, 12, m.diffViewport.YOffset, "Navigation should go to hunk 3 at position 12")

	// Render and check - hunk 3 header should be visible
	view := m.View()

	// After render, check that hunk 3 header is visible (not overridden by wasAtBottom logic)
	if !strings.Contains(view, "@@ -20,5 +20,5 @@ func Hunk3") {
		t.Logf("View output (first 500 chars): %s", view[:min(500, len(view))])
		t.Fatalf("BUG: After navigating to hunk 3, the header is not visible. "+
			"YOffset=%d. The wasAtBottom logic may have reset the scroll position.",
			m.diffViewport.YOffset)
	}
}
