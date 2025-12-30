// Package vimtextarea provides a vim-style text editing component.
//
// This file provides grapheme cluster helpers for Unicode-aware text operations.
//
// Triple-Unit Model:
//
// This module distinguishes between three units of text measurement:
//
//  1. Bytes: The underlying storage unit in Go strings (len() returns bytes).
//     A single grapheme can be 1-25+ bytes (e.g., 👨‍👩‍👧‍👦 is 25 bytes).
//
//  2. Graphemes: The logical unit of text that users perceive as a "character".
//     A grapheme cluster may consist of multiple code points (e.g., "e" + combining
//     accent = 1 grapheme). This is what cursorCol tracks.
//
//  3. Display Columns: The width in terminal cells that a grapheme occupies.
//     ASCII = 1 column, emoji = 2 columns, CJK = 2 columns. This is what m.width uses.
//
// All cursor positions (cursorCol, Position.Col) represent grapheme indices,
// not byte offsets. Use the conversion functions in this file to translate
// between units when needed.
package vimtextarea

import (
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// Character type constants for word boundary detection.
const (
	graphemeWhitespace = iota
	graphemeWord
	graphemePunctuation
)

// GraphemeCount returns the number of grapheme clusters in a string.
// For example: "hello" = 5, "h😀llo" = 5, "👨‍👩‍👧‍👦" = 1.
func GraphemeCount(s string) int {
	return uniseg.GraphemeClusterCount(s)
}

// GraphemeAt returns the grapheme cluster at the given grapheme index.
// Returns "" if index is out of bounds or negative.
func GraphemeAt(s string, graphemeIdx int) string {
	if graphemeIdx < 0 {
		return ""
	}

	idx := 0
	state := -1
	for len(s) > 0 {
		cluster, rest, _, newState := uniseg.StepString(s, state)
		if idx == graphemeIdx {
			return cluster
		}
		idx++
		s = rest
		state = newState
	}
	return ""
}

// NthGrapheme returns the nth grapheme cluster (0-indexed) and its byte offset.
// Returns ("", -1) if n is out of bounds or negative.
func NthGrapheme(s string, n int) (cluster string, byteOffset int) {
	if n < 0 {
		return "", -1
	}

	idx := 0
	offset := 0
	state := -1
	original := s
	for len(s) > 0 {
		cluster, rest, _, newState := uniseg.StepString(s, state)
		if idx == n {
			return cluster, offset
		}
		idx++
		offset = len(original) - len(rest)
		s = rest
		state = newState
	}
	return "", -1
}

// GraphemeToByteOffset converts a grapheme index to byte offset.
// Returns len(s) if graphemeIdx >= grapheme count.
// Returns 0 if graphemeIdx <= 0.
func GraphemeToByteOffset(s string, graphemeIdx int) int {
	if graphemeIdx <= 0 {
		return 0
	}

	idx := 0
	state := -1
	original := s
	for len(s) > 0 {
		_, rest, _, newState := uniseg.StepString(s, state)
		idx++
		if idx == graphemeIdx {
			return len(original) - len(rest)
		}
		s = rest
		state = newState
	}
	return len(original)
}

// ByteToGraphemeOffset converts a byte offset to grapheme index.
// If byteOffset falls within a grapheme, returns that grapheme's index.
// Returns 0 if byteOffset <= 0.
// Returns the grapheme count if byteOffset >= len(s).
func ByteToGraphemeOffset(s string, byteOffset int) int {
	if byteOffset <= 0 {
		return 0
	}
	if byteOffset >= len(s) {
		return GraphemeCount(s)
	}

	idx := 0
	currentPos := 0
	state := -1
	for len(s) > 0 {
		cluster, rest, _, newState := uniseg.StepString(s, state)
		nextPos := currentPos + len(cluster)
		if byteOffset < nextPos {
			return idx
		}
		idx++
		currentPos = nextPos
		s = rest
		state = newState
	}
	return idx
}

// SliceByGraphemes returns a substring from grapheme index start to end (exclusive).
// Similar to s[start:end] but grapheme-aware.
// Returns "" for invalid ranges.
func SliceByGraphemes(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end < start {
		return ""
	}

	startByte := GraphemeToByteOffset(s, start)
	endByte := GraphemeToByteOffset(s, end)

	if startByte >= len(s) {
		return ""
	}
	if endByte > len(s) {
		endByte = len(s)
	}

	return s[startByte:endByte]
}

// GraphemeDisplayWidth returns the display width of a single grapheme cluster
// in terminal cells. ASCII = 1, emoji = 2, CJK = 2.
func GraphemeDisplayWidth(cluster string) int {
	if cluster == "" {
		return 0
	}
	return runewidth.StringWidth(cluster)
}

// StringDisplayWidth returns the total display width of a string in terminal cells.
func StringDisplayWidth(s string) int {
	return runewidth.StringWidth(s)
}

// graphemeType returns the type of a grapheme cluster for word boundary detection.
// This is the grapheme-aware replacement for charType(rune).
//
// Classification rules:
//   - Whitespace: space, tab, newline, carriage return
//   - Word: alphanumeric characters, underscore, or non-ASCII letters/numbers
//   - Punctuation: everything else (including emoji)
func graphemeType(cluster string) int {
	if cluster == "" {
		return graphemeWhitespace
	}

	// Check first rune for classification
	// For multi-rune grapheme clusters (emoji, combining marks),
	// we classify based on the base character
	for _, r := range cluster {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			return graphemeWhitespace
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_':
			return graphemeWord
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			// Non-ASCII letters and numbers are word characters
			return graphemeWord
		default:
			return graphemePunctuation
		}
	}
	return graphemePunctuation
}

// GraphemeIterator provides efficient iteration over grapheme clusters.
// Use NewGraphemeIterator to create an iterator, then call Next() in a loop.
//
// Example:
//
//	iter := NewGraphemeIterator("hello😀")
//	for iter.Next() {
//	    fmt.Printf("Grapheme %d at byte %d: %q\n", iter.Index(), iter.BytePos(), iter.Cluster())
//	}
type GraphemeIterator struct {
	original string
	rest     string
	state    int
	cluster  string
	bytePos  int
	index    int
	started  bool
}

// NewGraphemeIterator creates a new iterator over grapheme clusters in s.
func NewGraphemeIterator(s string) *GraphemeIterator {
	return &GraphemeIterator{
		original: s,
		rest:     s,
		state:    -1,
		index:    -1,
		started:  false,
	}
}

// Next advances the iterator to the next grapheme cluster.
// Returns false when there are no more grapheme clusters.
func (g *GraphemeIterator) Next() bool {
	if len(g.rest) == 0 {
		return false
	}

	if g.started {
		g.bytePos = len(g.original) - len(g.rest)
		g.index++
	} else {
		g.bytePos = 0
		g.index = 0
		g.started = true
	}

	cluster, rest, _, newState := uniseg.StepString(g.rest, g.state)
	g.cluster = cluster
	g.rest = rest
	g.state = newState

	return true
}

// Cluster returns the current grapheme cluster.
// Returns "" if Next() has not been called or returned false.
func (g *GraphemeIterator) Cluster() string {
	return g.cluster
}

// BytePos returns the byte offset of the current grapheme cluster in the original string.
func (g *GraphemeIterator) BytePos() int {
	return g.bytePos
}

// Index returns the grapheme index of the current cluster (0-indexed).
// Returns -1 if Next() has not been called.
func (g *GraphemeIterator) Index() int {
	return g.index
}

// Reset resets the iterator to the beginning of the string.
func (g *GraphemeIterator) Reset() {
	g.rest = g.original
	g.state = -1
	g.cluster = ""
	g.bytePos = 0
	g.index = -1
	g.started = false
}

// ReverseGraphemeIterator provides efficient backward iteration over grapheme clusters.
// This is useful for backward word motion (b command).
//
// Note: Due to the nature of grapheme cluster segmentation, this iterator
// pre-computes all graphemes and stores them in memory. For very long strings,
// consider using GraphemeIterator with Reset() instead if memory is a concern.
type ReverseGraphemeIterator struct {
	clusters   []graphemeInfo
	currentIdx int
	started    bool
}

type graphemeInfo struct {
	cluster string
	bytePos int
}

// NewReverseGraphemeIterator creates a new reverse iterator over grapheme clusters in s.
func NewReverseGraphemeIterator(s string) *ReverseGraphemeIterator {
	// Pre-compute all graphemes
	var clusters []graphemeInfo
	iter := NewGraphemeIterator(s)
	for iter.Next() {
		clusters = append(clusters, graphemeInfo{
			cluster: iter.Cluster(),
			bytePos: iter.BytePos(),
		})
	}

	return &ReverseGraphemeIterator{
		clusters:   clusters,
		currentIdx: len(clusters), // Start past the end
		started:    false,
	}
}

// Next advances the iterator to the previous grapheme cluster (moving backward).
// Returns false when there are no more grapheme clusters.
func (r *ReverseGraphemeIterator) Next() bool {
	if !r.started {
		r.started = true
		r.currentIdx = len(r.clusters) - 1
	} else {
		r.currentIdx--
	}

	return r.currentIdx >= 0
}

// Cluster returns the current grapheme cluster.
func (r *ReverseGraphemeIterator) Cluster() string {
	if r.currentIdx < 0 || r.currentIdx >= len(r.clusters) {
		return ""
	}
	return r.clusters[r.currentIdx].cluster
}

// BytePos returns the byte offset of the current grapheme cluster in the original string.
func (r *ReverseGraphemeIterator) BytePos() int {
	if r.currentIdx < 0 || r.currentIdx >= len(r.clusters) {
		return 0
	}
	return r.clusters[r.currentIdx].bytePos
}

// Index returns the grapheme index of the current cluster (0-indexed).
func (r *ReverseGraphemeIterator) Index() int {
	return r.currentIdx
}

// GraphemesInRange returns all grapheme clusters between grapheme indices start and end (exclusive).
// Returns a slice of grapheme cluster strings.
func GraphemesInRange(s string, start, end int) []string {
	if start < 0 {
		start = 0
	}
	if end < start {
		return nil
	}

	var result []string
	idx := 0
	state := -1
	for len(s) > 0 {
		cluster, rest, _, newState := uniseg.StepString(s, state)
		if idx >= start && idx < end {
			result = append(result, cluster)
		}
		if idx >= end {
			break
		}
		idx++
		s = rest
		state = newState
	}
	return result
}

// InsertAtGrapheme inserts the given text at the specified grapheme index.
// Returns the resulting string.
func InsertAtGrapheme(s string, graphemeIdx int, insert string) string {
	byteOffset := GraphemeToByteOffset(s, graphemeIdx)
	return s[:byteOffset] + insert + s[byteOffset:]
}

// DeleteGraphemeRange deletes grapheme clusters from start to end (exclusive).
// Returns the resulting string.
func DeleteGraphemeRange(s string, start, end int) string {
	startByte := GraphemeToByteOffset(s, start)
	endByte := GraphemeToByteOffset(s, end)
	return s[:startByte] + s[endByte:]
}

// TruncateToDisplayWidth truncates a string to fit within the given display width.
// Returns the truncated string (will not split grapheme clusters).
func TruncateToDisplayWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	var result strings.Builder
	currentWidth := 0

	iter := NewGraphemeIterator(s)
	for iter.Next() {
		clusterWidth := GraphemeDisplayWidth(iter.Cluster())
		if currentWidth+clusterWidth > maxWidth {
			break
		}
		result.WriteString(iter.Cluster())
		currentWidth += clusterWidth
	}

	return result.String()
}
