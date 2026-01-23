package dashboard

import (
	"os"
	"testing"

	zone "github.com/lrstanley/bubblezone"
)

func TestMain(m *testing.M) {
	zone.NewGlobal()
	os.Exit(m.Run())
}
