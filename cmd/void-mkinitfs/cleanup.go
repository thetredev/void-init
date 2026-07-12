package main

import "sync"

// cleanupStack is a LIFO stack of cleanup callbacks, unwound in reverse
// order of registration. Needed because, unlike systemd-nspawn's
// self-cleaning private mount namespace, the loop/nbd device and mount
// state this pipeline creates via qemu-nbd/mount are host-visible and
// won't clean themselves up.
type cleanupStack struct {
	mu  sync.Mutex
	fns []func()
}

// push registers fn to run on unwind, after everything pushed before it.
func (c *cleanupStack) push(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fns = append(c.fns, fn)
}

// unwind runs every registered callback in reverse order of registration.
// Safe to call more than once, including concurrently: main's deferred
// unwind and the signal handler's can race, and the mutex makes the
// second caller block until the first finishes, then find an empty stack
// and run nothing - rather than double-running callbacks or exiting
// mid-unmount.
func (c *cleanupStack) unwind() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := len(c.fns) - 1; i >= 0; i-- {
		c.fns[i]()
	}
	c.fns = nil
}
