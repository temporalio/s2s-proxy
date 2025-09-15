package mux

import (
	"fmt"
	"strings"

	"go.temporal.io/server/common/log"
)

// wrapLoggerForYamux converts a temporal Logger to a Yamux logger
type wrapLoggerForYamux struct {
	logger log.Logger
}

// determineLevel returns the appropriate log level for a Yamux log string.
// Yamux logs have the format "[<level>] yamux: <message>". At time of writing, only ERR and WARN are logged
func (l wrapLoggerForYamux) parseLogLevel(s string) {
	if strings.HasPrefix(s, "[ERR]") {
		l.logger.Error(s)
	} else if strings.HasPrefix(s, "[WARN]") {
		l.logger.Warn(s)
	} else {
		l.logger.Info(s)
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
