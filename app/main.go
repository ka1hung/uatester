// Command app is the desktop version of uatester (Wails v3): a simulated
// OPC UA server and a client in one window, to test the two against each
// other or against other OPC UA products.
package main

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ka1hung/uatester/core"
)

//go:embed frontend
var assets embed.FS

// AppService answers questions about the app itself.
type AppService struct {
	logs    *logHub
	dataDir string
}

// Info is shown in the window's header.
type Info struct {
	Version string `json:"version"`
	DataDir string `json:"dataDir"`
}

// Info returns the version and where certificates are kept.
func (a *AppService) Info() Info { return Info{Version: core.Version, DataDir: a.dataDir} }

// Logs returns recent log lines.
func (a *AppService) Logs() []LogLine { return a.logs.recent() }

func main() {
	// Certificates live in the user's config folder, so the server and the
	// client keep their identity, and other products trust them only once.
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	dataDir := filepath.Join(base, "uatester")

	var app atomic.Pointer[application.App]
	emit := func(name string, data any) {
		if a := app.Load(); a != nil {
			a.Event.Emit(name, data)
		}
	}
	logs := &logHub{emit: emit}

	web, err := fs.Sub(assets, "frontend")
	if err != nil {
		log.Fatal(err)
	}
	a := application.New(application.Options{
		Name:        "uatester",
		Description: "OPC UA server and client test tool",
		Services: []application.Service{
			application.NewService(&AppService{logs: logs, dataDir: dataDir}),
			application.NewService(&ServerService{logs: logs, pkiDir: filepath.Join(dataDir, "server-pki")}),
			application.NewService(&ClientService{logs: logs, emit: emit, pkiDir: filepath.Join(dataDir, "client-pki")}),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(web)},
		Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	app.Store(a)
	a.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "uatester " + core.Version,
		Width:     1440,
		Height:    900,
		MinWidth:  1100,
		MinHeight: 700,
		URL:       "/",
	})
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
