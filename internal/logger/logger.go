package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/cenkalti/log"
)

var DefaultLogHandler log.Handler

func SetLogLevel(l log.Level) { DefaultLogHandler.SetLevel(l) }

func init() {
	h := log.NewWriterHandler(os.Stderr)
	h.SetFormatter(logFormatter{})
	DefaultLogHandler = h
}

type Logger log.Logger

func New(name string) Logger {
	logger := log.NewLogger(name)
	logger.SetLevel(log.DEBUG) // forward all messages to handler
	logger.SetHandler(DefaultLogHandler)
	return logger
}

type logFormatter struct{}

// Format outputs a message like "2014-02-28 18:15:57 [example] INFO     somethinfig happened"
func (f logFormatter) Format(rec *log.Record) string {
	return fmt.Sprintf("%s %-8s [%s] %-8s %s",
		fmt.Sprint(rec.Time)[:19],
		rec.Level,
		rec.LoggerName,
		filepath.Base(rec.Filename)+":"+strconv.Itoa(rec.Line),
		rec.Message)
}