package core

// OutputActivity ignores terminal control payloads, retaining state across reads.
// A title/OSC update, cursor movement or whitespace alone is not unread output.
type OutputActivity struct{ state uint8 }

func (a *OutputActivity) Consume(data []byte) bool {
	visible := false
	for _, b := range data {
		switch a.state {
		case 0:
			if b == 27 {
				a.state = 1
			} else if b == 7 || b > 32 && b != 127 {
				visible = true
			}
		case 1:
			switch b {
			case '[':
				a.state = 3
			case ']', 'P', '^', '_', 'X':
				a.state = 4
			default:
				if b >= 0x20 && b <= 0x2f {
					a.state = 2
				} else {
					a.state = 0
				}
			}
		case 2:
			if b >= 0x30 && b <= 0x7e {
				a.state = 0
			}
		case 3:
			if b == 27 {
				a.state = 1
			} else if b >= 0x40 && b <= 0x7e {
				a.state = 0
			}
		case 4:
			if b == 7 {
				a.state = 0
			} else if b == 27 {
				a.state = 5
			}
		case 5:
			if b == '\\' {
				a.state = 0
			} else if b != 27 {
				a.state = 4
			}
		}
	}
	return visible
}
