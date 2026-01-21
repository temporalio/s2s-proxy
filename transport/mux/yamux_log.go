package mux

import (
	"fmt"
	"strings"

	"go.temporal.io/server/common/log"
)

type LogLevelCap int

const (
	DemoteToDebug LogLevelCap = iota
	DemoteToInfo
	DemoteToWarn
	DemoteToError
	NoDemote
)

// wrapLoggerForYamux converts a temporal Logger to a Yamux logger
type wrapLoggerForYamux struct {
	maxLogLevel LogLevelCap // Note: Defaults to debug
	logger      log.Logger
}

// determineLevel returns the appropriate log level for a Yamux log string.
// Yamux logs have the format "[<level>] yamux: <message>". At time of writing, only ERR and WARN are logged
func (l wrapLoggerForYamux) parseLogLevel(s string) {
	if strings.HasPrefix(s, "[ERR]") {
		l.logAtLevel(s, min(l.maxLogLevel, DemoteToError))
	} else if strings.HasPrefix(s, "[WARN]") {
		l.logAtLevel(s, min(l.maxLogLevel, DemoteToWarn))
	} else {
		l.logAtLevel(s, min(l.maxLogLevel, DemoteToInfo))
	}
}

func (l wrapLoggerForYamux) logAtLevel(s string, level LogLevelCap) {
	switch level {
	case DemoteToDebug:
		l.logger.Debug(s)
	case DemoteToInfo:
		l.logger.Info(s)
	case DemoteToWarn:
		l.logger.Warn(s)
	case DemoteToError:
		l.logger.Error(s)
	case NoDemote:
		l.logger.Fatal(s)
	}
}

func (l wrapLoggerForYamux) Print(v ...interface{}) {
	l.parseLogLevel(fmt.Sprint(v...))
}
func (l wrapLoggerForYamux) Printf(format string, v ...interface{}) {
	l.parseLogLevel(fmt.Sprintf(format, v...))
}
func (l wrapLoggerForYamux) Println(v ...interface{}) {
	l.parseLogLevel(fmt.Sprintln(v...))
}
