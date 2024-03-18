package logger

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogrusLogger is a Logger implementation that sends all logs to stdout using
// the Logrus package to get nice formatting
type LogrusLogger struct {
	LogFormatter string
	LogOutput    string
	File         struct {
		FileName   string
		MaxSize    int
		MaxBackups int
		Compress   bool
	}

	logger *logrus.Logger
	level  logrus.Level
}

type LogrusEntry struct {
	entry *logrus.Entry
	level logrus.Level
}

func (l *LogrusLogger) Start() error {
	l.logger.SetLevel(l.level)

	// disabling report caller and using a hook to do the same, so avoiding additional processing here
	l.logger.SetReportCaller(false) //nolint:all
	l.logger.AddHook(&CallerHook{})

	switch l.LogOutput {
	case "stdout":
		l.logger.SetOutput(os.Stdout)
	case "stderr":
		l.logger.SetOutput(os.Stderr)
	case "file":
		l.logger.SetOutput(&lumberjack.Logger{
			Filename:   l.File.FileName,
			MaxSize:    l.File.MaxSize,
			MaxBackups: l.File.MaxBackups,
			Compress:   l.File.Compress,
		})
	}

	switch l.LogFormatter {
	case "logfmt":
		l.logger.SetFormatter(&logrus.TextFormatter{ //nolint:all
			DisableColors:          true,
			ForceQuote:             true,
			FullTimestamp:          true,
			DisableLevelTruncation: true,
			QuoteEmptyFields:       true,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyFile:  "file",
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
				logrus.FieldKeyFunc:  "caller",
			},
		})
	case "json":
		l.logger.SetFormatter(&logrus.JSONFormatter{
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyFile:  "file",
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
				logrus.FieldKeyFunc:  "caller",
			},
		})
	}
	return nil
}

func (l *LogrusLogger) Init() *logrus.Logger {
	if l.logger != nil {
		return l.logger
	}
	l.logger = logrus.New()
	return l.logger
}

func (l *LogrusLogger) Panic() Entry {
	if !l.logger.IsLevelEnabled(logrus.PanicLevel) {
		return nullEntry
	}

	return &LogrusEntry{
		entry: logrus.NewEntry(l.logger),
		level: logrus.PanicLevel,
	}
}

func (l *LogrusLogger) Fatal() Entry {
	if !l.logger.IsLevelEnabled(logrus.FatalLevel) {
		return nullEntry
	}

	return &LogrusEntry{
		entry: logrus.NewEntry(l.logger),
		level: logrus.FatalLevel,
	}
}

func (l *LogrusLogger) Warn() Entry {
	if !l.logger.IsLevelEnabled(logrus.WarnLevel) {
		return nullEntry
	}

	return &LogrusEntry{
		entry: logrus.NewEntry(l.logger),
		level: logrus.WarnLevel,
	}
}

func (l *LogrusLogger) Trace() Entry {
	if !l.logger.IsLevelEnabled(logrus.TraceLevel) {
		return nullEntry
	}

	return &LogrusEntry{
		entry: logrus.NewEntry(l.logger),
		level: logrus.TraceLevel,
	}
}

func (l *LogrusLogger) Debug() Entry {
	if !l.logger.IsLevelEnabled(logrus.DebugLevel) {
		return nullEntry
	}

	return &LogrusEntry{
		entry: logrus.NewEntry(l.logger),
		level: logrus.DebugLevel,
	}
}

func (l *LogrusLogger) Info() Entry {
	if !l.logger.IsLevelEnabled(logrus.InfoLevel) {
		return nullEntry
	}

	return &LogrusEntry{
		entry: logrus.NewEntry(l.logger),
		level: logrus.InfoLevel,
	}
}

func (l *LogrusLogger) Error() Entry {
	if !l.logger.IsLevelEnabled(logrus.ErrorLevel) {
		return nullEntry
	}

	return &LogrusEntry{
		entry: logrus.NewEntry(l.logger),
		level: logrus.ErrorLevel,
	}
}

func (l *LogrusLogger) SetLevel(level string) error {
	logrusLevel, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	// record the choice and set it if we're already initialized
	l.level = logrusLevel
	if l.logger != nil {
		l.logger.SetLevel(logrusLevel)
	}
	return nil
}

func (l *LogrusEntry) WithField(key string, value interface{}) Entry {
	return &LogrusEntry{
		entry: l.entry.WithField(key, value),
		level: l.level,
	}
}

func (l *LogrusEntry) WithString(key, value string) Entry {
	return &LogrusEntry{
		entry: l.entry.WithField(key, value),
		level: l.level,
	}
}

func (l *LogrusEntry) WithFields(fields map[string]interface{}) Entry {
	return &LogrusEntry{
		entry: l.entry.WithFields(fields),
		level: l.level,
	}
}

func (l *LogrusEntry) Logf(f string, args ...interface{}) {
	switch l.level {
	case logrus.WarnLevel:
		l.entry.Warnf(f, args...)
	case logrus.FatalLevel:
		l.entry.Fatalf(f, args...)
	case logrus.PanicLevel:
		l.entry.Panicf(f, args...)
	case logrus.TraceLevel:
		l.entry.Tracef(f, args...)
	case logrus.DebugLevel:
		l.entry.Debugf(f, args...)
	case logrus.InfoLevel:
		l.entry.Infof(f, args...)
	default:
		l.entry.Errorf(f, args...)
	}
}

var (
	callerInitOnce     sync.Once
	presentProjectRoot string
)

type CallerHook struct{}

func (h *CallerHook) Fire(entry *logrus.Entry) error {
	functionName, fileName := h.caller()
	if fileName != "" {
		entry.Data[logrus.FieldKeyFile] = fileName
	}
	if functionName != "" {
		entry.Data[logrus.FieldKeyFunc] = functionName
	}

	return nil
}

func (h *CallerHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		logrus.DebugLevel,
	}
}

func (h *CallerHook) caller() (function, file string) {
	callerInitOnce.Do(func() {
		presentProjectRoot, _ = os.Getwd()
		presentProjectRoot = path.Join(presentProjectRoot, "../")
	})

	pcs := make([]uintptr, 25)
	_ = runtime.Callers(0, pcs)
	frames := runtime.CallersFrames(pcs)

	for next, again := frames.Next(); again; next, again = frames.Next() {
		if !strings.Contains(next.File, "/usr/local/go/") &&
			!strings.Contains(next.File, "logger") &&
			!strings.Contains(next.File, "logrus") &&
			strings.HasPrefix(next.File, presentProjectRoot) {
			return next.Function, fmt.Sprintf("%s:%d", strings.TrimPrefix(next.File, presentProjectRoot), next.Line)
		}
	}

	return
}
