package ui

// ClipboardGeneration identifies native ownership/content changes, including
// replacement followed by restoring the same text. False means unavailable;
// callers holding private clipboard snapshots must fail closed in that case.
func (c *Context) ClipboardGeneration() (uint64, bool) {
	ctx := c.build()
	if ctx == nil {
		return 0, false
	}
	if h, ok := ctx.rt.host.(interface{ clipboardGeneration() (uint64, bool) }); ok {
		return h.clipboardGeneration()
	}
	return 0, false
}

func (h *windowHost) clipboardGeneration() (uint64, bool) {
	if c, ok := h.conn.Clipboard.(interface{ Generation() (uint64, bool) }); ok {
		return c.Generation()
	}
	return 0, false
}

func (h *headless) clipboardGeneration() (uint64, bool) { return h.clipboardVersion, true }
