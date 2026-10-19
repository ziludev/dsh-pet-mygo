// dsh-pet-mygo is a feasibility probe: can MyGo's transparent always-on-top
// frameless window render a HEVC-with-alpha .mov transparently through the
// system webview (WKWebView on macOS)? It loads a tiny self-generated sample
// (a solid square over a fully transparent background) and shows it as a
// floating desktop sprite. If the square floats with no black box around it,
// the gate passes and a full dsh-pet desktop port is viable.
package main

import (
	"log"
	"runtime"

	"github.com/egoist/mygo"
)

func main() {
	mygo.App.WhenReady(func() {
		opts := mygo.WindowOptions{
			Title:       "dsh-pet mygo probe",
			Width:       320,
			Height:      320,
			X:           400,
			Y:           700,
			Frameless:   true,
			Transparent: true,
			AlwaysOnTop: true,
			SkipTaskbar: true,
		}
		// Vibrancy is macOS/Windows only; a translucent blurred material
		// behind the sprite is not wanted here, so leave it off.
		_ = runtime.GOOS

		win := mygo.NewWindow(opts)
		if err := win.Page().LoadFile("resources/index.html"); err != nil {
			log.Fatalf("load probe page: %v", err)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
