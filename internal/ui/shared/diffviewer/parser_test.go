package diffviewer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseDiff_SingleFile(t *testing.T) {
	input := `diff --git a/file.go b/file.go
index abc1234..def5678 100644
--- a/file.go
+++ b/file.go
@@ -10,6 +10,7 @@ func example() {
 	context line
-	deleted line
+	added line
 	more context
`

	files, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, files, 1)

	f := files[0]
	require.Equal(t, "file.go", f.OldPath)
	require.Equal(t, "file.go", f.NewPath)
	require.Equal(t, 1, f.Additions)
	require.Equal(t, 1, f.Deletions)
	require.False(t, f.IsBinary)
	require.False(t, f.IsRenamed)
	require.False(t, f.IsNew)
	require.False(t, f.IsDeleted)
	require.Len(t, f.Hunks, 1)

	h := f.Hunks[0]
	require.Equal(t, 10, h.OldStart)
	require.Equal(t, 6, h.OldCount)
	require.Equal(t, 10, h.NewStart)
	require.Equal(t, 7, h.NewCount)
	require.Contains(t, h.Header, "@@ -10,6 +10,7 @@")

	// Verify lines: hunk header + context + deletion + addition + context
	require.GreaterOrEqual(t, len(h.Lines), 4)

	// Check for deletion
	var hasDeletion, hasAddition bool
	for _, line := range h.Lines {
		if line.Type == LineDeletion {
			hasDeletion = true
			require.Contains(t, line.Content, "deleted line")
			require.Greater(t, line.OldLineNum, 0)
			require.Equal(t, 0, line.NewLineNum)
		}
		if line.Type == LineAddition {
			hasAddition = true
			require.Contains(t, line.Content, "added line")
			require.Equal(t, 0, line.OldLineNum)
			require.Greater(t, line.NewLineNum, 0)
		}
	}
	require.True(t, hasDeletion, "should have deletion line")
	require.True(t, hasAddition, "should have addition line")
}

func TestParseDiff_MultipleFiles(t *testing.T) {
	input := `diff --git a/first.go b/first.go
--- a/first.go
+++ b/first.go
@@ -1,3 +1,4 @@
 line one
+added
 line two
diff --git a/second.go b/second.go
--- a/second.go
+++ b/second.go
@@ -5,2 +5,1 @@
-removed
 kept
`

	files, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, files, 2)

	require.Equal(t, "first.go", files[0].OldPath)
	require.Equal(t, 1, files[0].Additions)
	require.Equal(t, 0, files[0].Deletions)

	require.Equal(t, "second.go", files[1].OldPath)
	require.Equal(t, 0, files[1].Additions)
	require.Equal(t, 1, files[1].Deletions)
}

func TestParseDiff_BinaryFile(t *testing.T) {
	input := `diff --git a/image.png b/image.png
index abc1234..def5678 100644
Binary files a/image.png and b/image.png differ
`

	files, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, files, 1)

	f := files[0]
	require.Equal(t, "image.png", f.OldPath)
	require.Equal(t, "image.png", f.NewPath)
	require.True(t, f.IsBinary)
	require.Len(t, f.Hunks, 0) // Binary files have no hunks
}

func TestParseDiff_RenamedFile(t *testing.T) {
	input := `diff --git a/old_name.go b/new_name.go
similarity index 95%
rename from old_name.go
rename to new_name.go
index abc1234..def5678 100644
--- a/old_name.go
+++ b/new_name.go
@@ -10,3 +10,3 @@ func foo() {
 context
-old content
+new content
`

	files, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, files, 1)

	f := files[0]
	require.Equal(t, "old_name.go", f.OldPath)
	require.Equal(t, "new_name.go", f.NewPath)
	require.True(t, f.IsRenamed)
	require.Equal(t, 95, f.Similarity)
	require.False(t, f.IsBinary)
}

func TestParseDiff_NewFile(t *testing.T) {
	input := `diff --git a/newfile.go b/newfile.go
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/newfile.go
@@ -0,0 +1,5 @@
+package main
+
+func hello() {
+	println("hello")
+}
`

	files, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, files, 1)

	f := files[0]
	require.True(t, f.IsNew)
	require.False(t, f.IsDeleted)
	require.Equal(t, "/dev/null", f.OldPath)
	require.Equal(t, "newfile.go", f.NewPath)
	require.Equal(t, 5, f.Additions)
	require.Equal(t, 0, f.Deletions)
}

func TestParseDiff_DeletedFile(t *testing.T) {
	input := `diff --git a/removed.go b/removed.go
deleted file mode 100644
index abc1234..0000000
--- a/removed.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
-
-func old() {}
`

	files, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, files, 1)

	f := files[0]
	require.True(t, f.IsDeleted)
	require.False(t, f.IsNew)
	require.Equal(t, "removed.go", f.OldPath)
	require.Equal(t, "/dev/null", f.NewPath)
	require.Equal(t, 0, f.Additions)
	require.Equal(t, 3, f.Deletions)
}

func TestParseDiff_Empty(t *testing.T) {
	input := ""

	files, err := parseDiff(input)
	require.NoError(t, err)
	require.Nil(t, files)
	require.Len(t, files, 0)
}

func TestParseDiff_HunkHeader(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		oldStart int
		oldCount int
		newStart int
		newCount int
	}{
		{
			name: "standard hunk",
			input: `diff --git a/f.go b/f.go
--- a/f.go
+++ b/f.go
@@ -10,20 +15,25 @@ func context
 line
`,
			oldStart: 10,
			oldCount: 20,
			newStart: 15,
			newCount: 25,
		},
		{
			name: "single line old",
			input: `diff --git a/f.go b/f.go
--- a/f.go
+++ b/f.go
@@ -5 +5,3 @@ context
 line
`,
			oldStart: 5,
			oldCount: 1,
			newStart: 5,
			newCount: 3,
		},
		{
			name: "single line new",
			input: `diff --git a/f.go b/f.go
--- a/f.go
+++ b/f.go
@@ -10,5 +10 @@ context
 line
`,
			oldStart: 10,
			oldCount: 5,
			newStart: 10,
			newCount: 1,
		},
		{
			name: "zero count old",
			input: `diff --git a/f.go b/f.go
--- a/f.go
+++ b/f.go
@@ -0,0 +1,10 @@ context
+line
`,
			oldStart: 0,
			oldCount: 0,
			newStart: 1,
			newCount: 10,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			files, err := parseDiff(tc.input)
			require.NoError(t, err)
			require.Len(t, files, 1)
			require.Len(t, files[0].Hunks, 1)

			h := files[0].Hunks[0]
			require.Equal(t, tc.oldStart, h.OldStart, "old start")
			require.Equal(t, tc.oldCount, h.OldCount, "old count")
			require.Equal(t, tc.newStart, h.NewStart, "new start")
			require.Equal(t, tc.newCount, h.NewCount, "new count")
		})
	}
}

func TestParseDiff_PermissionChange(t *testing.T) {
	input := `diff --git a/script.sh b/script.sh
old mode 100644
new mode 100755
index abc1234..abc1234 100755
--- a/script.sh
+++ b/script.sh
@@ -1,3 +1,3 @@
 #!/bin/bash
-echo "old"
+echo "new"
`

	files, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, files, 1)

	f := files[0]
	require.Equal(t, "script.sh", f.OldPath)
	require.Equal(t, "script.sh", f.NewPath)
	// Permission changes don't affect IsBinary/IsNew/IsDeleted flags
	require.False(t, f.IsBinary)
}

func TestParseDiff_NoNewlineAtEndOfFile(t *testing.T) {
	input := `diff --git a/file.txt b/file.txt
--- a/file.txt
+++ b/file.txt
@@ -1,2 +1,2 @@
 line one
-line two
\ No newline at end of file
+line two with newline
`

	files, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, 1, files[0].Additions)
	require.Equal(t, 1, files[0].Deletions)
}

func TestParseDiff_MalformedInput_NoError(t *testing.T) {
	// Parser should be resilient to garbage input, not panic
	inputs := []string{
		"random garbage",
		"@@ this is not a valid hunk",
		"diff --git incomplete",
		"--- only old file",
		"+++ only new file",
		"\n\n\n",
		"some text\nwith\nnewlines",
	}

	for _, input := range inputs {
		t.Run(input[:min(20, len(input))], func(t *testing.T) {
			// Should not panic
			_, err := parseDiff(input)
			// Error is acceptable, panic is not
			_ = err
		})
	}
}

func TestParseDiff_LineNumbers(t *testing.T) {
	input := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -5,5 +5,6 @@ func foo() {
 line5
 line6
-line7deleted
+line7added
+line7.5new
 line8
`

	files, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Len(t, files[0].Hunks, 1)

	h := files[0].Hunks[0]
	// Lines should track: hunk header, context(5), context(6), deletion(7), addition(7), addition(8), context(8->9)

	// Find specific lines and verify their numbers
	for _, line := range h.Lines {
		switch {
		case line.Type == LineContext && line.Content == "line5":
			require.Equal(t, 5, line.OldLineNum)
			require.Equal(t, 5, line.NewLineNum)
		case line.Type == LineDeletion && line.Content == "line7deleted":
			require.Equal(t, 7, line.OldLineNum)
			require.Equal(t, 0, line.NewLineNum)
		case line.Type == LineAddition && line.Content == "line7added":
			require.Equal(t, 0, line.OldLineNum)
			require.Equal(t, 7, line.NewLineNum)
		case line.Type == LineAddition && line.Content == "line7.5new":
			require.Equal(t, 0, line.OldLineNum)
			require.Equal(t, 8, line.NewLineNum)
		}
	}
}

func TestParseDiff_MultipleHunks(t *testing.T) {
	input := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@ func first
 a
+b
 c
@@ -10,2 +11,3 @@ func second
 x
+y
+z
`

	files, err := parseDiff(input)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Len(t, files[0].Hunks, 2)

	require.Equal(t, 1, files[0].Hunks[0].OldStart)
	require.Equal(t, 10, files[0].Hunks[1].OldStart)

	// Total additions: 1 from first hunk + 2 from second hunk = 3
	require.Equal(t, 3, files[0].Additions)
}
