module github.com/unxed/goWidgets

go 1.25.5

require (
	github.com/ebitengine/purego v0.9.0
	github.com/unxed/winkeys v0.1.1
	golang.org/x/sys v0.31.0
)

require github.com/go-webgpu/goffi v0.6.2 // indirect

replace github.com/ebitengine/purego => github.com/unxed/pureffi v0.1.19
