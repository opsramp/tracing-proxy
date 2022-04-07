package logger

import (
	"github.com/jirs5/tracing-proxy/config"
	"github.com/sirupsen/logrus"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
	"os"
)

// LogrusLogger is a Logger implementation that sends all logs to stdout using
// the Logrus package to get nice formatting
type LogrusLogger struct {
	Config config.Config `inject:""`

	logger *logrus.Logger
	level  logrus.Level
}

type LogrusEntry struct {
	entry *logrus.Entry
	level logrus.Level
}

func (l *LogrusLogger) Start() error {
	l.logger.SetLevel(l.level)
	l.logger.SetReportCaller(true)

	logrusConfig, err := l.Config.GetLogrusConfig()
	if err != nil {
		return err
	}

	switch logrusConfig.LogOutput {
	case "stdout":
		l.logger.SetOutput(os.Stdout)
	case "stderr":
		l.logger.SetOutput(os.Stderr)
	case "file":
		l.logger.SetOutput(&lumberjack.Logger{
			Filename:   logrusConfig.File.FileName,
			MaxSize:    logrusConfig.File.MaxSize,
			MaxBackups: logrusConfig.File.MaxBackups,
			Compress:   logrusConfig.File.Compress,
		})
	}

	switch logrusConfig.LogFormatter {
	case "logfmt":
		l.logger.SetFormatter(&logrus.TextFormatter{
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

func (l *LogrusEntry) WithString(key string, value string) Entry {
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
