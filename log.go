package velocity

import (
	"log/slog"

	nwep "github.com/usenwep/nwep-go"
)

// Logger is the logging interface used throughout velocity. It follows the
// structured logging convention established by log/slog: each method accepts
// a message string and alternating key-value pairs as variadic arguments.
//
// All velocity components - including the built-in middleware, the server
// lifecycle, and handler error reporting - log through this interface. The
// caller can supply a custom implementation via WithLogger to integrate with
// any logging backend.
//
// The default implementation wraps slog.Default and is returned by
// DefaultLogger.
type Logger interface {
	// Debug logs a message at debug level. Use this for verbose
	// diagnostic output that is normally suppressed in production.
	Debug(msg string, args ...any)

	// Info logs a message at info level. Use this for routine
	// operational events such as server startup or request completion.
	Info(msg string, args ...any)

	// Warn logs a message at warn level. Use this for unexpected but
	// recoverable conditions.
	Warn(msg string, args ...any)

	// Error logs a message at error level. Use this for failures that
	// require operator attention.
	Error(msg string, args ...any)
}

type slogLogger struct {
	l *slog.Logger
}

func (s *slogLogger) Debug(msg string, args ...any) { s.l.Debug(msg, args...) }
func (s *slogLogger) Info(msg string, args ...any)  { s.l.Info(msg, args...) }
func (s *slogLogger) Warn(msg string, args ...any)  { s.l.Warn(msg, args...) }
func (s *slogLogger) Error(msg string, args ...any) { s.l.Error(msg, args...) }

// DefaultLogger returns a Logger backed by slog.Default. This is the logger
// used by Server when no explicit logger is configured via WithLogger.
func DefaultLogger() Logger {
	return &slogLogger{l: slog.Default()}
}

// SlogLogger wraps an existing *slog.Logger as a velocity Logger. This allows
// the caller to configure the slog handler, level, and output independently
// and have velocity use it. l must not be nil.
func SlogLogger(l *slog.Logger) Logger {
	return &slogLogger{l: l}
}

// BridgeNWEPLogs installs a callback that forwards log messages from the nwep
// C library to the given velocity Logger. Each nwep log entry is mapped to the
// corresponding Logger level: TRACE and DEBUG map to Debug, INFO to Info, WARN
// to Warn, and ERROR to Error. The nwep component name is included as a
// structured "component" key.
//
// Only one log callback can be active at a time - calling BridgeNWEPLogs again
// replaces the previous callback. This function is typically called once at
// startup:
//
//	velocity.BridgeNWEPLogs(velocity.DefaultLogger())
func BridgeNWEPLogs(log Logger) {
	nwep.SetLogCallback(func(entry *nwep.LogEntry) {
		switch entry.Level {
		case nwep.LogTrace, nwep.LogDebug:
			log.Debug(entry.Message, "component", entry.Component)
		case nwep.LogInfo:
			log.Info(entry.Message, "component", entry.Component)
		case nwep.LogWarn:
			log.Warn(entry.Message, "component", entry.Component)
		case nwep.LogError:
			log.Error(entry.Message, "component", entry.Component)
		default:
			log.Debug(entry.Message, "component", entry.Component)
		}
	})
}

// SetLogLevel sets the minimum severity level for the nwep C library's
// internal logger. Messages below this level are discarded before they reach
// the callback installed by BridgeNWEPLogs. Valid levels are nwep.LogTrace,
// nwep.LogDebug, nwep.LogInfo, nwep.LogWarn, and nwep.LogError.
func SetLogLevel(level nwep.LogLevel) {
	nwep.SetLogLevel(level)
}
