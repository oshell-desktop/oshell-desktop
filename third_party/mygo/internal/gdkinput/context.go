package gdkinput

// Context owns the identity of one GTK input target. A retired context may
// still emit asynchronous signals; its identity must never select the new
// target. Native allocation and references stay in the GTK backend.
type Context struct{ current uintptr }

func (c *Context) Handle() uintptr { return c.current }
func (c *Context) Accept(source uintptr) bool {
	return source != 0 && source == c.current
}

// Replace publishes the new identity before retiring the old context, whose
// focus-out/reset/destructor may synchronously emit signals. Allocate next
// while the old context is alive so its address cannot be reused here.
func (c *Context) Replace(next uintptr, retire func(uintptr)) {
	old := c.current
	if old == next {
		return
	}
	c.current = next
	if old != 0 {
		retire(old)
	}
}

// Filter keeps a native reference across callbacks which can switch targets
// or close the window. A key from an old target must not fall through into a
// new one, even when the old input method returned "not handled".
func (c *Context) Filter(filter func(uintptr) bool, retain, release func(uintptr)) bool {
	taken := false
	current := c.Use(func(im uintptr) { taken = filter(im) }, retain, release)
	return taken || !current
}

// Use protects any native call that can synchronously reenter application
// code. The result indicates whether it still belongs to the current target.
func (c *Context) Use(action func(uintptr), retain, release func(uintptr)) bool {
	im := c.current
	if im == 0 {
		return false
	}
	retain(im)
	defer release(im)
	action(im)
	return c.current == im
}
