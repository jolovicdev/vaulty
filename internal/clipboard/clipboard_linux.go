//go:build linux

package clipboard

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
)

// Linux has no clipboard in the kernel or the standard library, so this
// backend drives whichever helper the session provides.
//
// Known limits, stated rather than papered over:
//   - Wayland needs wl-clipboard. Without it there is no way to set the
//     clipboard from a process that is not the focused toplevel, and this
//     package reports ErrUnavailable.
//   - wl-copy keeps a process alive to serve the data. Clearing works by
//     running "wl-copy --clear", which kills that server; writing an empty
//     string is not enough.
//   - Reading back under Wayland can fail while our window is not focused.
//     clearIfOurs treats a failed read as "clear anyway".
type backend struct {
	copyCmd  []string
	pasteCmd []string
	// clearCmd is used instead of copying an empty string when the helper
	// needs it.
	clearCmd []string
}

func detect() (backend, bool) {
	wayland := os.Getenv("WAYLAND_DISPLAY") != ""

	if wayland {
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return backend{
				copyCmd:  []string{"wl-copy", "--type", "text/plain;charset=utf-8"},
				pasteCmd: []string{"wl-paste", "--no-newline"},
				clearCmd: []string{"wl-copy", "--clear"},
			}, true
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		return backend{
			copyCmd:  []string{"xclip", "-selection", "clipboard", "-in"},
			pasteCmd: []string{"xclip", "-selection", "clipboard", "-out"},
		}, true
	}
	if _, err := exec.LookPath("xsel"); err == nil {
		return backend{
			copyCmd:  []string{"xsel", "--clipboard", "--input"},
			pasteCmd: []string{"xsel", "--clipboard", "--output"},
		}, true
	}
	// wl-copy also works under XWayland sessions that set DISPLAY only.
	if _, err := exec.LookPath("wl-copy"); err == nil {
		return backend{
			copyCmd:  []string{"wl-copy", "--type", "text/plain;charset=utf-8"},
			pasteCmd: []string{"wl-paste", "--no-newline"},
			clearCmd: []string{"wl-copy", "--clear"},
		}, true
	}
	return backend{}, false
}

// Available reports whether this system has a clipboard helper, so the UI can
// say so before the user tries to copy.
func Available() bool {
	_, ok := detect()
	return ok
}

func write(value string) error {
	b, ok := detect()
	if !ok {
		return ErrUnavailable
	}
	if value == "" && len(b.clearCmd) > 0 {
		//nolint:gosec // every argument comes from the table in detect()
		return exec.Command(b.clearCmd[0], b.clearCmd[1:]...).Run()
	}

	//nolint:gosec // every argument comes from the table in detect()
	cmd := exec.Command(b.copyCmd[0], b.copyCmd[1:]...)
	// The secret goes over stdin, never as an argument, so it cannot appear
	// in another user's ps output.
	cmd.Stdin = strings.NewReader(value)
	return cmd.Run()
}

func read() (string, error) {
	b, ok := detect()
	if !ok {
		return "", ErrUnavailable
	}
	var out bytes.Buffer
	//nolint:gosec // every argument comes from the table in detect()
	cmd := exec.Command(b.pasteCmd[0], b.pasteCmd[1:]...)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}
