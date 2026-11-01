package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// MinBeadsVersion is the minimum beads database version required by perles.
// Update this when perles starts using features from newer beads versions.
//
// 1.0.0 is the first release of the current Dolt era: current bd refuses
// server workspaces last touched by bd 0.55.4 through 0.62.0 until they are
// migrated explicitly (bd's cmd/bd/legacy_upgrade_guard.go).
const MinBeadsVersion = "1.0.0"

// MinEmbeddedBeadsVersion is the minimum bd version perles requires for an
// embedded Dolt workspace, which it reads through bd. bd 1.0.5 added the
// stored is_blocked column (migration 0046_add_is_blocked) that
// `bd blocked --json` lists, so a snapshot's blocked and ready match server
// mode. Server mode keeps MinBeadsVersion.
const MinEmbeddedBeadsVersion = "1.0.5"

// CheckVersion returns nil if current >= MinBeadsVersion, error otherwise.
func CheckVersion(current string) error {
	if CompareVersions(current, MinBeadsVersion) < 0 {
		return fmt.Errorf("beads version %s required, found %s", MinBeadsVersion, current)
	}
	return nil
}

// CompareVersions compares two semver strings (X.Y.Z format).
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func CompareVersions(a, b string) int {
	partsA := strings.Split(strings.TrimPrefix(a, "v"), ".")
	partsB := strings.Split(strings.TrimPrefix(b, "v"), ".")

	for i := range 3 {
		numA, _ := strconv.Atoi(safeIndex(partsA, i))
		numB, _ := strconv.Atoi(safeIndex(partsB, i))
		if numA < numB {
			return -1
		}
		if numA > numB {
			return 1
		}
	}
	return 0
}

func safeIndex(parts []string, i int) string {
	if i < len(parts) {
		return parts[i]
	}
	return "0"
}
