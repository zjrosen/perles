package diffviewer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// Regex patterns for diff parsing
	diffHeaderRegex      = regexp.MustCompile(`^diff --git a/(.+) b/(.+)$`)
	hunkHeaderRegex      = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)
	oldFileRegex         = regexp.MustCompile(`^--- a/(.+)$`)
	newFileRegex         = regexp.MustCompile(`^\+\+\+ b/(.+)$`)
	oldFileNullRegex     = regexp.MustCompile(`^--- /dev/null$`)
	newFileNullRegex     = regexp.MustCompile(`^\+\+\+ /dev/null$`)
	similarityRegex      = regexp.MustCompile(`^similarity index (\d+)%$`)
	renameFromRegex      = regexp.MustCompile(`^rename from (.+)$`)
	renameToRegex        = regexp.MustCompile(`^rename to (.+)$`)
	binaryFilesRegex     = regexp.MustCompile(`^Binary files .+ and .+ differ$`)
	oldModeRegex         = regexp.MustCompile(`^old mode (\d+)$`)
	newModeRegex         = regexp.MustCompile(`^new mode (\d+)$`)
	indexLineRegex       = regexp.MustCompile(`^index [a-f0-9]+\.\.[a-f0-9]+`)
	newFileModeRegex     = regexp.MustCompile(`^new file mode (\d+)$`)
	deletedFileModeRegex = regexp.MustCompile(`^deleted file mode (\d+)$`)
)

// parseDiff parses unified diff output into structured DiffFile slices.
// It handles standard unified diff format including edge cases like:
// - Binary files
// - Renamed files with similarity index
// - New files (--- /dev/null)
// - Deleted files (+++ /dev/null)
// - Permission changes (old mode / new mode)
func parseDiff(output string) ([]DiffFile, error) {
	if output == "" {
		return nil, nil
	}

	var files []DiffFile
	lines := strings.Split(output, "\n")
	var currentFile *DiffFile
	var currentHunk *DiffHunk
	oldLineNum := 0
	newLineNum := 0

	for _, line := range lines {

		// Start of a new file diff
		if matches := diffHeaderRegex.FindStringSubmatch(line); matches != nil {
			// Save previous file if exists
			if currentFile != nil {
				if currentHunk != nil {
					currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
					currentHunk = nil
				}
				files = append(files, *currentFile)
			}

			currentFile = &DiffFile{
				OldPath: matches[1],
				NewPath: matches[2],
			}
			currentHunk = nil
			continue
		}

		if currentFile == nil {
			continue
		}

		// Check for old file header
		if oldFileNullRegex.MatchString(line) {
			currentFile.IsNew = true
			currentFile.OldPath = "/dev/null"
			continue
		}
		if matches := oldFileRegex.FindStringSubmatch(line); matches != nil {
			currentFile.OldPath = matches[1]
			continue
		}

		// Check for new file header
		if newFileNullRegex.MatchString(line) {
			currentFile.IsDeleted = true
			currentFile.NewPath = "/dev/null"
			continue
		}
		if matches := newFileRegex.FindStringSubmatch(line); matches != nil {
			currentFile.NewPath = matches[1]
			continue
		}

		// Check for similarity index (renames)
		if matches := similarityRegex.FindStringSubmatch(line); matches != nil {
			similarity, err := strconv.Atoi(matches[1])
			if err == nil {
				currentFile.Similarity = similarity
				currentFile.IsRenamed = true
			}
			continue
		}

		// Check for rename from/to
		if matches := renameFromRegex.FindStringSubmatch(line); matches != nil {
			currentFile.OldPath = matches[1]
			currentFile.IsRenamed = true
			continue
		}
		if matches := renameToRegex.FindStringSubmatch(line); matches != nil {
			currentFile.NewPath = matches[1]
			currentFile.IsRenamed = true
			continue
		}

		// Check for binary files
		if binaryFilesRegex.MatchString(line) {
			currentFile.IsBinary = true
			continue
		}

		// Check for new file mode (marks new files)
		if newFileModeRegex.MatchString(line) {
			currentFile.IsNew = true
			continue
		}

		// Check for deleted file mode (marks deleted files)
		if deletedFileModeRegex.MatchString(line) {
			currentFile.IsDeleted = true
			continue
		}

		// Skip mode changes and index lines (not needed for display)
		if oldModeRegex.MatchString(line) || newModeRegex.MatchString(line) || indexLineRegex.MatchString(line) {
			continue
		}

		// Parse hunk header
		if matches := hunkHeaderRegex.FindStringSubmatch(line); matches != nil {
			// Save previous hunk if exists
			if currentHunk != nil {
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			}

			oldStart, err := strconv.Atoi(matches[1])
			if err != nil {
				return nil, fmt.Errorf("invalid old start line in hunk header: %s", line)
			}

			oldCount := 1
			if matches[2] != "" {
				oldCount, err = strconv.Atoi(matches[2])
				if err != nil {
					return nil, fmt.Errorf("invalid old count in hunk header: %s", line)
				}
			}

			newStart, err := strconv.Atoi(matches[3])
			if err != nil {
				return nil, fmt.Errorf("invalid new start line in hunk header: %s", line)
			}

			newCount := 1
			if matches[4] != "" {
				newCount, err = strconv.Atoi(matches[4])
				if err != nil {
					return nil, fmt.Errorf("invalid new count in hunk header: %s", line)
				}
			}

			currentHunk = &DiffHunk{
				OldStart: oldStart,
				OldCount: oldCount,
				NewStart: newStart,
				NewCount: newCount,
				Header:   line,
				Lines: []DiffLine{{
					Type:    LineHunkHeader,
					Content: strings.TrimSpace(matches[5]),
				}},
			}
			oldLineNum = oldStart
			newLineNum = newStart
			continue
		}

		// Parse diff content lines
		if currentHunk == nil {
			continue
		}

		if len(line) == 0 {
			// Empty line in diff context (treat as context with empty content)
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:       LineContext,
				OldLineNum: oldLineNum,
				NewLineNum: newLineNum,
				Content:    "",
			})
			oldLineNum++
			newLineNum++
			continue
		}

		prefix := line[0]
		content := ""
		if len(line) > 1 {
			content = line[1:]
		}

		switch prefix {
		case ' ':
			// Context line
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:       LineContext,
				OldLineNum: oldLineNum,
				NewLineNum: newLineNum,
				Content:    content,
			})
			oldLineNum++
			newLineNum++
		case '-':
			// Deletion
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:       LineDeletion,
				OldLineNum: oldLineNum,
				NewLineNum: 0,
				Content:    content,
			})
			currentFile.Deletions++
			oldLineNum++
		case '+':
			// Addition
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:       LineAddition,
				OldLineNum: 0,
				NewLineNum: newLineNum,
				Content:    content,
			})
			currentFile.Additions++
			newLineNum++
		case '\\':
			// "\ No newline at end of file" - skip but don't error
			continue
		default:
			// Unknown prefix - could be end of hunk or malformed input
			// Don't error on this, just skip the line
			continue
		}
	}

	// Save final file and hunk
	if currentFile != nil {
		if currentHunk != nil {
			currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
		}
		files = append(files, *currentFile)
	}

	return files, nil
}
