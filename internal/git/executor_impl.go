package git

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Git-specific errors for worktree operations.
var (
	// ErrBranchAlreadyCheckedOut indicates the branch is checked out in another worktree.
	ErrBranchAlreadyCheckedOut = errors.New("branch already checked out in another worktree")

	// ErrPathAlreadyExists indicates the worktree path already exists.
	ErrPathAlreadyExists = errors.New("worktree path already exists")

	// ErrWorktreeLocked indicates the worktree is locked.
	ErrWorktreeLocked = errors.New("worktree is locked")

	// ErrNotGitRepo indicates the directory is not a git repository.
	ErrNotGitRepo = errors.New("not a git repository")

	// ErrUnsafeParentDirectory indicates the parent directory is restricted.
	ErrUnsafeParentDirectory = errors.New("unsafe parent directory")
)

// Compile-time check that RealExecutor implements GitExecutor.
var _ GitExecutor = (*RealExecutor)(nil)

// RealExecutor implements GitExecutor by executing actual git commands.
type RealExecutor struct {
	workDir string
}

// NewRealExecutor creates a new RealExecutor.
func NewRealExecutor(workDir string) *RealExecutor {
	return &RealExecutor{workDir: workDir}
}

// runGit executes a git command and returns an error if it fails.
func (e *RealExecutor) runGit(args ...string) error {
	_, err := e.runGitOutput(args...)
	return err
}

// runGitOutput executes a git command and returns stdout and any error.
func (e *RealExecutor) runGitOutput(args ...string) (string, error) {
	//nolint:gosec // G204: args come from controlled sources
	cmd := exec.Command("git", args...)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		// Parse git-specific errors
		if stderrStr != "" {
			return "", parseGitError(stderrStr, err)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// parseGitError converts git stderr messages to specific error types.
func parseGitError(stderr string, originalErr error) error {
	stderrLower := strings.ToLower(stderr)

	// Branch already checked out: fatal: '<branch>' is already checked out
	if strings.Contains(stderrLower, "is already checked out") ||
		strings.Contains(stderrLower, "already checked out at") {
		return fmt.Errorf("%w: %s", ErrBranchAlreadyCheckedOut, stderr)
	}

	// Path already exists: fatal: '<path>' already exists
	if strings.Contains(stderrLower, "already exists") {
		return fmt.Errorf("%w: %s", ErrPathAlreadyExists, stderr)
	}

	// Locked worktree: fatal: '<path>' is locked
	if strings.Contains(stderrLower, "is locked") {
		return fmt.Errorf("%w: %s", ErrWorktreeLocked, stderr)
	}

	// Not a git repository
	if strings.Contains(stderrLower, "not a git repository") {
		return fmt.Errorf("%w: %s", ErrNotGitRepo, stderr)
	}

	return fmt.Errorf("git error: %s: %w", stderr, originalErr)
}

// IsGitRepo checks if the current directory is a git repository.
func (e *RealExecutor) IsGitRepo() bool {
	err := e.runGit("rev-parse", "--git-dir")
	return err == nil
}

// IsWorktree checks if the current directory is inside a git worktree (not the main repo).
func (e *RealExecutor) IsWorktree() (bool, error) {
	// Get the git dir path
	gitDir, err := e.runGitOutput("rev-parse", "--git-dir")
	if err != nil {
		return false, err
	}

	// In a worktree, .git is a file pointing to the actual git dir.
	// In the main repo, .git is a directory.
	var gitPath string
	if filepath.IsAbs(gitDir) {
		gitPath = gitDir
	} else if e.workDir != "" {
		gitPath = filepath.Join(e.workDir, gitDir)
	} else {
		gitPath = gitDir
	}

	info, err := os.Stat(gitPath)
	if err != nil {
		return false, fmt.Errorf("failed to stat git dir: %w", err)
	}

	// If .git is a file (not a directory), we're in a worktree
	return !info.IsDir(), nil
}

// IsBareRepo checks if the repository is a bare repository.
func (e *RealExecutor) IsBareRepo() (bool, error) {
	output, err := e.runGitOutput("rev-parse", "--is-bare-repository")
	if err != nil {
		return false, err
	}
	return output == "true", nil
}

// IsDetachedHead checks if HEAD is detached (not on a branch).
func (e *RealExecutor) IsDetachedHead() (bool, error) {
	// symbolic-ref fails if HEAD is detached
	err := e.runGit("symbolic-ref", "HEAD")
	if err != nil {
		// Check if it's because HEAD is detached vs other error
		if _, revErr := e.runGitOutput("rev-parse", "HEAD"); revErr == nil {
			// HEAD exists but is not a symbolic ref - detached
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// GetCurrentBranch returns the name of the current branch.
func (e *RealExecutor) GetCurrentBranch() (string, error) {
	// First try git branch --show-current (git 2.22+)
	output, err := e.runGitOutput("branch", "--show-current")
	if err == nil && output != "" {
		return output, nil
	}

	// Fallback: parse symbolic-ref
	output, err = e.runGitOutput("symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return output, nil
}

// GetMainBranch detects the main branch name using multiple strategies.
// Order: config → remote HEAD → main/master existence → fallback to "main"
func (e *RealExecutor) GetMainBranch() (string, error) {
	// 1. Check git config init.defaultBranch
	if branch, err := e.runGitOutput("config", "init.defaultBranch"); err == nil && branch != "" {
		return branch, nil
	}

	// 2. Check remote HEAD (works for cloned repos)
	// Returns: refs/remotes/origin/main -> extract "main"
	if ref, err := e.runGitOutput("symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}

	// 3. Check which of main/master exists locally
	if err := e.runGit("show-ref", "--verify", "--quiet", "refs/heads/main"); err == nil {
		return "main", nil
	}
	if err := e.runGit("show-ref", "--verify", "--quiet", "refs/heads/master"); err == nil {
		return "master", nil
	}

	// 4. Fallback to "main"
	return "main", nil
}

// IsOnMainBranch checks if the current branch is the main branch.
func (e *RealExecutor) IsOnMainBranch() (bool, error) {
	currentBranch, err := e.GetCurrentBranch()
	if err != nil {
		return false, err
	}

	mainBranch, err := e.GetMainBranch()
	if err != nil {
		return false, err
	}

	return currentBranch == mainBranch, nil
}

// GetRepoRoot returns the root directory of the git repository.
func (e *RealExecutor) GetRepoRoot() (string, error) {
	return e.runGitOutput("rev-parse", "--show-toplevel")
}

// HasUncommittedChanges checks if there are uncommitted changes in the working directory.
func (e *RealExecutor) HasUncommittedChanges() (bool, error) {
	output, err := e.runGitOutput("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return output != "", nil
}

// unsafeParentDirs lists directories that should never be used as worktree parents.
var unsafeParentDirs = map[string]bool{
	"/":        true,
	"/System":  true,
	"/usr":     true,
	"/bin":     true,
	"/sbin":    true,
	"/etc":     true,
	"/var":     true,
	"/tmp":     true,
	"/private": true,
}

// DetermineWorktreePath determines the best path for a new worktree.
// Strategy: prefer sibling directory, fallback to .perles/worktrees/
func (e *RealExecutor) DetermineWorktreePath(sessionID string) (string, error) {
	repoRoot, err := e.GetRepoRoot()
	if err != nil {
		return "", fmt.Errorf("failed to get repo root: %w", err)
	}

	projectName := filepath.Base(repoRoot)
	shortID := sessionID
	if len(sessionID) > 8 {
		shortID = sessionID[:8]
	}

	parentDir := filepath.Dir(repoRoot)

	// Check if parent directory is safe
	if isSafeParentDir(parentDir) {
		// Try sibling directory
		siblingPath := filepath.Join(parentDir, fmt.Sprintf("%s-worktree-%s", projectName, shortID))
		return siblingPath, nil
	}

	// Fallback to .perles/worktrees/
	fallbackPath := filepath.Join(repoRoot, ".perles", "worktrees", sessionID)
	return fallbackPath, nil
}

// isSafeParentDir checks if a directory is safe to use as a worktree parent.
func isSafeParentDir(dir string) bool {
	// Check against known unsafe directories
	if unsafeParentDirs[dir] {
		return false
	}

	// Also check if it starts with common system prefixes on macOS/Linux
	systemPrefixes := []string{"/System/", "/usr/", "/bin/", "/sbin/", "/etc/", "/var/", "/private/"}
	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(dir, prefix) {
			return false
		}
	}

	// Check if directory is writable
	return isWritable(dir)
}

// isWritable checks if a directory is writable.
func isWritable(dir string) bool {
	// Try to create a temp file to check writability
	testFile := filepath.Join(dir, ".perles-write-test")
	//nolint:gosec // G304: testFile path is constructed from dir parameter
	f, err := os.Create(testFile)
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(testFile)
	return true
}

// CreateWorktree creates a new worktree at the specified path.
// If branch is empty, creates a new branch based on HEAD.
func (e *RealExecutor) CreateWorktree(path, newBranch, baseBranch string) error {
	// git worktree add -b <new-branch> <path> [<start-point>]
	// -b creates a new branch; baseBranch is the starting point
	args := []string{"worktree", "add", "-b", newBranch, path}

	if baseBranch != "" {
		// Use specified branch as starting point
		args = append(args, baseBranch)
	}
	// If baseBranch is empty, git uses current HEAD as starting point

	return e.runGit(args...)
}

// RemoveWorktree removes a worktree at the specified path.
func (e *RealExecutor) RemoveWorktree(path string) error {
	// First try normal remove
	err := e.runGit("worktree", "remove", path)
	if err != nil {
		// If it fails, try with --force
		return e.runGit("worktree", "remove", "--force", path)
	}
	return nil
}

// PruneWorktrees removes stale worktree references.
func (e *RealExecutor) PruneWorktrees() error {
	return e.runGit("worktree", "prune")
}

// ListWorktrees returns information about all worktrees.
func (e *RealExecutor) ListWorktrees() ([]WorktreeInfo, error) {
	output, err := e.runGitOutput("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	return parseWorktreeList(output), nil
}

// parseWorktreeList parses the porcelain output of git worktree list.
// Format:
//
//	worktree /path/to/worktree
//	HEAD <sha>
//	branch refs/heads/branch-name
//	<blank line>
func parseWorktreeList(output string) []WorktreeInfo {
	var worktrees []WorktreeInfo
	var current WorktreeInfo

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// End of a worktree entry
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = WorktreeInfo{}
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}

		key, value := parts[0], parts[1]
		switch key {
		case "worktree":
			current.Path = value
		case "HEAD":
			current.HEAD = value
		case "branch":
			// Extract branch name from refs/heads/branch-name
			if after, found := strings.CutPrefix(value, "refs/heads/"); found {
				current.Branch = after
			} else {
				current.Branch = value
			}
		}
	}

	// Don't forget the last entry if output doesn't end with blank line
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees
}

// ListBranches returns all local branches, sorted with current branch first then alphabetically.
func (e *RealExecutor) ListBranches() ([]BranchInfo, error) {
	// git branch --format outputs each branch with consistent formatting
	// %(refname:short) gives the short branch name
	// %(HEAD) gives '*' if current, ' ' otherwise
	output, err := e.runGitOutput("branch", "--format=%(HEAD)%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	if output == "" {
		return nil, nil
	}

	var branches []BranchInfo
	var currentBranch *BranchInfo

	for line := range strings.SplitSeq(output, "\n") {
		if line == "" {
			continue
		}

		// %(HEAD) outputs '*' for current branch, ' ' for others
		// But some git versions may not output the space for non-current branches
		var isCurrent bool
		var name string
		switch line[0] {
		case '*':
			isCurrent = true
			name = line[1:]
		case ' ':
			isCurrent = false
			name = line[1:]
		default:
			// No HEAD marker - treat as non-current branch
			isCurrent = false
			name = line
		}

		branch := BranchInfo{
			Name:      name,
			IsCurrent: isCurrent,
		}

		if isCurrent {
			currentBranch = &branch
		} else {
			branches = append(branches, branch)
		}
	}

	// Sort non-current branches alphabetically
	sort.Slice(branches, func(i, j int) bool {
		return branches[i].Name < branches[j].Name
	})

	// Put current branch first
	if currentBranch != nil {
		branches = append([]BranchInfo{*currentBranch}, branches...)
	}

	return branches, nil
}

// BranchExists checks if a branch with the given name exists.
func (e *RealExecutor) BranchExists(name string) bool {
	err := e.runGit("show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}
