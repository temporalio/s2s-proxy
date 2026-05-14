package common

import "time"

// Policy holds process-wide policy settings shared across packages.
// The Global singleton is read by subsystems (e.g. the MuxManager) at start
// time; override fields directly, or via the Update* methods, before the
// subsystems start.
type Policy struct {
	// MuxManagerStartDelay is how long the MuxManager's Start() waits for the
	// underlying MuxProvider to publish initial connections before returning.
	// A value of 0 tells Start() to skip the wait entirely.
	MuxManagerStartDelay time.Duration
}

// UpdateMuxManagerStartDelay sets MuxManagerStartDelay to d. A value of 0 is
// permitted and tells MuxManager.Start() to skip the initial wait.
func (p *Policy) UpdateMuxManagerStartDelay(d time.Duration) {
	p.MuxManagerStartDelay = d
}

// GlobalPolicy is the process-wide policy singleton.
var GlobalPolicy = &Policy{
	MuxManagerStartDelay: time.Minute,
}
