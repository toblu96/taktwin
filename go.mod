module github.com/toblu96/taktwin

go 1.26.0

require (
	github.com/gopcua/opcua v0.9.1
	github.com/toblu96/taktwin/st v0.0.0
	github.com/wailsapp/wails/v3 v3.1.0
)

require (
	github.com/coder/websocket v1.8.14 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.46.0 // indirect
)

// The Structured Text compiler and runtime is a module of its own, in this repository.
replace github.com/toblu96/taktwin/st => ./st
