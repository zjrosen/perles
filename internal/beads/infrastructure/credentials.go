package infrastructure

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// doltServerPassword returns the password for the Dolt server at host:port the
// way bd does (configfile.ServerPasswordFor): BEADS_DOLT_PASSWORD when set,
// else the credentials-file entry for exactly [host:port], else "". A server
// with no port has no credentials-file entry.
func doltServerPassword(host string, port int) string {
	if p := os.Getenv("BEADS_DOLT_PASSWORD"); p != "" {
		return p
	}
	if port <= 0 {
		return ""
	}
	return lookupCredentialsPassword(host, port)
}

// credentialsFilePath is BEADS_CREDENTIALS_FILE, or bd's default credentials
// file: ~/.config/beads/credentials (%APPDATA%\beads\credentials on Windows).
func credentialsFilePath() string {
	if p := os.Getenv("BEADS_CREDENTIALS_FILE"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "beads", "credentials")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "beads", "credentials")
}

// lookupCredentialsPassword reads the password for host:port from bd's
// INI-style credentials file:
//
//	[127.0.0.1:3307]
//	password=localDevPassword
//
// An IPv6 host matches a section written either as [[::1]:3307] or [::1:3307].
func lookupCredentialsPassword(host string, port int) string {
	path := credentialsFilePath()
	if path == "" {
		return ""
	}
	return readCredentialsPassword(path, credentialsSectionKeys(host, port)...)
}

// credentialsSectionKeys returns the section names that hold host:port's
// password, most canonical first (configfile.CredentialsSectionKeys).
func credentialsSectionKeys(host string, port int) []string {
	bare := unbracketHost(host)
	joined := net.JoinHostPort(bare, fmt.Sprint(port))
	legacy := fmt.Sprintf("%s:%d", bare, port)
	if legacy == joined {
		return []string{joined}
	}
	return []string{joined, legacy}
}

// readCredentialsPassword returns the password of the first of sectionKeys
// whose section has a password key, or "" on any error.
func readCredentialsPassword(path string, sectionKeys ...string) string {
	f, err := os.Open(path) //nolint:gosec // path comes from BEADS_CREDENTIALS_FILE or the home dir
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	wanted := make(map[string]bool, len(sectionKeys))
	for _, key := range sectionKeys {
		wanted[key] = true
	}
	found := make(map[string]string, len(sectionKeys))

	scanner := bufio.NewScanner(f)
	section := ""
	inSection := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			inSection = wanted[section]
			continue
		}
		if !inSection {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) == "password" {
			if _, seen := found[section]; !seen {
				found[section] = strings.TrimSpace(value)
			}
		}
	}

	for _, key := range sectionKeys {
		if pw, ok := found[key]; ok {
			return pw
		}
	}
	return ""
}
