package git

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
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

	// ErrDetachedHead indicates HEAD is not pointing to a branch (detached HEAD state).
	ErrDetachedHead = errors.New("detached HEAD state")
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
// Returns ErrDetachedHead if HEAD is not pointing to a branch (common in CI).
func (e *RealExecutor) GetCurrentBranch() (string, error) {
	// First try git branch --show-current (git 2.22+)
	// This returns empty string in detached HEAD state (no error)
	output, err := e.runGitOutput("branch", "--show-current")
	if err == nil && output != "" {
		return output, nil
	}

	// Fallback: parse symbolic-ref
	output, err = e.runGitOutput("symbolic-ref", "--short", "HEAD")
	if err != nil {
		// Check if we're in detached HEAD state
		if strings.Contains(err.Error(), "not a symbolic ref") {
			return "", ErrDetachedHead
		}
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

// IsOnMainBranch returns true if the current branch is the main branch.
// Returns false (not error) if in detached HEAD state.
func (e *RealExecutor) IsOnMainBranch() (bool, error) {
	currentBranch, err := e.GetCurrentBranch()
	if err != nil {
		if errors.Is(err, ErrDetachedHead) {
			return false, nil
		}
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

// ErrDiffTimeout is returned when a git diff operation times out.
var ErrDiffTimeout = errors.New("git diff timed out")

// diffTimeout is the maximum time allowed for diff operations to prevent hanging.
const diffTimeout = 5 * time.Second

// runGitOutputWithContext executes a git command with a context for timeout support.
func (e *RealExecutor) runGitOutputWithContext(ctx context.Context, args ...string) (string, error) {
	//nolint:gosec // G204: args come from controlled sources
	cmd := exec.CommandContext(ctx, "git", args...)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Check if the error is due to context timeout/cancellation
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("%w: git %s", ErrDiffTimeout, strings.Join(args, " "))
		}

		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return "", parseGitError(stderrStr, err)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GetDiff returns the unified diff output for the given ref.
// Uses a 5-second timeout to prevent hanging on large repos.
func (e *RealExecutor) GetDiff(ref string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), diffTimeout)
	defer cancel()
	return e.runGitOutputWithContext(ctx, "diff", ref)
}

// GetDiffStat returns the --numstat output for the given ref.
// Format: "additions\tdeletions\tpath" per line.
// Uses a 5-second timeout to prevent hanging on large repos.
func (e *RealExecutor) GetDiffStat(ref string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), diffTimeout)
	defer cancel()
	return e.runGitOutputWithContext(ctx, "diff", "--numstat", ref)
}

// GetFileDiff returns the diff for a single file against the given ref.
// Uses a 5-second timeout to prevent hanging on large repos.
func (e *RealExecutor) GetFileDiff(ref, path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), diffTimeout)
	defer cancel()
	return e.runGitOutputWithContext(ctx, "diff", ref, "--", path)
}

// GetWorkingDirDiff returns the diff of uncommitted changes (staged + unstaged vs HEAD).
// Uses a 5-second timeout to prevent hanging on large repos.
func (e *RealExecutor) GetWorkingDirDiff() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), diffTimeout)
	defer cancel()
	return e.runGitOutputWithContext(ctx, "diff", "HEAD")
}

// GetUntrackedFiles returns the list of untracked files (new files not yet staged).
// Uses a 5-second timeout to prevent hanging on large repos.
func (e *RealExecutor) GetUntrackedFiles() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), diffTimeout)
	defer cancel()

	output, err := e.runGitOutputWithContext(ctx, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	if output == "" {
		return nil, nil
	}

	// Split by newlines and filter empty strings
	lines := strings.Split(output, "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// GetCommitDiff returns the diff for a specific commit (what changed in that commit).
// Uses git show to get the commit's patch.
// Uses a 5-second timeout to prevent hanging on large repos.
func (e *RealExecutor) GetCommitDiff(hash string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), diffTimeout)
	defer cancel()
	// Use --format= to suppress commit metadata, only show the diff
	return e.runGitOutputWithContext(ctx, "show", "--format=", hash)
}

// GetFileContent returns the content of a file in the working directory.
// Used for displaying untracked files that have no diff.
func (e *RealExecutor) GetFileContent(path string) (string, error) {
	fullPath := path
	if e.workDir != "" && !filepath.IsAbs(path) {
		fullPath = filepath.Join(e.workDir, path)
	}

	//nolint:gosec // G304: path comes from git ls-files output, not user input
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", path, err)
	}

	return string(content), nil
}

// commitLogDelimiter is used to separate fields in git log output.
// Using ASCII Record Separator (0x1E) which is extremely unlikely to appear in commit messages.
const commitLogDelimiter = "\x1e"

// GetCommitLog returns the most recent commits, up to the specified limit.
// Uses a 5-second timeout to prevent hanging on large repos.
// Returns an empty slice for empty repositories.
// Also detects which commits have been pushed to the remote tracking branch.
func (e *RealExecutor) GetCommitLog(limit int) ([]CommitInfo, error) {
	return e.GetCommitLogForRef("", limit)
}

// GetCommitLogForRef returns commit history for a specific ref (branch, tag, etc.).
// If ref is empty, returns commits for HEAD.
// Uses a 5-second timeout to prevent hanging on large repos.
// Returns an empty slice for empty repositories.
// Also detects which commits have been pushed to the remote tracking branch.
func (e *RealExecutor) GetCommitLogForRef(ref string, limit int) ([]CommitInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), diffTimeout)
	defer cancel()

	// Format: full_hash<RS>short_hash<RS>subject<RS>author<RS>ISO_date
	// Using ASCII Record Separator (0x1E) to avoid issues with commit messages
	args := []string{"log", "--format=%H\x1e%h\x1e%s\x1e%an\x1e%aI", "-n", strconv.Itoa(limit)}
	if ref != "" {
		args = append(args, ref)
	}

	output, err := e.runGitOutputWithContext(ctx, args...)
	if err != nil {
		errStr := err.Error()
		// Empty repo or no commits returns empty slice, not error
		if strings.Contains(errStr, "does not have any commits") {
			return nil, nil
		}
		// Invalid ref should return error (only treat as empty for HEAD/empty ref)
		if ref == "" && (strings.Contains(errStr, "bad revision") || strings.Contains(errStr, "unknown revision")) {
			return nil, nil
		}
		return nil, err
	}

	if output == "" {
		return nil, nil
	}

	commits := parseCommitLog(output)

	// Determine which commits are pushed by finding the remote tracking branch tip
	pushedHashes := e.getPushedCommitHashes(ctx)
	for i := range commits {
		_, commits[i].IsPushed = pushedHashes[commits[i].Hash]
	}

	return commits, nil
}

// getPushedCommitHashes returns a set of commit hashes that exist on the remote tracking branch.
// Returns empty map if no remote tracking branch exists or on error.
func (e *RealExecutor) getPushedCommitHashes(ctx context.Context) map[string]struct{} {
	result := make(map[string]struct{})

	// Get the upstream tracking branch (e.g., origin/main)
	upstream, err := e.runGitOutputWithContext(ctx, "rev-parse", "--abbrev-ref", "@{upstream}")
	if err != nil {
		// No upstream configured - all commits are "unpushed"
		return result
	}

	// Get commits that are on the remote (use merge-base to find common ancestor,
	// then all commits from there back are "pushed")
	// Alternative: list commits reachable from upstream
	output, err := e.runGitOutputWithContext(ctx, "log", "--format=%H", upstream)
	if err != nil {
		return result
	}

	for line := range strings.SplitSeq(output, "\n") {
		if line != "" {
			result[line] = struct{}{}
		}
	}

	return result
}

// parseCommitLog parses the output of git log --format='%H<RS>%h<RS>%s<RS>%an<RS>%aI'.
func parseCommitLog(output string) []CommitInfo {
	var commits []CommitInfo

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Split on Record Separator delimiter: hash<RS>short<RS>subject<RS>author<RS>date
		parts := strings.SplitN(line, commitLogDelimiter, 5)
		if len(parts) < 5 {
			continue // Invalid line format
		}

		hash := parts[0]
		shortHash := parts[1]
		subject := parts[2]
		author := parts[3]
		dateStr := parts[4]

		// Parse ISO 8601 date (e.g., "2024-01-15T10:30:00-05:00")
		date, err := time.Parse(time.RFC3339, dateStr)
		if err != nil {
			// Fallback to zero time if parsing fails
			date = time.Time{}
		}

		commits = append(commits, CommitInfo{
			Hash:      hash,
			ShortHash: shortHash,
			Subject:   subject,
			Author:    author,
			Date:      date,
		})
	}

	return commits
}
