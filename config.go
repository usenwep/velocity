package velocity

import nwep "github.com/usenwep/nwep-go"

// Config holds server configuration values that can be loaded from a file,
// environment, or any other source and applied to a Server via WithConfig.
// Zero-valued fields are ignored - only fields with non-zero values are
// applied, allowing partial configuration.
//
// Config is a convenience for declarative setup. For programmatic
// configuration, the individual With* options (WithKeypair, WithSettings,
// etc.) provide finer-grained control.
type Config struct {
	// Addr is the UDP listen address in "host:port" format. If empty,
	// the address passed to New is used unchanged.
	Addr string

	// KeyFile is the path to a hex-encoded Ed25519 seed file. If the
	// file does not exist, a new keypair is generated and saved. See
	// LoadOrGenerateKeypair for details. If both KeyFile and KeyEnv
	// are set, KeyFile takes precedence.
	KeyFile string

	// KeyEnv is the name of an environment variable containing a
	// hex-encoded Ed25519 seed. It is only used if KeyFile is empty or
	// if no keypair was loaded from KeyFile.
	KeyEnv string

	// Role sets the server's advertised role in the WEB/1 handshake.
	// Common values are "regular", "log_server", and "anchor".
	Role string

	// MaxStreams sets the maximum number of concurrent streams per
	// connection. If zero, the nwep default (100) is used.
	MaxStreams uint32

	// MaxMessageSize sets the maximum size of a single protocol
	// message in bytes. If zero, the nwep default (24 MiB) is used.
	MaxMessageSize uint32

	// TimeoutMs sets the connection idle timeout in milliseconds.
	// If zero, the nwep default (30000) is used.
	TimeoutMs uint32

	// Compression sets the compression algorithm for the connection.
	// If empty, no compression is used.
	Compression string

	// LogLevel sets the minimum severity for the nwep C library's
	// internal logger. If zero, the level is not changed.
	LogLevel nwep.LogLevel
}

// DefaultConfig returns a Config with sensible defaults: port 4433, info-level
// logging, and a 30-second timeout. All other fields are zero-valued.
func DefaultConfig() *Config {
	return &Config{
		Addr:      ":4433",
		LogLevel:  nwep.LogInfo,
		TimeoutMs: 30000,
	}
}

// Apply applies the non-zero fields of cfg to the Server. It is called
// internally by WithConfig and should not be called directly.
//
// KeyFile is loaded first. If KeyFile is empty or produces no keypair, KeyEnv
// is tried. LogLevel is applied via SetLogLevel. All transport-related fields
// are collected into an nwep.Settings and stored on the server.
//
// This function returns a non-nil error if key loading fails.
func (cfg *Config) Apply(s *Server) error {
	if cfg.KeyFile != "" {
		kp, err := LoadOrGenerateKeypair(cfg.KeyFile)
		if err != nil {
			return err
		}
		s.keypair = kp
	}
	if cfg.KeyEnv != "" && s.keypair == nil {
		kp, err := KeypairFromEnv(cfg.KeyEnv)
		if err != nil {
			return err
		}
		s.keypair = kp
	}
	if cfg.LogLevel != 0 {
		SetLogLevel(cfg.LogLevel)
	}
	settings := nwep.Settings{}
	if cfg.MaxStreams > 0 {
		settings.MaxStreams = cfg.MaxStreams
	}
	if cfg.MaxMessageSize > 0 {
		settings.MaxMessageSize = cfg.MaxMessageSize
	}
	if cfg.TimeoutMs > 0 {
		settings.TimeoutMs = cfg.TimeoutMs
	}
	if cfg.Compression != "" {
		settings.Compression = cfg.Compression
	}
	if cfg.Role != "" {
		settings.Role = cfg.Role
	}
	s.settings = &settings
	return nil
}
