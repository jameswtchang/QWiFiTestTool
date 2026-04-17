package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultLogDir is the default directory for storing log files
	DefaultLogDir = "/tmp/wifi_logs"
	// FallbackLogDir is used when the default directory is not writable
	FallbackLogDir = "/tmp/wifi_logs"
)

// SessionLogger handles per-session logging for WiFi operations
type SessionLogger struct {
	mu        sync.Mutex
	file      *os.File
	logDir    string
	logPath   string
	startTime time.Time
}

var (
	currentSession *SessionLogger
	sessionMu      sync.Mutex
)

// NewSession creates a new logging session and returns the logger
func NewSession(prefix string) (*SessionLogger, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	// Close previous session if any
	if currentSession != nil {
		currentSession.Close()
	}

	now := time.Now()
	timestamp := now.Format("20060102_150405")

	// Try default log dir, fallback if needed
	logDir := DefaultLogDir
	if err := os.MkdirAll(logDir, 0755); err != nil {
		logDir = FallbackLogDir
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("無法建立 log 目錄: %w", err)
		}
	}

	filename := fmt.Sprintf("%s_%s.log", prefix, timestamp)
	logPath := filepath.Join(logDir, filename)

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("無法建立 log 檔: %w", err)
	}

	logger := &SessionLogger{
		file:      file,
		logDir:    logDir,
		logPath:   logPath,
		startTime: now,
	}

	// Write session header
	logger.writeRaw("=" + strings.Repeat("=", 69))
	logger.writeRaw(fmt.Sprintf("Session Start: %s", now.Format("2006-01-02 15:04:05")))
	logger.writeRaw(fmt.Sprintf("Log File: %s", logPath))
	logger.writeRaw("=" + strings.Repeat("=", 69))

	currentSession = logger

	fmt.Printf("[LOG] 日誌檔案: %s\n", logPath)
	return logger, nil
}

// GetCurrentSession returns the current active session logger (may be nil)
func GetCurrentSession() *SessionLogger {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	return currentSession
}

// LogCmd logs a command execution with its output and error
func (l *SessionLogger) LogCmd(cmdStr string, output []byte, err error) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	elapsed := time.Since(l.startTime).Truncate(time.Millisecond)
	ts := time.Now().Format("15:04:05.000")

	l.writef("[%s] [+%s] CMD: %s", ts, elapsed, cmdStr)
	if len(output) > 0 {
		l.writef("  OUTPUT:\n%s", indentLines(string(output), "    "))
	}
	if err != nil {
		l.writef("  ERROR: %v", err)
	} else {
		l.writef("  STATUS: OK")
	}
	l.writef("")
}

// LogEvent logs a general event
func (l *SessionLogger) LogEvent(format string, args ...interface{}) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().Format("15:04:05.000")
	elapsed := time.Since(l.startTime).Truncate(time.Millisecond)
	msg := fmt.Sprintf(format, args...)
	l.writef("[%s] [+%s] EVENT: %s", ts, elapsed, msg)
}

// LogConfig logs a config file's content
func (l *SessionLogger) LogConfig(label, path, content string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().Format("15:04:05.000")
	l.writef("[%s] CONFIG [%s]: %s", ts, label, path)
	l.writef("--- begin ---")
	l.writef("%s", content)
	l.writef("--- end ---")
	l.writef("")
}

// LogDaemonStart logs daemon start with the log file path
func (l *SessionLogger) LogDaemonStart(daemon string, args []string, logFile string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().Format("15:04:05.000")
	elapsed := time.Since(l.startTime).Truncate(time.Millisecond)
	l.writef("[%s] [+%s] DAEMON_START: %s %s", ts, elapsed, daemon, strings.Join(args, " "))
	if logFile != "" {
		l.writef("  DAEMON_LOG: %s", logFile)
	}
	l.writef("")
}

// GetDaemonLogPath returns a path for the wpa_supplicant daemon log file
func (l *SessionLogger) GetDaemonLogPath(daemonName string) string {
	if l == nil {
		return ""
	}
	timestamp := l.startTime.Format("20060102_150405")
	return filepath.Join(l.logDir, fmt.Sprintf("%s_%s.log", daemonName, timestamp))
}

// GetLogPath returns the current log file path
func (l *SessionLogger) GetLogPath() string {
	if l == nil {
		return ""
	}
	return l.logPath
}

// Close finalizes the session log
func (l *SessionLogger) Close() {
	if l == nil || l.file == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	elapsed := time.Since(l.startTime).Truncate(time.Millisecond)
	l.writef("")
	l.writeRaw(fmt.Sprintf("Session End (duration: %s)", elapsed))
	l.writeRaw("=" + strings.Repeat("=", 69))
	l.file.Close()
	l.file = nil
}

func (l *SessionLogger) writef(format string, args ...interface{}) {
	if l.file == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	l.file.WriteString(msg + "\n")
}

func (l *SessionLogger) writeRaw(s string) {
	if l.file == nil {
		return
	}
	l.file.WriteString(s + "\n")
}

// indentLines adds a prefix to each line
func indentLines(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// TimestampWriter wraps an io.Writer and prepends a timestamp to each line.
// Use this as Stdout/Stderr for daemon processes so log lines have real wall-clock time.
type TimestampWriter struct {
	file    *os.File
	buf     strings.Builder
}

// NewTimestampWriter opens (or creates) logPath and returns a TimestampWriter.
func NewTimestampWriter(logPath string) (*TimestampWriter, error) {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &TimestampWriter{file: f}, nil
}

// Write implements io.Writer. It buffers until newline, then writes with timestamp.
func (tw *TimestampWriter) Write(p []byte) (n int, err error) {
	tw.buf.Write(p)
	for {
		s := tw.buf.String()
		idx := strings.IndexByte(s, '\n')
		if idx == -1 {
			break
		}
		line := s[:idx]
		tw.buf.Reset()
		tw.buf.WriteString(s[idx+1:])

		ts := time.Now().Format("2006-01-02 15:04:05.000")
		_, err = fmt.Fprintf(tw.file, "[%s] %s\n", ts, line)
		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Close flushes any remaining buffer and closes the underlying file.
func (tw *TimestampWriter) Close() error {
	if s := tw.buf.String(); s != "" {
		ts := time.Now().Format("2006-01-02 15:04:05.000")
		fmt.Fprintf(tw.file, "[%s] %s\n", ts, s)
	}
	return tw.file.Close()
}
