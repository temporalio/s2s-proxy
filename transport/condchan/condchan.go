package condchan

import (
	"sync"
)

// Code adapted from Jonas Jasas's CondChan package: https://gitlab.com/jonas.jasas/condchan/-/blob/master/condchan.go?ref_type=heads

// CondChan implements a condition variable, a rendezvous point for goroutines waiting for or announcing the occurrence
// of an event.
//
// Each Cond has an associated Locker L (often a *Mutex or *RWMutex),
// which must be held when changing the condition and when calling the Wait method.
//
// A Cond must not be copied after first use.
type (
	noCopy   struct{}
	CondChan struct {
		L   sync.Locker
		ch  chan struct{}
		chL sync.RWMutex

		_ noCopy
	}
)

// go vet is looking for Lock and Unlock on the noCopy struct

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

type selectFn func(<-chan struct{})

// New returns a new CondChan with Locker l.
func New(l sync.Locker) *CondChan {
	return &CondChan{
		L:   l,
		ch:  make(chan struct{}),
		chL: sync.RWMutex{},
	}
}

// WithSelectFn atomically unlocks cc.L and executes fn.
// After later resuming execution, Wait locks cc.L before returning.
//
// fn is executed passing channel in to it.
// Passed channel will signal by emitting struct{} or by closing.
// Inside fn should be select statement using passed channel together with the other channels that signals execution continuation.
func (cc *CondChan) WithSelectFn(fn selectFn) {
	cc.chL.RLock()
	ch := cc.ch
	cc.chL.RUnlock()

	cc.L.Unlock()
	fn(ch)
	cc.L.Lock()
}

// Wait atomically unlocks cc.L and suspends execution of the calling goroutine.
// After later resuming execution, Wait locks cc.L before returning.
// Unlike in other systems, Wait cannot return unless awoken by Broadcast or Signal.
func (cc *CondChan) Wait() {
	cc.chL.RLock()
	ch := cc.ch
	cc.chL.RUnlock()

	cc.L.Unlock()
	<-ch
	cc.L.Lock()
}

// Signal wakes one goroutine waiting on cc, if there is any.
// It is allowed but not required for the caller to hold cc.L during the call.
func (cc *CondChan) Signal() {
	cc.chL.RLock()
	select {
	case cc.ch <- struct{}{}:
	default:
	}
	cc.chL.RUnlock()
}

// Broadcast wakes all goroutines waiting on cc.
// It is allowed but not required for the caller to hold cc.L during the call.
func (cc *CondChan) Broadcast() {
	cc.chL.Lock()
	close(cc.ch)
	cc.ch = make(chan struct{})
	cc.chL.Unlock()
}
