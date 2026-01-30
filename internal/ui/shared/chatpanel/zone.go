package chatpanel

import "fmt"

// Zone ID constants for mouse click detection in the chat panel.
// Uses bubblezone for click detection on tabs and input area.
// zoneTabPrefix is the prefix for tab zone IDs.
const zoneTabPrefix = "chatpanel-tab:"

// makeTabZoneID creates a zone ID for a chat panel tab.
func makeTabZoneID(index int) string {
	return fmt.Sprintf("%s%d", zoneTabPrefix, index)
}

// MakeTabZoneID is the exported version for use by app.go.
// Returns the zone ID for the given tab index.
func MakeTabZoneID(index int) string {
	return makeTabZoneID(index)
}
