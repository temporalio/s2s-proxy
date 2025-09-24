package broadcaster

import (
	"sync"
)

// Code adapted from Jonas Jasas's CondChan package: https://gitlab.com/jonas.jasas/condchan/-/blob/master/condchan.go?ref_type=heads

// Broadcaster implements waitable notification associated with a provided Locker. This is weaker than a sync.Cond
// because it does not implement wait lists, it just provides a channel, then closes the channel to notify.
// There is an overhead of one channel creation per call to Broadcaster.Broadcast
// A Broadcaster must not be copied after first use.
type (
	noCopy    struct{}
	Broadcaster struct {
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

// New returns a new Broadcaster with Locker l.
func New(l sync.Locker) *Broadcaster {
	return &Broadcaster{
		L:   l,
		ch:  make(chan struct{}),
		chL: sync.RWMutex{},
	}
}

// WaitWithSelect atomically unlocks cc.L and executes fn.
// After later resuming execution, Wait locks cc.L before returning.
//
// fn is executed passing channel in to it.
// Passed channel will signal by emitting struct{} or by closing.
// Inside fn should be select statement using passed channel together with the other channels that signals execution continuation.
func (cc *Broadcaster) WaitWithSelect(fn selectFn) {
	cc.chL.RLock()
	ch := cc.ch
	cc.chL.RUnlock()

	cc.L.Unlock()
	fn(ch)
	cc.L.Lock()
}

// Broadcast wakes all goroutines waiting on cc.
// It is allowed but not required for the caller to hold cc.L during the call.
func (cc *Broadcaster) Broadcast() {
	cc.chL.Lock()
	close(cc.ch)
	cc.ch = make(chan struct{})
	cc.chL.Unlock()
}
