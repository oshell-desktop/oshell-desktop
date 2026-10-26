package ui

import (
	"runtime"

	"github.com/egoist/mygo/internal/platform"
)

// Key identifies a key. Letter and digit keys follow the keyboard layout:
// KeyA is the key that types an a.
type Key uint16

// Keys.
const (
	KeyUnknown      = Key(platform.KeyUnknown)
	KeyEnter        = Key(platform.KeyEnter)
	KeyEscape       = Key(platform.KeyEscape)
	KeyBackspace    = Key(platform.KeyBackspace)
	KeyTab          = Key(platform.KeyTab)
	KeySpace        = Key(platform.KeySpace)
	KeyDelete       = Key(platform.KeyDelete)
	KeyInsert       = Key(platform.KeyInsert)
	KeyHome         = Key(platform.KeyHome)
	KeyEnd          = Key(platform.KeyEnd)
	KeyPageUp       = Key(platform.KeyPageUp)
	KeyPageDown     = Key(platform.KeyPageDown)
	KeyLeft         = Key(platform.KeyLeft)
	KeyRight        = Key(platform.KeyRight)
	KeyUp           = Key(platform.KeyUp)
	KeyDown         = Key(platform.KeyDown)
	KeyF1           = Key(platform.KeyF1)
	KeyF2           = Key(platform.KeyF2)
	KeyF3           = Key(platform.KeyF3)
	KeyF4           = Key(platform.KeyF4)
	KeyF5           = Key(platform.KeyF5)
	KeyF6           = Key(platform.KeyF6)
	KeyF7           = Key(platform.KeyF7)
	KeyF8           = Key(platform.KeyF8)
	KeyF9           = Key(platform.KeyF9)
	KeyF10          = Key(platform.KeyF10)
	KeyF11          = Key(platform.KeyF11)
	KeyF12          = Key(platform.KeyF12)
	KeyMinus        = Key(platform.KeyMinus)
	KeyEqual        = Key(platform.KeyEqual)
	KeyComma        = Key(platform.KeyComma)
	KeyPeriod       = Key(platform.KeyPeriod)
	KeySlash        = Key(platform.KeySlash)
	KeySemicolon    = Key(platform.KeySemicolon)
	KeyQuote        = Key(platform.KeyQuote)
	KeyBracketLeft  = Key(platform.KeyBracketLeft)
	KeyBracketRight = Key(platform.KeyBracketRight)
	KeyBackslash    = Key(platform.KeyBackslash)
	KeyBackquote    = Key(platform.KeyBackquote)
	KeyContextMenu  = Key(platform.KeyContextMenu)
	Key0            = Key(platform.Key0)
	Key1            = Key(platform.Key1)
	Key2            = Key(platform.Key2)
	Key3            = Key(platform.Key3)
	Key4            = Key(platform.Key4)
	Key5            = Key(platform.Key5)
	Key6            = Key(platform.Key6)
	Key7            = Key(platform.Key7)
	Key8            = Key(platform.Key8)
	Key9            = Key(platform.Key9)
	KeyA            = Key(platform.KeyA)
	KeyB            = Key(platform.KeyB)
	KeyC            = Key(platform.KeyC)
	KeyD            = Key(platform.KeyD)
	KeyE            = Key(platform.KeyE)
	KeyF            = Key(platform.KeyF)
	KeyG            = Key(platform.KeyG)
	KeyH            = Key(platform.KeyH)
	KeyI            = Key(platform.KeyI)
	KeyJ            = Key(platform.KeyJ)
	KeyK            = Key(platform.KeyK)
	KeyL            = Key(platform.KeyL)
	KeyM            = Key(platform.KeyM)
	KeyN            = Key(platform.KeyN)
	KeyO            = Key(platform.KeyO)
	KeyP            = Key(platform.KeyP)
	KeyQ            = Key(platform.KeyQ)
	KeyR            = Key(platform.KeyR)
	KeyS            = Key(platform.KeyS)
	KeyT            = Key(platform.KeyT)
	KeyU            = Key(platform.KeyU)
	KeyV            = Key(platform.KeyV)
	KeyW            = Key(platform.KeyW)
	KeyX            = Key(platform.KeyX)
	KeyY            = Key(platform.KeyY)
	KeyZ            = Key(platform.KeyZ)
	// KeyBack and KeyForward go back and forward in a history: the side
	// buttons of a mouse, and the keys of keyboards that have them. A
	// Router takes them.
	KeyBack     = Key(platform.KeyBack)
	KeyForward  = Key(platform.KeyForward)
	KeyAdd      = Key(platform.KeyAdd)
	KeyMultiply = Key(platform.KeyMultiply)
	KeyClear    = Key(platform.KeyClear)
	KeyF13      = Key(platform.KeyF13)
	KeyF14      = Key(platform.KeyF14)
	KeyF15      = Key(platform.KeyF15)
	KeyF16      = Key(platform.KeyF16)
	KeyF17      = Key(platform.KeyF17)
	KeyF18      = Key(platform.KeyF18)
	KeyF19      = Key(platform.KeyF19)
	KeyF20      = Key(platform.KeyF20)
	KeyF21      = Key(platform.KeyF21)
	KeyF22      = Key(platform.KeyF22)
	KeyF23      = Key(platform.KeyF23)
	KeyF24      = Key(platform.KeyF24)
	KeyF25      = Key(platform.KeyF25)
	KeyF26      = Key(platform.KeyF26)
	KeyF27      = Key(platform.KeyF27)
	KeyF28      = Key(platform.KeyF28)
	KeyF29      = Key(platform.KeyF29)
	KeyF30      = Key(platform.KeyF30)
	KeyF31      = Key(platform.KeyF31)
	KeyF32      = Key(platform.KeyF32)
	KeyF33      = Key(platform.KeyF33)
	KeyF34      = Key(platform.KeyF34)
	KeyF35      = Key(platform.KeyF35)
)

// Modifiers are modifier keys.
type Modifiers uint8

// Modifier keys.
const (
	Shift = Modifiers(platform.ModShift)
	Ctrl  = Modifiers(platform.ModCtrl)
	Alt   = Modifiers(platform.ModAlt)
	// Super is Command on macOS and the Windows key elsewhere.
	Super = Modifiers(platform.ModSuper)
)

// Cmd is the modifier of the platform's shortcuts: Command on macOS,
// Control elsewhere, as in Cmd+C.
var Cmd = func() Modifiers {
	if runtime.GOOS == "darwin" {
		return Super
	}
	return Ctrl
}()

// Cursor is a pointer shape.
type Cursor uint8

// Pointer shapes.
const (
	CursorDefault    = Cursor(platform.CursorDefault)
	CursorPointer    = Cursor(platform.CursorPointer)
	CursorText       = Cursor(platform.CursorText)
	CursorMove       = Cursor(platform.CursorMove)
	CursorResizeEW   = Cursor(platform.CursorResizeEW)
	CursorResizeNS   = Cursor(platform.CursorResizeNS)
	CursorResizeNWSE = Cursor(platform.CursorResizeNWSE)
	CursorResizeNESW = Cursor(platform.CursorResizeNESW)
	CursorNotAllowed = Cursor(platform.CursorNotAllowed)
	CursorCrosshair  = Cursor(platform.CursorCrosshair)
	CursorGrab       = Cursor(platform.CursorGrab)
	CursorGrabbing   = Cursor(platform.CursorGrabbing)
	// CursorResizeN, E, S and W resize toward one side only, as at the
	// limit of a resize.
	CursorResizeN = Cursor(platform.CursorResizeN)
	CursorResizeE = Cursor(platform.CursorResizeE)
	CursorResizeS = Cursor(platform.CursorResizeS)
	CursorResizeW = Cursor(platform.CursorResizeW)
	// CursorResizeColumn and CursorResizeRow move the line between columns
	// or rows, as of a table or a split.
	CursorResizeColumn = Cursor(platform.CursorResizeColumn)
	CursorResizeRow    = Cursor(platform.CursorResizeRow)
	CursorVerticalText = Cursor(platform.CursorVerticalText)
	// CursorCopy and CursorAlias show that a drop copies or links what is
	// dragged.
	CursorCopy        = Cursor(platform.CursorCopy)
	CursorAlias       = Cursor(platform.CursorAlias)
	CursorContextMenu = Cursor(platform.CursorContextMenu)
	// CursorNone hides the pointer over the element, as over a video.
	CursorNone = Cursor(platform.CursorNone)
)
