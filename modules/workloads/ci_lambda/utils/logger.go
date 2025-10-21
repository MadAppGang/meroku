package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"madappgang.com/infrastructure/ci_lambda/config"
)

// Logger provides structured logging for CloudWatch
type Logger struct {
	level       config.LogLevel
	projectName string
	environment string
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp   string                 `json:"timestamp"`
	Level       string                 `json:"level"`
	Message     string                 `json:"message"`
	ProjectName string                 `json:"project_name,omitempty"`
	Environment string                 `json:"environment,omitempty"`
	Fields      map[string]interface{} `json:"fields,omitempty"`
}

// NewLogger creates a new structured logger
func NewLogger(cfg *config.Config) *Logger {
	return &Logger{
		level:       cfg.LogLevel,
		projectName: cfg.ProjectName,
		environment: cfg.Environment,
	}
}

// Debug logs a debug message with optional fields
func (l *Logger) Debug(message string, fields map[string]interface{}) {
	if l.shouldLog(config.LogLevelDebug) {
		l.log(config.LogLevelDebug, message, fields)
	}
}

// Info logs an info message with optional fields
func (l *Logger) Info(message string, fields map[string]interface{}) {
	if l.shouldLog(config.LogLevelInfo) {
		l.log(config.LogLevelInfo, message, fields)
	}
}

// Warn logs a warning message with optional fields
func (l *Logger) Warn(message string, fields map[string]interface{}) {
	if l.shouldLog(config.LogLevelWarn) {
		l.log(config.LogLevelWarn, message, fields)
	}
}

// Error logs an error message with optional fields
func (l *Logger) Error(message string, fields map[string]interface{}) {
	if l.shouldLog(config.LogLevelError) {
		l.log(config.LogLevelError, message, fields)
	}
}

// WithFields creates a copy of the logger with additional context fields
func (l *Logger) WithFields(fields map[string]interface{}) *ContextLogger {
	return &ContextLogger{
		logger: l,
		fields: fields,
	}
}

func (l *Logger) log(level config.LogLevel, message string, fields map[string]interface{}) {
	entry := LogEntry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Level:       string(level),
		Message:     message,
		ProjectName: l.projectName,
		Environment: l.environment,
		Fields:      fields,
	}

	// Marshal to JSON for structured CloudWatch logs
	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		// Fallback to simple logging if JSON marshaling fails
		fmt.Fprintf(os.Stderr, "[%s] %s: %s (marshal error: %v)\n",
			entry.Timestamp, entry.Level, message, err)
		return
	}

	fmt.Println(string(jsonBytes))
}

func (l *Logger) shouldLog(level config.LogLevel) bool {
	levelOrder := map[config.LogLevel]int{
		config.LogLevelDebug: 0,
		config.LogLevelInfo:  1,
		config.LogLevelWarn:  2,
		config.LogLevelError: 3,
	}

	return levelOrder[level] >= levelOrder[l.level]
}

// ContextLogger is a logger with pre-set context fields
type ContextLogger struct {
	logger *Logger
	fields map[string]interface{}
}

// Debug logs with context fields
func (c *ContextLogger) Debug(message string, extraFields map[string]interface{}) {
	c.logger.Debug(message, c.mergeFields(extraFields))
}

// Info logs with context fields
func (c *ContextLogger) Info(message string, extraFields map[string]interface{}) {
	c.logger.Info(message, c.mergeFields(extraFields))
}

// Warn logs with context fields
func (c *ContextLogger) Warn(message string, extraFields map[string]interface{}) {
	c.logger.Warn(message, c.mergeFields(extraFields))
}

// Error logs with context fields
func (c *ContextLogger) Error(message string, extraFields map[string]interface{}) {
	c.logger.Error(message, c.mergeFields(extraFields))
}

func (c *ContextLogger) mergeFields(extraFields map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})

	// Add context fields first
	for k, v := range c.fields {
		merged[k] = v
	}

	// Add extra fields (these can override context fields)
	for k, v := range extraFields {
		merged[k] = v
	}

	return merged
}
