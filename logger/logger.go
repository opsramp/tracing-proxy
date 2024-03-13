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
	WithString(key, value string) Entry

	WithFields(fields map[string]interface{}) Entry
	Logf(f string, args ...interface{})
}

func GetLoggerImplementation(format, output, filename string, maxSize, maxBackup int, compress bool) Logger {
	lgr := &LogrusLogger{
		LogFormatter: format,
		LogOutput:    output,
		File: struct {
			FileName   string
			MaxSize    int
			MaxBackups int
			Compress   bool
		}{
			FileName:   filename,
			MaxSize:    maxSize,
			MaxBackups: maxBackup,
			Compress:   compress,
		},
	}
	_ = lgr.Init()
	_ = lgr.Start()
	return lgr
}
