package logger

import (
	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	"os"
)

var log *logr.Logger
var logrusLog *logrus.Logger

func InitLogger() {
	logrusLog = logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})

	logg := logrusr.New(logrusLog)
	log = &logg
}

func GetLogger() *logr.Logger {
	return log
}

func GetLogrus() *logrus.Logger {
	return logrusLog
}
