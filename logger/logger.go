package logger

import "github.com/sirupsen/logrus"

type Logger interface {
	Debug() Entry
	Info() Entry
	Error() Entry
	Fatal() Entry
	Panic() Entry
	Warn() Entry
	// SetLevel sets the logging level (debug, info, warn, error)
	SetLevel(level string) error

	Init() *logrus.Logger
}

type Entry interface {
	WithField(key string, value interface{}) Entry

	// WithString does the same thing as WithField, but is more efficient for
	// disabled log levels. (Because the value parameter doesn't escape.)
	WithString(key string, value string) Entry

	WithFields(fields map[string]interface{}) Entry
	Logf(f string, args ...interface{})
}

func GetLoggerImplementation() Logger {
	return &LogrusLogger{}
}
