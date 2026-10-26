package ui

// KeepFocusOnClick leaves keyboard focus on the current editor when this
// element or its descendants are clicked. Useful for non-modal copy controls.
func (e Element) KeepFocusOnClick() Element {
	if n := e.node(); n != nil {
		n.flags |= flagKeepFocus
	}
	return e
}
