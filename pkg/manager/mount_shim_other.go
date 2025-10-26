//go:build !linux

package manager

const (
	// Linux mount flags used for tests; values chosen to match linux for assertions.
	msBind    = 0x1000
	msRec     = 0x4000
	msRemount = 0x20
	msRdonly  = 0x1
)

type mountFunc func(source, target, fstype string, flags uintptr, data string) error

type unmountFunc func(target string, flags int) error

// On non-linux, provide stubs to satisfy compilation; tests override these when needed.
var unixMount mountFunc = func(source, target, fstype string, flags uintptr, data string) error { return nil }
var unixUnmount unmountFunc = func(target string, flags int) error { return nil }
