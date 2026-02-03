package revenium

import "log"

// Logger provides simple leveled logging for the revenium package.
type Logger struct {
	debug bool
}

func newLogger(debug bool) *Logger {
	return &Logger{debug: debug}
}

// Debug logs a message at debug level (only when debug mode is enabled).
func (l *Logger) Debug(msg string, args ...any) {
	if l.debug {
		log.Printf("[revenium:debug] "+msg, args...)
	}
}

// Info logs a message at info level.
func (l *Logger) Info(msg string, args ...any) {
	log.Printf("[revenium:info] "+msg, args...)
}

// Warn logs a message at warn level.
func (l *Logger) Warn(msg string, args ...any) {
	log.Printf("[revenium:warn] "+msg, args...)
}

// Error logs a message at error level.
func (l *Logger) Error(msg string, args ...any) {
	log.Printf("[revenium:error] "+msg, args...)
}
