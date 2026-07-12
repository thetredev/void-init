package main

// cleanupStack is a LIFO stack of cleanup callbacks, unwound in reverse
// order of registration. Needed because, unlike systemd-nspawn's
// self-cleaning private mount namespace, the loop/nbd device and mount
// state this pipeline creates via qemu-nbd/mount are host-visible and
// won't clean themselves up.
type cleanupStack struct {
	fns []func()
}

// push registers fn to run on unwind, after everything pushed before it.
func (c *cleanupStack) push(fn func()) {
	c.fns = append(c.fns, fn)
}

// unwind runs every registered callback in reverse order of registration.
// Safe to call more than once: already-unwound callbacks are discarded
// after their first run.
func (c *cleanupStack) unwind() {
	fns := c.fns
	c.fns = nil

	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}
