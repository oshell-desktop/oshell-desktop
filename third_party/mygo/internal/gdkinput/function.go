package gdkinput

import "github.com/egoist/mygo/internal/platform"

// FunctionKey maps the contiguous GDK_KEY_F1..F35 symbols. The platform
// enum preserves its earlier values, so F13..F35 form a separate range.
func FunctionKey(symbol uint32) platform.Key {
	if symbol >= 0xffbe && symbol <= 0xffc9 {
		return platform.KeyF1 + platform.Key(symbol-0xffbe)
	}
	if symbol >= 0xffca && symbol <= 0xffe0 {
		return platform.KeyF13 + platform.Key(symbol-0xffca)
	}
	return platform.KeyUnknown
}
