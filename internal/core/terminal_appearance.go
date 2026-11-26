package core

// TerminalAppearance contains portable rendering preferences. An absent object
// leaves each client using its original terminal layout and font policy.
type TerminalAppearance struct {
	Top               int    `json:"top"`
	Bottom            int    `json:"bottom"`
	Left              int    `json:"left"`
	Right             int    `json:"right"`
	LineSpacing       int    `json:"lineSpacing"`
	CharacterSpacing  int    `json:"characterSpacing"`
	CursorShape       string `json:"cursorShape"`
	CursorBlink       bool   `json:"cursorBlink"`
	BlinkInterval     int    `json:"blinkInterval"`
	BoldStyle         string `json:"boldStyle"`
	AsianFont         string `json:"asianFont"`
	AsianSize         int    `json:"asianSize"`
	FontWeight        int    `json:"fontWeight"`
	AsianWeight       int    `json:"asianWeight"`
	AllowProportional bool   `json:"allowProportional"`
	FontQuality       string `json:"fontQuality"`
	TabColor          string `json:"tabColor"`
	ImagePosition     string `json:"imagePosition"`
}

func DefaultTerminalAppearance() TerminalAppearance {
	return TerminalAppearance{
		Top: 5, Bottom: 5, Left: 5, Right: 5, CursorShape: "block",
		BlinkInterval: 600, BoldStyle: "both", FontWeight: 400,
		AllowProportional: true, FontQuality: "system", ImagePosition: "center",
	}
}
