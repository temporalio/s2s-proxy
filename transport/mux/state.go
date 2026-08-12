package mux

import (
	"fmt"

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

// SessionCounts summarises the sessions held by a MultiMuxManager.
type SessionCounts struct {
	Connected int
	Errored   int
	Closed    int
	Total     int
}

// CountSessions summarises a mux manager's sessions.
func CountSessions(m MultiMuxManager) SessionCounts {
	var c SessionCounts
	if m == nil {
		return c
	}
	for _, conn := range m.GetMuxConnections() {
		c.Total++
		state := conn.State()
		if state == nil {
			continue
		}
		switch state.State {
		case session.Connected:
			c.Connected++
		case session.Error:
			c.Errored++
		case session.Closed:
			c.Closed++
		}
	}
	return c
}

// String renders the counts for a log line, as "connected=3/4 errored=0 closed=1".
func (c SessionCounts) String() string {
	return fmt.Sprintf("connected=%d/%d errored=%d closed=%d", c.Connected, c.Total, c.Errored, c.Closed)
}
