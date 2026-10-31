package infrastructure

import (
	"errors"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// closedPort returns a TCP port on 127.0.0.1 that nothing listens on.
func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

func TestNewClient_UnsupportedWorkspace(t *testing.T) {
	isolateBeadsEnv(t)
	beadsDir := newBeadsDir(t)
	writeMetadata(t, beadsDir, `{"backend":"dolt","dolt_mode":"embedded"}`)

	_, err := NewClient(beadsDir, fallbackOpts)
	var uw *UnsupportedWorkspaceError
	require.ErrorAs(t, err, &uw)
	require.Equal(t, WorkspaceEmbeddedDolt, uw.Kind)
}

func TestNewClient_NoWorkspace(t *testing.T) {
	isolateBeadsEnv(t)

	_, err := NewClient(newBeadsDir(t), fallbackOpts)
	require.ErrorIs(t, err, ErrWorkspaceNotFound)
}

func TestNewClient_ServerWithoutPort(t *testing.T) {
	isolateBeadsEnv(t)
	beadsDir := newBeadsDir(t)
	writeMetadata(t, beadsDir, serverMetadata)

	_, err := NewClient(beadsDir, fallbackOpts)
	var su *ServerUnreachableError
	require.ErrorAs(t, err, &su)
	require.ErrorIs(t, err, errNoServerPort)
	require.Equal(t, "127.0.0.1 (no port found)", su.Conn.Location())
	require.True(t, su.Conn.ManagedByBD())
}

func TestNewClient_ServerNotListening(t *testing.T) {
	isolateBeadsEnv(t)
	beadsDir := newBeadsDir(t)
	writeMetadata(t, beadsDir, serverMetadata)
	port := closedPort(t)
	writeFile(t, beadsDir, "dolt-server.port", strconv.Itoa(port))

	_, err := NewClient(beadsDir, fallbackOpts)
	var su *ServerUnreachableError
	require.ErrorAs(t, err, &su)
	require.Equal(t, port, su.Conn.Port)
	require.Equal(t, PortSourcePortFile, su.Conn.PortSource)
	require.Contains(t, su.Error(), "127.0.0.1:"+strconv.Itoa(port))
}

func TestSocketTransport(t *testing.T) {
	tests := []struct {
		name       string
		conn       Connection
		socketUp   bool
		tcpUp      bool
		wantSocket string
	}{
		{name: "no socket uses TCP", conn: Connection{Host: "127.0.0.1", Port: 3307}, wantSocket: ""},
		{name: "live socket", conn: Connection{Host: "127.0.0.1", Port: 3307, Socket: "/s"}, socketUp: true, wantSocket: "/s"},
		{name: "dead socket, live TCP", conn: Connection{Host: "127.0.0.1", Port: 3307, Socket: "/s"}, tcpUp: true, wantSocket: ""},
		{name: "both down keeps the socket", conn: Connection{Host: "127.0.0.1", Port: 3307, Socket: "/s"}, wantSocket: "/s"},
		{name: "socket only", conn: Connection{Host: "127.0.0.1", Socket: "/s"}, tcpUp: true, wantSocket: "/s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := dialProbe
			t.Cleanup(func() { dialProbe = orig })
			dialProbe = func(network, _ string, _ time.Duration) error {
				if (network == "unix" && tt.socketUp) || (network == "tcp" && tt.tcpUp) {
					return nil
				}
				return errors.New("down")
			}

			require.Equal(t, tt.wantSocket, socketTransport(tt.conn))
		})
	}
}

func TestNewDoltServerClient_SocketSkipsTCPProbe(t *testing.T) {
	orig := dialProbe
	t.Cleanup(func() { dialProbe = orig })
	var probed []string
	dialProbe = func(network, addr string, _ time.Duration) error {
		probed = append(probed, network+":"+addr)
		return errors.New("down")
	}

	_, err := NewDoltServerClient(Connection{BeadsDir: t.TempDir(), Host: "127.0.0.1", Socket: "/nonexistent/dolt.sock", User: "root", Database: "beads"})
	require.Error(t, err, "nothing listens on the socket")
	require.Equal(t, []string{"unix:/nonexistent/dolt.sock"}, probed, "a socket connection must not be gated on a TCP probe")
}
