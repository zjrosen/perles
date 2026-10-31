package infrastructure

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/zjrosen/perles/internal/log"
)

// Dolt modes (bd's configfile.DoltMode*).
const (
	doltModeEmbedded      = "embedded"
	doltModeServer        = "server"
	doltModeProxiedServer = "proxied-server"
)

const (
	defaultDoltServerHost   = "127.0.0.1"
	defaultDoltServerPort   = 3307 // bd's default for a server it does not run
	defaultSharedServerPort = 3308 // bd's doltserver.DefaultSharedServerPort
	defaultDoltServerUser   = "root"
	defaultDoltDatabase     = "beads"
	doltServerPortFile      = "dolt-server.port"
)

// PortSource records where a Connection's port came from.
type PortSource string

const (
	PortSourceNone            PortSource = ""
	PortSourceBDContext       PortSource = "bd context"
	PortSourceEnv             PortSource = "BEADS_DOLT_SERVER_PORT"
	PortSourcePortFile        PortSource = "dolt-server.port"
	PortSourceDoltConfig      PortSource = "dolt config.yaml listener.port"
	PortSourceConfigYAML      PortSource = "config.yaml dolt.port"
	PortSourceMetadata        PortSource = "metadata.json dolt_server_port"
	PortSourceLegacyEnv       PortSource = "BEADS_DOLT_PORT"
	PortSourceSharedDefault   PortSource = "shared-server default"
	PortSourceExternalDefault PortSource = "default"
)

// authoritative reports whether the port is a user assertion of where the
// server lives rather than bd's bookkeeping or a default; the legacy
// BEADS_DOLT_PORT replaces only non-authoritative ports
// (doltserver.PortSource.IsAuthoritative).
func (s PortSource) authoritative() bool {
	switch s {
	case PortSourceEnv, PortSourceDoltConfig, PortSourceConfigYAML, PortSourceMetadata:
		return true
	default:
		return false
	}
}

// Connection is how perles reaches a beads workspace's Dolt SQL server.
type Connection struct {
	BeadsDir string
	// Host is the server host as configured; an IPv6 literal may be bracketed.
	Host string
	// Port is the TCP port, or 0 when no source named one.
	Port       int
	PortSource PortSource
	// Socket is a unix socket path; when set it is tried before TCP.
	Socket   string
	User     string
	Password string //nolint:gosec // G117: DSN password, never serialized
	Database string
	TLS      bool
	// SharedServer reports bd's shared-server mode (one server per machine).
	SharedServer bool
	// FromBDContext reports that `bd context` supplied the mode, host, port
	// and database.
	FromBDContext bool
}

// DisplayHost is the host without IPv6 brackets.
func (c Connection) DisplayHost() string {
	return unbracketHost(c.Host)
}

// Addr is host:port, bracketing an IPv6 host.
func (c Connection) Addr() string {
	return net.JoinHostPort(unbracketHost(c.Host), strconv.Itoa(c.Port))
}

// Dialable reports whether the connection has an address: a port or a socket.
func (c Connection) Dialable() bool {
	return c.Port > 0 || c.Socket != ""
}

// Location names the server for messages: host:port, the socket, or the host
// and the fact that no port was found.
func (c Connection) Location() string {
	switch {
	case c.Socket != "":
		return "unix:" + c.Socket
	case c.Port > 0:
		return c.Addr()
	default:
		return unbracketHost(c.Host) + " (no port found)"
	}
}

// ManagedByBD reports whether the server is one bd starts itself (`bd dolt
// start`): the shared server, or a TCP server on 127.0.0.1.
func (c Connection) ManagedByBD() bool {
	if c.Socket != "" {
		return false
	}
	return c.SharedServer || bdRunsServerAt(c.Host)
}

// dsn builds the MySQL DSN, over the unix socket when socket is non-empty and
// over TCP otherwise. TLS is disabled explicitly unless configured, because
// go-sql-driver's "preferred" default fails against Dolt servers without TLS.
func (c Connection) dsn(socket string) string {
	cfg := mysql.NewConfig()
	cfg.User = c.User
	cfg.Passwd = c.Password
	cfg.DBName = c.Database
	cfg.ParseTime = true
	cfg.Timeout = 5 * time.Second
	if socket != "" {
		cfg.Net = "unix"
		cfg.Addr = socket
	} else {
		cfg.Net = "tcp"
		cfg.Addr = c.Addr()
	}
	if c.TLS {
		cfg.TLSConfig = "true"
	} else {
		cfg.TLSConfig = "false"
	}
	return cfg.FormatDSN()
}

// ResolveOptions configures ResolveConnection.
type ResolveOptions struct {
	// WorkDir is the directory bd runs in (the perles working directory).
	WorkDir string
	// BDContext runs `bd context --json`; nil runs the bd on PATH.
	BDContext BDContextRunner
}

// ResolveConnection finds the Dolt SQL server for the workspace at beadsDir.
//
// It refuses workspaces perles cannot read with *UnsupportedWorkspaceError
// (graph formats, historical SQLite, non-Dolt backends, embedded and
// proxied-server Dolt) and returns ErrWorkspaceNotFound when beadsDir holds no
// workspace. Otherwise it asks `bd context --json` for the effective mode,
// host, port and database, and falls back to resolving them from bd's config
// files and environment the way bd does when bd is missing, too old, or fails.
// The socket, TLS, user and password always come from bd's configuration
// (bd context does not report them).
func ResolveConnection(beadsDir string, opts ResolveOptions) (*Connection, error) {
	if !dirExists(beadsDir) {
		return nil, fmt.Errorf("%w: %s does not exist", ErrWorkspaceNotFound, beadsDir)
	}
	loadBeadsEnvFile(beadsDir)

	meta, err := LoadMetadata(beadsDir)
	if err != nil {
		if hasGraphFormatMarker(beadsDir) {
			return nil, graphMarkerError(beadsDir)
		}
		return nil, err
	}
	if err := classifyWorkspace(beadsDir, meta); err != nil {
		return nil, err
	}

	w := newWorkspaceConfig(beadsDir, meta)
	mode := w.mode()
	if !hasWorkspaceState(beadsDir, meta) && mode != doltModeServer {
		return nil, fmt.Errorf("%w: %s holds no beads metadata or Dolt data", ErrWorkspaceNotFound, beadsDir)
	}

	runner := opts.BDContext
	if runner == nil {
		runner = defaultBDContextRunner
	}
	bdCtx, ctxErr := queryBDContext(runner, beadsDir, opts.WorkDir)
	if ctxErr != nil {
		log.Debug(log.CatDB, "bd context unavailable, resolving the Dolt connection from config files", "error", ctxErr)
		bdCtx = nil
	} else {
		log.Debug(log.CatDB, "bd context", "bdVersion", bdCtx.BdVersion, "backend", bdCtx.Backend,
			"doltMode", bdCtx.DoltMode, "host", bdCtx.ServerHost, "port", bdCtx.ServerPort, "database", bdCtx.Database)
		if err := checkBackend(beadsDir, bdCtx.Backend); err != nil {
			return nil, err
		}
		mode = bdContextMode(bdCtx, w.sharedServer())
	}

	switch mode {
	case doltModeProxiedServer:
		return nil, &UnsupportedWorkspaceError{Kind: WorkspaceProxiedServer, BeadsDir: beadsDir}
	case doltModeEmbedded:
		return nil, &UnsupportedWorkspaceError{Kind: WorkspaceEmbeddedDolt, BeadsDir: beadsDir}
	}

	conn := &Connection{
		BeadsDir:     beadsDir,
		Host:         w.host(),
		Socket:       w.socket(),
		User:         w.user(),
		Database:     w.database(),
		TLS:          w.tls(),
		SharedServer: w.sharedServer(),
	}
	if bdCtx != nil {
		conn.FromBDContext = true
		if bdCtx.ServerHost != "" {
			conn.Host = bdCtx.ServerHost
		}
		if bdCtx.Database != "" {
			conn.Database = bdCtx.Database
		}
		if bdCtx.ServerPort > 0 {
			conn.Port, conn.PortSource = bdCtx.ServerPort, PortSourceBDContext
		}
	}
	if conn.Port == 0 {
		conn.Port, conn.PortSource = w.port(conn.Host)
	}
	conn.Password = doltServerPassword(conn.Host, conn.Port)
	return conn, nil
}

// bdContextMode is the Dolt mode bd context reports. bd context reports a
// server host only in server mode, but it does not apply dolt.shared-server
// from config.yaml (bd's store selection does), so shared selects server
// unless the workspace is proxied.
func bdContextMode(bdCtx *BDContext, shared bool) string {
	switch {
	case bdCtx.ProxiedDir != "" || strings.EqualFold(bdCtx.DoltMode, doltModeProxiedServer):
		return doltModeProxiedServer
	case bdCtx.ServerHost != "" || strings.EqualFold(bdCtx.DoltMode, doltModeServer) || shared:
		return doltModeServer
	default:
		return doltModeEmbedded
	}
}

// workspaceConfig is bd's configuration for one workspace: its metadata (nil
// when absent), merged config.yaml, and the environment.
type workspaceConfig struct {
	beadsDir string
	meta     *BeadsMetadata
	yaml     beadsYAML
}

func newWorkspaceConfig(beadsDir string, meta *BeadsMetadata) workspaceConfig {
	if meta == nil {
		meta = &BeadsMetadata{}
	}
	return workspaceConfig{beadsDir: beadsDir, meta: meta, yaml: loadBeadsYAML(beadsDir)}
}

// mode resolves the Dolt mode the way bd's store selection does
// (configfile.IsDoltServerMode, IsDoltProxiedServerMode and the shared-server
// override in cmd/bd main.go).
func (w workspaceConfig) mode() string {
	if strings.EqualFold(w.meta.DoltMode, doltModeProxiedServer) {
		return doltModeProxiedServer
	}
	if w.isDoltServerMode() || w.sharedServer() {
		return doltModeServer
	}
	return doltModeEmbedded
}

// isDoltServerMode mirrors configfile.Config.IsDoltServerMode:
// BEADS_DOLT_SERVER_MODE=1, BEADS_DOLT_SHARED_SERVER, a non-local host,
// metadata.json dolt_mode, then config.yaml dolt.mode.
func (w workspaceConfig) isDoltServerMode() bool {
	if os.Getenv("BEADS_DOLT_SERVER_MODE") == "1" {
		return true
	}
	if envSharedServer() {
		return true
	}
	if w.hostImpliesServerMode() {
		return true
	}
	if w.meta.DoltMode != "" {
		return strings.EqualFold(w.meta.DoltMode, doltModeServer)
	}
	return strings.EqualFold(w.yaml.get("dolt.mode"), doltModeServer)
}

// hostImpliesServerMode mirrors configfile.Config.HostImpliesServerMode: a
// configured non-local host means a server, along the effective host chain
// (env, metadata.json, config.yaml), with an explicit mode at each level
// suppressing inference from the levels below it.
func (w workspaceConfig) hostImpliesServerMode() bool {
	if strings.EqualFold(w.meta.DoltMode, doltModeProxiedServer) {
		return false
	}
	if h := os.Getenv("BEADS_DOLT_SERVER_HOST"); h != "" {
		return !isLocalHost(h)
	}
	if w.meta.DoltMode != "" {
		return false
	}
	if w.meta.DoltServerHost != "" {
		return !isLocalHost(w.meta.DoltServerHost)
	}
	if w.yaml.get("dolt.mode") != "" {
		return false
	}
	if h := w.yaml.get("dolt.host"); h != "" {
		return !isLocalHost(h)
	}
	return false
}

// sharedServer mirrors doltserver.IsSharedServerMode: BEADS_DOLT_SHARED_SERVER
// ("1" or "true"), then dolt.shared-server in config.yaml.
func (w workspaceConfig) sharedServer() bool {
	return envSharedServer() || w.yaml.getBool("dolt.shared-server")
}

func envSharedServer() bool {
	v := os.Getenv("BEADS_DOLT_SHARED_SERVER")
	return v == "1" || strings.EqualFold(v, "true")
}

// serverExternal mirrors doltserver's ServerModeExternal: a server bd does not
// start (resolveServerMode).
func (w workspaceConfig) serverExternal() bool {
	if os.Getenv("BEADS_DOLT_SERVER_MODE") == "1" || w.sharedServer() || w.hostImpliesServerMode() {
		return true
	}
	if !strings.EqualFold(w.meta.DoltMode, doltModeProxiedServer) &&
		(envPortValue("BEADS_DOLT_SERVER_PORT") > 0 || envPortValue("BEADS_DOLT_PORT") > 0) {
		return true
	}
	if strings.EqualFold(w.meta.DoltMode, doltModeEmbedded) {
		return false
	}
	return w.meta.DoltServerPort > 0
}

// host is BEADS_DOLT_SERVER_HOST, metadata.json dolt_server_host,
// config.yaml dolt.host, then 127.0.0.1.
func (w workspaceConfig) host() string {
	if h := os.Getenv("BEADS_DOLT_SERVER_HOST"); h != "" {
		return h
	}
	if w.meta.DoltServerHost != "" {
		return w.meta.DoltServerHost
	}
	if h := w.yaml.get("dolt.host"); h != "" {
		return h
	}
	return defaultDoltServerHost
}

// database is BEADS_DOLT_SERVER_DATABASE, metadata.json dolt_database, then
// "beads".
func (w workspaceConfig) database() string {
	if d := os.Getenv("BEADS_DOLT_SERVER_DATABASE"); d != "" {
		return d
	}
	if w.meta.DoltDatabase != "" {
		return w.meta.DoltDatabase
	}
	return defaultDoltDatabase
}

// user is BEADS_DOLT_SERVER_USER, metadata.json dolt_server_user, then root.
func (w workspaceConfig) user() string {
	if u := os.Getenv("BEADS_DOLT_SERVER_USER"); u != "" {
		return u
	}
	if w.meta.DoltServerUser != "" {
		return w.meta.DoltServerUser
	}
	return defaultDoltServerUser
}

// socket is BEADS_DOLT_SERVER_SOCKET, then metadata.json dolt_server_socket.
func (w workspaceConfig) socket() string {
	if s := os.Getenv("BEADS_DOLT_SERVER_SOCKET"); s != "" {
		return s
	}
	return w.meta.DoltServerSocket
}

// tls is BEADS_DOLT_SERVER_TLS ("1" or "true"), then metadata.json
// dolt_server_tls.
func (w workspaceConfig) tls() bool {
	if t := os.Getenv("BEADS_DOLT_SERVER_TLS"); t != "" {
		return t == "1" || strings.EqualFold(t, "true")
	}
	return w.meta.DoltServerTLS
}

// port resolves the server port for host the way bd's DialConfig does:
//
//  1. BEADS_DOLT_SERVER_PORT
//  2. the port file bd writes when it starts the server (.beads/dolt-server.port,
//     or the shared server's), ignored for a host other than 127.0.0.1
//  3. listener.port in the Dolt data directory's config.yaml
//  4. dolt.port in config.yaml
//  5. metadata.json dolt_server_port (deprecated)
//  6. the shared-server default 3308
//  7. BEADS_DOLT_PORT (legacy), replacing only a port file or default port
//  8. 3307 for an external server on another host
//
// It returns 0 when nothing names a port for a server bd runs locally: bd
// would start one on a fresh port, so there is nothing to connect to yet.
func (w workspaceConfig) port(host string) (int, PortSource) {
	shared := w.sharedServer()
	serverDir := w.beadsDir
	if shared {
		if dir, err := sharedServerPath(); err == nil {
			serverDir = dir
		}
	}
	bdServer := bdRunsServerAt(host)

	port, src := w.portChain(serverDir, shared, false)
	if port == 0 && shared {
		port, src = defaultSharedServerPort, PortSourceSharedDefault
	}
	if !shared {
		external := w.serverExternal()
		if !bdServer && src == PortSourcePortFile {
			// The port file is bd's bookkeeping for its own server on
			// 127.0.0.1; it never describes a server on another host.
			port, src = w.portChain(serverDir, false, true)
		}
		if external && !bdServer {
			port, src = applyLegacyPortEnv(port, src)
			if port == 0 {
				port, src = defaultDoltServerPort, PortSourceExternalDefault
			}
		}
	}
	return applyLegacyPortEnv(port, src)
}

// portChain walks bd's port sources (doltserver portSources) for serverDir.
func (w workspaceConfig) portChain(serverDir string, shared, skipPortFile bool) (int, PortSource) {
	if p := envPortValue("BEADS_DOLT_SERVER_PORT"); p > 0 {
		return p, PortSourceEnv
	}
	if !skipPortFile {
		if p := readPortFile(filepath.Join(serverDir, doltServerPortFile)); p > 0 {
			log.Debug(log.CatDB, "Discovered Dolt server port from port file", "dir", serverDir, "port", p)
			return p, PortSourcePortFile
		}
	}

	serverMeta := w.meta
	if serverDir != w.beadsDir {
		serverMeta, _ = LoadMetadata(serverDir)
		if serverMeta == nil {
			serverMeta = &BeadsMetadata{}
		}
	}
	doltDir := doltDataDir(serverDir, serverMeta)
	if shared {
		doltDir = filepath.Join(serverDir, "dolt")
	}
	if p := doltConfigListenerPort(doltDir); p > 0 {
		return p, PortSourceDoltConfig
	}
	if p, err := strconv.Atoi(w.yaml.get("dolt.port")); err == nil && p > 0 {
		return p, PortSourceConfigYAML
	}
	if serverMeta.DoltServerPort > 0 {
		log.Debug(log.CatDB, "Using deprecated dolt_server_port from metadata.json", "port", serverMeta.DoltServerPort)
		return serverMeta.DoltServerPort, PortSourceMetadata
	}
	return 0, PortSourceNone
}

// applyLegacyPortEnv applies BEADS_DOLT_PORT when BEADS_DOLT_SERVER_PORT is
// unset and the port is not one the user named (doltserver.applyLegacyPortEnv).
func applyLegacyPortEnv(port int, src PortSource) (int, PortSource) {
	if os.Getenv("BEADS_DOLT_SERVER_PORT") != "" || src.authoritative() {
		return port, src
	}
	if p, err := strconv.Atoi(os.Getenv("BEADS_DOLT_PORT")); err == nil && p > 0 {
		return p, PortSourceLegacyEnv
	}
	return port, src
}

// doltDataDir is the Dolt server data directory for beadsDir: BEADS_DOLT_DATA_DIR,
// metadata.json dolt_data_dir, an absolute metadata.json database, then
// beadsDir/dolt (configfile.Config.DatabasePath).
func doltDataDir(beadsDir string, meta *BeadsMetadata) string {
	if d := os.Getenv("BEADS_DOLT_DATA_DIR"); d != "" {
		if filepath.IsAbs(d) {
			return d
		}
		return filepath.Join(beadsDir, d)
	}
	if d := meta.DoltDataDir; d != "" {
		if filepath.IsAbs(d) {
			return d
		}
		return filepath.Join(beadsDir, d)
	}
	if filepath.IsAbs(meta.Database) {
		return meta.Database
	}
	return filepath.Join(beadsDir, "dolt")
}

// doltConfigListenerPort reads listener.port from the Dolt server's own
// config.yaml in doltDir, or 0 when it is absent.
func doltConfigListenerPort(doltDir string) int {
	m := readYAMLFile(filepath.Join(doltDir, "config.yaml"))
	if m == nil {
		return 0
	}
	v, ok := lookupYAMLKey(m, "listener.port")
	if !ok {
		return 0
	}
	p, err := strconv.Atoi(strings.TrimSpace(yamlScalarString(v)))
	if err != nil || p <= 0 {
		return 0
	}
	return p
}

func readPortFile(path string) int {
	data, err := os.ReadFile(path) //nolint:gosec // port file within the .beads or shared-server dir
	if err != nil {
		return 0
	}
	p, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || p <= 0 {
		return 0
	}
	return p
}

func envPortValue(name string) int {
	p, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || p <= 0 {
		return 0
	}
	return p
}

// sharedServerPath is bd's shared-server state directory: BEADS_SHARED_SERVER_DIR,
// else ~/.beads/shared-server.
func sharedServerPath() (string, error) {
	if d := os.Getenv("BEADS_SHARED_SERVER_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".beads", "shared-server"), nil
}

// hostKind classifies a Dolt host by spelling (configfile.ClassifyDoltHost).
type hostKind int

const (
	hostBdServer      hostKind = iota // 127.0.0.1, localhost, 0.0.0.0, "" (bd's own server)
	hostIPv6Loopback                  // ::1 or :: in any spelling
	hostOtherLoopback                 // 127.0.0.2 and the like
	hostRemote                        // anything else
)

func classifyHost(host string) hostKind {
	h := strings.ToLower(unbracketHost(host))
	if h == "" || h == "localhost" {
		return hostBdServer
	}
	addr, err := netip.ParseAddr(h)
	if err != nil || addr.Zone() != "" {
		return hostRemote
	}
	addr = addr.Unmap()
	switch {
	case addr.Is4() && (addr == netip.AddrFrom4([4]byte{127, 0, 0, 1}) || addr.IsUnspecified()):
		return hostBdServer
	case addr.Is4() && addr.IsLoopback():
		return hostOtherLoopback
	case addr.Is6() && (addr.IsLoopback() || addr.IsUnspecified()):
		return hostIPv6Loopback
	}
	return hostRemote
}

// bdRunsServerAt reports whether host is where bd's own server listens
// (127.0.0.1 in any spelling).
func bdRunsServerAt(host string) bool {
	return classifyHost(host) == hostBdServer
}

// isLocalHost reports whether host names this machine's loopback, which does
// not imply server mode (configfile.IsLocalHostString).
func isLocalHost(host string) bool {
	switch classifyHost(host) {
	case hostBdServer, hostIPv6Loopback:
		return true
	}
	return false
}

// unbracketHost trims whitespace and one pair of enclosing brackets, so
// "[::1]" becomes "::1".
func unbracketHost(host string) string {
	host = strings.TrimSpace(host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return host[1 : len(host)-1]
	}
	return host
}
