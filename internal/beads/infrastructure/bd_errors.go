package infrastructure

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// bdExitMigrationFrozen is bd's exit code for a write refused because a
// MIGRATION-FREEZE marker is active (cmd/bd/errors.go ExitMigrationFrozen).
const bdExitMigrationFrozen = 14

// maxBDMessageRunes caps bd's text in an error so it stays toast-sized.
const maxBDMessageRunes = 500

// Hints prefixed to bd's own text for failure classes perles can name.
const (
	hintMigrate           = "beads database needs a schema migration: run `bd migrate schema`"
	hintMigrateCoordinate = "beads database needs a schema migration coordinated with its remote: follow bd's guidance"
	hintUpgradeBD         = "bd is older than the beads database: upgrade bd"
	hintFrozen            = "workspace is frozen for a migration: retry once it finishes"
	hintDolt              = "Dolt ≥ 2.6.0 required: upgrade dolt (and restart the Dolt server)"
	hintTimeout           = "bd may be waiting on a workspace lock or migrating the database; retry shortly"
)

// bdError is a failed bd invocation. Its message keeps bd's own explanation
// (flattened to one line) behind an actionable hint for known failure classes.
type bdError struct {
	// Command is the bd command that failed, e.g. "update" or "comments add".
	Command string
	// Message is bd's explanation, extracted from stderr or JSON output.
	Message string
	// Hint is perles' actionable hint for a known failure class, or "".
	Hint string
	// ExitCode is bd's exit status (-1 if bd did not run to completion).
	ExitCode int
	// TimedOut reports that bd was killed after Timeout.
	TimedOut bool
	Timeout  time.Duration

	cause error
}

func (e *bdError) Error() string {
	var b strings.Builder
	b.WriteString("bd ")
	b.WriteString(e.Command)
	if e.TimedOut {
		fmt.Fprintf(&b, " timed out after %s", e.Timeout)
	} else {
		b.WriteString(" failed")
	}
	// "bd <cmd> failed: <hint> — <bd's text>", or "...: <bd's text>" without a hint.
	detail := e.Message
	if detail == "" && e.cause != nil && !e.TimedOut {
		detail = e.cause.Error()
	}
	if e.Hint != "" {
		b.WriteString(": ")
		b.WriteString(e.Hint)
		if detail != "" {
			b.WriteString(" — ")
			b.WriteString(detail)
		}
	} else if detail != "" {
		b.WriteString(": ")
		b.WriteString(detail)
	}
	return b.String()
}

func (e *bdError) Unwrap() error { return e.cause }

// newBDError builds the error for a failed invocation of bd with args.
func newBDError(args []string, out bdOutput, timeout time.Duration) *bdError {
	e := &bdError{
		Command:  bdCommandName(args),
		ExitCode: out.ExitCode,
		TimedOut: out.TimedOut,
		Timeout:  timeout,
		cause:    out.Err,
	}
	if out.Err != nil && errors.Is(out.Err, exec.ErrNotFound) {
		e.Message = "bd CLI not found on PATH"
		return e
	}

	failure := parseBDFailure(out.Stdout, out.Stderr)
	message := failure.message()
	if out.TimedOut {
		e.Hint = hintTimeout
	} else {
		e.Hint = classifyBDFailure(failure, out.ExitCode)
		if e.Hint == "" && len(failure.hints) > 0 {
			// bd's own hint is only worth repeating when perles has none:
			// on the classified refusals it is a long agent directive.
			message = joinNonEmpty(message, "Hint: "+strings.Join(failure.hints, " "))
		}
	}
	e.Message = truncateRunes(collapseWhitespace(message), maxBDMessageRunes)
	if e.Message == "" && out.Err == nil && !out.TimedOut {
		e.Message = fmt.Sprintf("exit status %d", out.ExitCode)
	}
	return e
}

// bdCommandName names the bd command in args for error messages.
func bdCommandName(args []string) string {
	if len(args) == 0 {
		return "command"
	}
	if len(args) > 1 && args[1] == "add" && (args[0] == "comments" || args[0] == "dep") {
		return args[0] + " add"
	}
	return args[0]
}

// bdFailure is what perles could read out of a failed invocation's output.
type bdFailure struct {
	// prose holds the non-JSON stderr lines, prefixes such as "Error: " removed.
	prose []string
	// errors holds error/message strings from JSON error objects (the
	// per-issue failed[].error entries when present, else error or message).
	errors []string
	// hints holds hint strings from JSON error objects.
	hints []string
	// keys holds the top-level keys of JSON error objects (e.g. migrate_consent).
	keys map[string]bool
}

// parseBDFailure reads bd's explanation from a failed invocation. stderr is
// read in full (prose lines and JSON objects, compact or indented); stdout is
// read for JSON error objects only, which is where --json sends flag and
// argument errors (HandleErrorRespectJSON).
func parseBDFailure(stdout, stderr string) bdFailure {
	f := bdFailure{keys: map[string]bool{}}
	f.scan(stderr, true)
	f.scan(stdout, false)
	return f
}

// scan walks s line by line, decoding any JSON object that starts a line and
// collecting the remaining lines as prose when keepProse is set.
func (f *bdFailure) scan(s string, keepProse bool) {
	for s != "" {
		line, rest, _ := strings.Cut(s, "\n")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{") {
			start := strings.Index(s, "{")
			dec := json.NewDecoder(strings.NewReader(s[start:]))
			var obj map[string]any
			if err := dec.Decode(&obj); err == nil {
				f.addJSON(obj)
				s = s[start+int(dec.InputOffset()):]
				continue
			}
		}
		if keepProse {
			if p := cleanProseLine(trimmed); p != "" {
				f.prose = append(f.prose, p)
			}
		}
		s = rest
	}
}

// addJSON records an error object, unwrapping the BD_JSON_ENVELOPE shape.
// Objects without error, message or failed keys (e.g. an issue printed on
// stdout before the failure) are ignored.
func (f *bdFailure) addJSON(obj map[string]any) {
	if data, ok := obj["data"].(map[string]any); ok {
		if _, versioned := obj["schema_version"]; versioned {
			for k, v := range obj {
				if k != "data" && k != "schema_version" {
					data[k] = v
				}
			}
			obj = data
		}
	}
	_, hasError := obj["error"]
	_, hasMessage := obj["message"]
	_, hasFailed := obj["failed"]
	if !hasError && !hasMessage && !hasFailed {
		return
	}
	for k := range obj {
		f.keys[k] = true
	}

	var perIssue []string
	if failed, ok := obj["failed"].([]any); ok {
		for _, item := range failed {
			if m, ok := item.(map[string]any); ok {
				if s := jsonString(m["error"]); s != "" {
					perIssue = append(perIssue, s)
				}
			}
		}
	}
	switch {
	case len(perIssue) > 0:
		f.errors = append(f.errors, perIssue...)
	case jsonString(obj["error"]) != "":
		f.errors = append(f.errors, jsonString(obj["error"]))
	case jsonString(obj["message"]) != "":
		f.errors = append(f.errors, jsonString(obj["message"]))
	}
	if h := jsonString(obj["hint"]); h != "" {
		f.hints = append(f.hints, h)
	}
}

// message joins the prose with the JSON errors the prose does not already
// say (bd often prints a refusal as prose and again in a JSON summary).
func (f bdFailure) message() string {
	parts := append([]string(nil), f.prose...)
	for _, msg := range f.errors {
		if !textCovers(strings.Join(parts, " "), msg) {
			parts = append(parts, msg)
		}
	}
	return collapseWhitespace(strings.Join(parts, " "))
}

// text is everything bd said, for classification.
func (f bdFailure) text() string {
	return strings.Join(append(append(append([]string(nil), f.prose...), f.errors...), f.hints...), " ")
}

// textCovers reports whether have already contains msg, or msg minus a
// leading "context: " prefix (e.g. "updating issue: <refusal>").
func textCovers(have, msg string) bool {
	if have == "" {
		return false
	}
	msg = collapseWhitespace(msg)
	have = collapseWhitespace(have)
	if strings.Contains(have, msg) {
		return true
	}
	if _, tail, ok := strings.Cut(msg, ": "); ok && tail != "" {
		return strings.Contains(have, tail)
	}
	return false
}

// classifyBDFailure returns an actionable hint for failure classes perles can
// name: a migration freeze, a bd older than the database, a database that
// needs a schema migration, or a Dolt server that is too old.
func classifyBDFailure(f bdFailure, exitCode int) string {
	lower := strings.ToLower(f.text())
	switch {
	case exitCode == bdExitMigrationFrozen,
		strings.Contains(lower, "frozen for migration"),
		strings.Contains(lower, "migration-freeze"):
		return hintFrozen
	case f.keys["schema_skew"],
		strings.Contains(lower, "binary knows up to"),
		strings.Contains(lower, "schema version mismatch") && strings.Contains(lower, " ahead"):
		return hintUpgradeBD
	case f.keys["remote_migrate_gate"],
		strings.Contains(lower, "remote-backed"),
		strings.Contains(lower, "shared server database"):
		if strings.Contains(lower, "adopt") || strings.Contains(lower, "forked") {
			return hintMigrateCoordinate
		}
		return hintMigrate
	case strings.Contains(lower, "gateway server owns"):
		// The gateway's administrator migrates it, and bd's text says so.
		return ""
	case f.keys["migrate_consent"],
		containsAny(lower,
			"without explicit consent",
			"bd migrate schema",
			"pending migration",
			"pending schema migration",
			"binary expects v"):
		return hintMigrate
	case strings.Contains(lower, "bd requires dolt"),
		strings.Contains(lower, "dolt") && strings.Contains(lower, "2.6.0") &&
			containsAny(lower, "or newer", "older than", "below", "upgrade"):
		return hintDolt
	}
	return ""
}

// isNotesOverwriteRefusal reports whether err is bd refusing to replace
// existing notes without --force (beads >= 1.3.2 notes-overwrite fence).
func isNotesOverwriteRefusal(err error) bool {
	e, ok := errors.AsType[*bdError](err)
	if !ok {
		return false
	}
	lower := strings.ToLower(e.Message)
	return containsAny(lower, "notes_overwrite", "append-notes", "would replace existing notes") ||
		(strings.Contains(lower, "--force") && strings.Contains(lower, "notes"))
}

// isUnknownFlag reports whether err is bd rejecting flag as unknown (the
// flag predates or postdates the installed bd).
func isUnknownFlag(err error, flag string) bool {
	e, ok := errors.AsType[*bdError](err)
	if !ok {
		return false
	}
	lower := strings.ToLower(e.Message)
	return strings.Contains(lower, "unknown flag") && strings.Contains(lower, strings.ToLower(flag))
}

// isUnknownCommand reports whether err is bd rejecting name as an unknown command.
func isUnknownCommand(err error, name string) bool {
	e, ok := errors.AsType[*bdError](err)
	if !ok {
		return false
	}
	return strings.Contains(strings.ToLower(e.Message), `unknown command "`+strings.ToLower(name)+`"`)
}

// cleanProseLine strips bd's error-line decorations and drops lines that
// carry no information for a perles user.
func cleanProseLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "note: bd --json output format will change") ||
		(strings.HasPrefix(lower, "run '") && strings.Contains(lower, "--help")) {
		return ""
	}
	for _, prefix := range []string{"⛔ ERROR:", "ERROR:", "Error:", "error:"} {
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			return strings.TrimSpace(rest)
		}
	}
	return line
}

func jsonString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func joinNonEmpty(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + " " + b
	}
}
