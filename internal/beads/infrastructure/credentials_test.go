package infrastructure

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const credentialsFixture = `
# team credentials
[127.0.0.1:3307]
password=localDev   # inline comment

[db.example.com:3307]
user=ignored
password = teamSecret
password = second-is-ignored

[[::1]:3307]
password=bracketedV6

[::1:3308]
password=legacyV6

[empty.example.com:3307]
password=
`

func TestDoltServerPassword(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		env  string
		want string
	}{
		{"local section with inline comment", "127.0.0.1", 3307, "", "localDev"},
		{"first password in section wins", "db.example.com", 3307, "", "teamSecret"},
		{"port must match exactly", "db.example.com", 3308, "", ""},
		{"bare IPv6 finds bracketed section", "::1", 3307, "", "bracketedV6"},
		{"bracketed IPv6 finds bracketed section", "[::1]", 3307, "", "bracketedV6"},
		{"IPv6 finds legacy section", "::1", 3308, "", "legacyV6"},
		{"empty password is a password", "empty.example.com", 3307, "", ""},
		{"unknown server", "other.example.com", 3307, "", ""},
		{"env wins over file", "127.0.0.1", 3307, "fromEnv", "fromEnv"},
		{"no port uses env only", "127.0.0.1", 0, "", ""},
		{"no port with env", "127.0.0.1", 0, "fromEnv", "fromEnv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateBeadsEnv(t)
			t.Setenv("BEADS_CREDENTIALS_FILE", writeFile(t, t.TempDir(), "credentials", credentialsFixture))
			t.Setenv("BEADS_DOLT_PASSWORD", tt.env)

			require.Equal(t, tt.want, doltServerPassword(tt.host, tt.port))
		})
	}
}

func TestDoltServerPassword_DefaultCredentialsPath(t *testing.T) {
	home := isolateBeadsEnv(t)
	writeFile(t, home, ".config/beads/credentials", "[127.0.0.1:3307]\npassword=fromHome\n")

	require.Equal(t, "fromHome", doltServerPassword("127.0.0.1", 3307))
}

func TestDoltServerPassword_MissingFile(t *testing.T) {
	isolateBeadsEnv(t)
	t.Setenv("BEADS_CREDENTIALS_FILE", "/nonexistent/credentials")

	require.Empty(t, doltServerPassword("127.0.0.1", 3307))
}
