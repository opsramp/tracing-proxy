package logger

import "github.com/sirupsen/logrus"

var nullEntry = &NullLoggerEntry{}

type NullLogger struct {
	Logger
}

func (n *NullLogger) Init() *logrus.Logger  { return logrus.New() }
func (n *NullLogger) Fatal() Entry          { return nullEntry }
func (n *NullLogger) Panic() Entry          { return nullEntry }
func (n *NullLogger) Warn() Entry           { return nullEntry }
func (n *NullLogger) Debug() Entry          { return nullEntry }
func (n *NullLogger) Info() Entry           { return nullEntry }
func (n *NullLogger) Error() Entry          { return nullEntry }
func (n *NullLogger) SetLevel(string) error { return nil }

type NullLoggerEntry struct{}

func (n *NullLoggerEntry) WithField(key string, value interface{}) Entry  { return n }
func (n *NullLoggerEntry) WithString(key string, value string) Entry      { return n }
func (n *NullLoggerEntry) WithFields(fields map[string]interface{}) Entry { return n }
func (n *NullLoggerEntry) Logf(string, ...interface{})                    {}
