package beads

import (
	"fmt"
	"strconv"
	"strings"
)

// MinBeadsVersion is the minimum beads database version required by perles.
// Update this when perles starts using features from newer beads versions.
const MinBeadsVersion = "0.41.0"

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
