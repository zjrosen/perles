package diffviewer

import (
	"path/filepath"
	"sort"
	"strings"
)

// FileTreeNode represents a node in the file tree (either a directory or file).
type FileTreeNode struct {
	Name     string          // Just the filename/dirname component
	Path     string          // Full path from root
	IsDir    bool            // True if this is a directory
	Expanded bool            // True if directory is expanded (shows children)
	Children []*FileTreeNode // Child nodes (sorted: dirs first, then files, alphabetically)
	File     *DiffFile       // Reference to the DiffFile (nil for directories)
	Depth    int             // Nesting depth (0 for root-level items)
}

// FileTree manages the tree structure for displaying changed files.
type FileTree struct {
	Root         []*FileTreeNode // Top-level nodes
	flattenCache []*FileTreeNode // Cached flattened visible nodes
	cacheValid   bool            // Whether flattenCache is valid
}

// NewFileTree creates a new file tree from a list of diff files.
// All directories start expanded by default.
func NewFileTree(files []DiffFile) *FileTree {
	ft := &FileTree{
		Root: make([]*FileTreeNode, 0),
	}

	// Local map for path tracking during construction
	nodesByPath := make(map[string]*FileTreeNode)

	for i := range files {
		file := &files[i]
		path := file.NewPath
		if file.IsDeleted {
			path = file.OldPath
		}
		ft.addFileWithPathMap(path, file, nodesByPath)
	}

	// Sort all levels
	ft.sortNodes(ft.Root)

	return ft
}

// addFileWithPathMap adds a file to the tree, creating parent directories as needed.
// Uses the provided nodesByPath map for tracking during construction.
func (ft *FileTree) addFileWithPathMap(path string, file *DiffFile, nodesByPath map[string]*FileTreeNode) {
	parts := strings.Split(filepath.ToSlash(path), "/")

	var parent *FileTreeNode
	var currentPath string

	// Create/find directory nodes for all path components except the last (file)
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		if currentPath == "" {
			currentPath = part
		} else {
			currentPath = currentPath + "/" + part
		}

		// Check if directory node already exists
		if existing, ok := nodesByPath[currentPath]; ok {
			parent = existing
			continue
		}

		// Create new directory node
		dirNode := &FileTreeNode{
			Name:     part,
			Path:     currentPath,
			IsDir:    true,
			Expanded: true, // Directories start expanded
			Children: make([]*FileTreeNode, 0),
			Depth:    i,
		}
		nodesByPath[currentPath] = dirNode

		// Add to parent's children or root
		if parent == nil {
			ft.Root = append(ft.Root, dirNode)
		} else {
			parent.Children = append(parent.Children, dirNode)
		}
		parent = dirNode
	}

	// Create file node
	fileName := parts[len(parts)-1]
	fileNode := &FileTreeNode{
		Name:  fileName,
		Path:  path,
		IsDir: false,
		File:  file,
		Depth: len(parts) - 1,
	}
	nodesByPath[path] = fileNode

	// Add file to parent's children or root
	if parent == nil {
		ft.Root = append(ft.Root, fileNode)
	} else {
		parent.Children = append(parent.Children, fileNode)
	}

	ft.cacheValid = false
}

// sortNodes recursively sorts nodes: directories first, then files, alphabetically.
func (ft *FileTree) sortNodes(nodes []*FileTreeNode) {
	sort.Slice(nodes, func(i, j int) bool {
		// Directories come before files
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		// Alphabetical within same type
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})

	// Recursively sort children
	for _, node := range nodes {
		if node.IsDir && len(node.Children) > 0 {
			ft.sortNodes(node.Children)
		}
	}
}

// VisibleNodes returns a flattened list of currently visible nodes.
// Respects expanded/collapsed state of directories.
func (ft *FileTree) VisibleNodes() []*FileTreeNode {
	if ft.cacheValid && ft.flattenCache != nil {
		return ft.flattenCache
	}

	ft.flattenCache = make([]*FileTreeNode, 0)
	ft.flattenNodes(ft.Root, &ft.flattenCache)
	ft.cacheValid = true
	return ft.flattenCache
}

// flattenNodes recursively collects visible nodes.
func (ft *FileTree) flattenNodes(nodes []*FileTreeNode, result *[]*FileTreeNode) {
	for _, node := range nodes {
		*result = append(*result, node)
		if node.IsDir && node.Expanded && len(node.Children) > 0 {
			ft.flattenNodes(node.Children, result)
		}
	}
}

// Toggle toggles the expanded state of a directory node.
// Returns true if state was changed.
func (ft *FileTree) Toggle(node *FileTreeNode) bool {
	if !node.IsDir {
		return false
	}
	node.Expanded = !node.Expanded
	ft.cacheValid = false
	return true
}

// statusIndicatorMap maps fileStatus to single-character indicators.
var statusIndicatorMap = map[fileStatus]string{
	statusModified:  "M",
	statusAdded:     "A",
	statusDeleted:   "D",
	statusRenamed:   "R",
	statusBinary:    "B",
	statusUntracked: "?",
	statusUnknown:   "",
}

// getNodeIndicator returns a status indicator for a file tree node.
// M = Modified, A = Added, D = Deleted, R = Renamed, B = Binary, ? = Untracked
func getNodeIndicator(node *FileTreeNode) string {
	if node.IsDir || node.File == nil {
		return ""
	}
	status := getFileStatus(node.File)
	return statusIndicatorMap[status]
}

// TotalStats calculates total additions and deletions for a node.
// For directories, this sums all descendant files.
func (node *FileTreeNode) TotalStats() (additions, deletions int) {
	if !node.IsDir {
		if node.File != nil {
			return node.File.Additions, node.File.Deletions
		}
		return 0, 0
	}

	for _, child := range node.Children {
		a, d := child.TotalStats()
		additions += a
		deletions += d
	}
	return additions, deletions
}

// FileCount returns the number of files under this node.
// For files, returns 1. For directories, counts all descendant files.
func (node *FileTreeNode) FileCount() int {
	if !node.IsDir {
		return 1
	}

	count := 0
	for _, child := range node.Children {
		count += child.FileCount()
	}
	return count
}

// CollectFiles returns all DiffFiles under this node.
// For files, returns a slice with just the file. For directories, collects all descendant files.
func (node *FileTreeNode) CollectFiles() []*DiffFile {
	if !node.IsDir {
		if node.File != nil {
			return []*DiffFile{node.File}
		}
		return nil
	}

	var files []*DiffFile
	for _, child := range node.Children {
		files = append(files, child.CollectFiles()...)
	}
	return files
}
