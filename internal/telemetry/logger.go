// Package telemetry provides the ghost logging system.
//
// Ghost logs are structured JSONL files written to ~/.glaude/logs/.
// They capture internal state transitions, API timings, and tool invocations
// without interfering with the terminal UI.
package telemetry

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Log is the package-level ghost logger.
// All internal subsystems should use this logger instead of writing to stdout/stderr.
var Log *logrus.Logger

// Init initializes the dual-track logging system.
//
// Backend track: full structured JSONL with log rotation, written to disk.
// Frontend track: the terminal UI layer controls its own rendering separately.
func Init() error {
	logDir := viper.GetString("log_dir")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("create log dir %s: %w", logDir, err)
	}

	Log = logrus.New()

	// Structured JSON output for machine consumption
	Log.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
	})

	// Rotate logs to prevent disk snowball
	Log.SetOutput(&lumberjack.Logger{
		Filename:   filepath.Join(logDir, "glaude.jsonl"),
		MaxSize:    50, // megabytes
		MaxBackups: 3,
		MaxAge:     7, // days
	})

	level, err := logrus.ParseLevel(viper.GetString("log_level"))
	if err != nil {
		level = logrus.InfoLevel
	}
	Log.SetLevel(level)

	return nil
}

// Close flushes any pending log entries.
// Should be called during graceful shutdown.
func Close() {
	if Log != nil {
		Log.Info("glaude session ended")
	}
}
