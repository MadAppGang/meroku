package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

// Level represents log level
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

// Logger provides structured logging with levels
type Logger struct {
	level  Level
	output io.Writer
}

// Global default logger instance
var defaultLogger = &Logger{
	level:  INFO,
	output: os.Stdout,
}

// New creates a new logger with the specified level and output
func New(level Level, output io.Writer) *Logger {
	return &Logger{level: level, output: output}
}

// Default returns the default logger instance
func Default() *Logger {
	return defaultLogger
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level Level) {
	l.level = level
}

// SetOutput sets the output writer
func (l *Logger) SetOutput(output io.Writer) {
	l.output = output
}

// log formats and writes a log message
func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	var prefix string
	switch level {
	case DEBUG:
		prefix = "🔍 DEBUG"
	case INFO:
		prefix = "ℹ️  INFO "
	case WARN:
		prefix = "⚠️  WARN "
	case ERROR:
		prefix = "❌ ERROR"
	}

	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, args...)

	log.SetOutput(l.output)
	log.SetFlags(0) // We handle our own formatting
	log.Printf("[%s] %s: %s\n", timestamp, prefix, message)
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// Success logs a success message (special info with checkmark emoji)
func (l *Logger) Success(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, args...)

	log.SetOutput(l.output)
	log.SetFlags(0)
	log.Printf("[%s] ✅ SUCCESS: %s\n", timestamp, message)
}

// Package-level convenience functions that use the default logger

// Debug logs a debug message using the default logger
func Debug(format string, args ...interface{}) {
	defaultLogger.Debug(format, args...)
}

// Info logs an info message using the default logger
func Info(format string, args ...interface{}) {
	defaultLogger.Info(format, args...)
}

// Warn logs a warning message using the default logger
func Warn(format string, args ...interface{}) {
	defaultLogger.Warn(format, args...)
}

// Error logs an error message using the default logger
func Error(format string, args ...interface{}) {
	defaultLogger.Error(format, args...)
}

// Success logs a success message using the default logger
func Success(format string, args ...interface{}) {
	defaultLogger.Success(format, args...)
}

// SetLevel sets the minimum log level for the default logger
func SetLevel(level Level) {
	defaultLogger.SetLevel(level)
}

// SetOutput sets the output writer for the default logger
func SetOutput(output io.Writer) {
	defaultLogger.SetOutput(output)
}
