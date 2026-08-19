package mux

import (
	"github.com/temporalio/s2s-proxy/transport/mux/session"
)

// Session state names.
const (
	SessionStateConnected = "connected"
	SessionStateClosed    = "closed"
	SessionStateErrored   = "error"
	SessionStateUnknown   = "unknown"
)

// SessionStateName names a session's state. A nil info means the state has not been recorded yet.
func SessionStateName(info *session.MuxSessionInfo) string {
	if info == nil {
		return SessionStateUnknown
	}
	switch info.State {
	case session.Connected:
		return SessionStateConnected
	case session.Closed:
		return SessionStateClosed
	case session.Error:
		return SessionStateErrored
	}
	return SessionStateUnknown
}
