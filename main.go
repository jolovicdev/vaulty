package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// The whole frontend, including both fonts, is compiled into the binary.
// There is no path by which this app loads a resource over the network.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "Vaulty",
		Width:     1180,
		Height:    760,
		MinWidth:  860,
		MinHeight: 520,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// The webview's own context menu would offer reload and inspect in a
		// production build; the app supplies its own actions instead.
		EnableDefaultContextMenu: false,
		// Wails only opens the inspector when the binary is built without
		// the production tag, which is what the Makefile's release target
		// passes. This field keeps that explicit rather than implied.
		Debug: options.Debug{
			OpenInspectorOnStartup: false,
		},
		OnStartup:     app.startup,
		OnShutdown:    app.shutdown,
		OnBeforeClose: app.beforeClose,
		Bind: []any{
			app,
		},
		Windows: &windows.Options{
			// The webview is asked to follow the OS theme; the frontend does
			// the same through prefers-color-scheme, so the window chrome and
			// the content agree.
			Theme: windows.SystemDefault,
			// Whether a missing WebView2 runtime is downloaded is decided by
			// a build tag, not by this struct: Wails compiles in a
			// bootstrapper download unless one of the wv2runtime.* tags is
			// set. The Makefile passes wv2runtime.error so that a missing
			// runtime is reported to the user and no network code is linked
			// in at all.
			Messages: &windows.Messages{
				Webview2NotInstalled: "Vaulty needs the Microsoft Edge WebView2 runtime, " +
					"which is not installed. Install it, then start Vaulty again. " +
					"Vaulty does not download anything itself.",
			},
		},
		Linux: &linux.Options{
			ProgramName: "vaulty",
			// WebKitGTK's DMA-BUF renderer is broken on several drivers
			// under Wayland and shows a blank window; the flag is the
			// documented workaround and costs nothing elsewhere.
			WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand,
		},
	})
	if err != nil {
		// There is no logger yet at this point and nothing here is secret.
		panic(err)
	}
}
