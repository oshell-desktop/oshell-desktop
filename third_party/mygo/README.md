<p align="center">
<img width="800" height="360" alt="MyGo!!!!!" src="https://github.com/user-attachments/assets/c3eee9bc-5f22-4032-95b6-776e8381910a" />
</p>

**Desktop apps in Go, with a web frontend or a native UI.**

Each MyGo window shows one of two kinds of interface, and one app can mix
them:

- **A web page**, in the webview the OS already has: WKWebView on macOS,
  WebKitGTK on Linux, WebView2 on Windows. Build the frontend with any web
  tools; it calls your Go code through a TypeScript client generated from it.
- **Native UI**, written in Go alone with package `ui` and
  drawn by MyGo itself on the GPU: no HTML, no JavaScript and no webview, so
  the window opens at once and takes little memory.

Either way, an app is a single Go binary of a few megabytes, focused on low
memory and CPU use.

- **Pure Go, no cgo**: build for every platform from any machine.
- **Typed IPC** for pages: bind Go services, stream values through channels
  and send typed events; the TypeScript client is generated from your Go
  code.
- **A Go UI toolkit** for native UI: flexbox and grid layout, widgets, text
  editing with input methods, virtualized lists, SVG icons, animations,
  screen reader support, and views you test without a window; Liquid Glass
  on every platform with the glass plugin.
- **Desktop APIs** for both: windows, menus, tray, dialogs, notifications,
  global shortcuts, deep links, file associations and more.
- **Ready to ship**: app bundles and disk images, Windows installers, Debian
  packages and a Linux install script, code signing, notarization, and signed
  auto-updates with delta updates and an update window in the manner of
  Sparkle.

A window with a web page, whose frontend calls Go:

```go
type Greeter struct{}

// Greet returns a greeting.
func (Greeter) Greet(name string) string { return "Hello, " + name }

func main() {
	mygo.Bind(Greeter{})
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "Hello", URL: "/"})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
```

```ts
import { Greeter } from "./mygo"; // generated

document.body.textContent = await Greeter.greet("Ada");
```

A window with native UI, whose view is a Go function of the app's state:

```go
type counter struct{ n int }

func (s *counter) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, fmt.Sprint(s.n)).FontSize(40).Bold()
		if ui.PrimaryButton(c, "Increment").Clicked() {
			s.n++
		}
	})
}

func main() {
	s := &counter{}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:   "Counter",
			Content: ui.View(s.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
```

## Getting started

With [Go](https://go.dev/dl/) 1.27+ and [Bun](https://bun.sh), create an app
with a web frontend:

```sh
bunx mygo-cli init my-app      # or: npx mygo-cli init my-app
cd my-app
bun run dev
```

An app of native UI needs Go alone:

```sh
go run github.com/egoist/mygo/cmd/mygo@latest init -template native my-app
cd my-app
go tool mygo dev
```

Read the [documentation](docs/README.md).

## License

MIT

## OShell local patch

This copy is based on v0.3.5-0.20261008155018-bad73d119dc8. OShell adds
`ui.Context.UseSoftwareRenderer(bool)` for UI-thread use during view construction.
It switches the current native window between CPU rendering and automatic GPU
selection without replacing its UI runtime. GPU failure keeps the existing software
fallback. The implementation is in `ui/renderer_preference.go`; `ui/window.go` honors
the preference when creating a GPU renderer. No platform backend APIs were changed.
The window, including its chrome drawn by MyGo, shares one renderer.

`Painter.GlyphsScaled` supports independent horizontal and vertical scaling for
terminal line modes. GPU and software renderers use the destination glyph size;
ordinary glyphs retain their existing rasterization and fast path.

`Painter.BitmapSubpixel` retains fractional destination geometry for terminal
images, including partial edge coverage in software and GPU scenes. Ordinary
`Painter.Image` and image widgets keep their existing pixel snapping.

OShell also exposes `Painter.BitmapRegionSubpixel` for normalized fractional source regions. Terminal Unicode image placeholders intersect in double precision before submitting cell-sized geometry, sharing a bitmap and its sampling across adjacent cells without allocating cropped textures.

OShell pointer adaptation: surface presses include host double-click interval and axis distances in DIPs (with an explicit presence flag for zero intervals). Windows reads user32, GTK reads the widget settings, and AppKit supplies its own interval/click count. `InputPointerCancel` ends lost capture or window focus without release actions. Input handlers must stop their pointer gesture on this event; `Tester.CancelPointer` exercises the same route. Native data drag sessions retain ownership until their own completion.

Press events additionally preserve optional source `Timestamp` and `HasTimestamp`; zero is valid. Event queue delays do not change ordinary-control or OShell word-click timing. Windows/GTK 32-bit millisecond wrap is expanded per surface, and AppKit uses NSEvent time. The source 32-bit clock cannot disambiguate an entire 49.7-day interval.

Click counts are resolved once per window before custom handlers, survive row replacement, and are shared by ordinary controls and OShell selection. Native counts remain authoritative. Matching releases retain the press count; a drag outside the host rectangle resets the Windows/GTK estimate. Physical multi-window equivalence remains unverified.

OShell GTK terminal input reports an independent physical `BaseLayout` identity: X11 uses XKB position names, and Wayland uses GDK evdev keycodes. Unknown positions are omitted. Held identities survive layout changes through release; IME and AltGr text remain text. XKB name tables are owned copies, invalidated on keymap changes, and disconnected/released when the surface is destroyed. Actual X11/Wayland desktop validation remains pending.
