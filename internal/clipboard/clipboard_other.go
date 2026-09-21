//go:build !linux && !windows

package clipboard

// The project targets Windows and Linux. Other platforms compile, so go vet
// and go test work there, but report no clipboard rather than pretending.

// Available reports whether this system has a clipboard helper.
func Available() bool { return false }

func write(string) error { return ErrUnavailable }

func read() (string, error) { return "", ErrUnavailable }
