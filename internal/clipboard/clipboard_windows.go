//go:build windows

package clipboard

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procOpenClipboard      = user32.NewProc("OpenClipboard")
	procCloseClipboard     = user32.NewProc("CloseClipboard")
	procEmptyClipboard     = user32.NewProc("EmptyClipboard")
	procSetClipboardData   = user32.NewProc("SetClipboardData")
	procGetClipboardData   = user32.NewProc("GetClipboardData")
	procRegisterClipFormat = user32.NewProc("RegisterClipboardFormatW")

	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalFree   = kernel32.NewProc("GlobalFree")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// These three registered formats are how Windows is told to keep a clipboard
// value out of Clipboard History and out of the cloud clipboard that syncs
// between a user's machines. Their presence on the clipboard is the signal;
// the payload is a single zero DWORD.
//
// Support is best effort: an older Windows 10 build without the history
// feature simply ignores formats it does not know, and the copy still works.
var excludeFormats = []string{
	"ExcludeClipboardContentFromMonitorProcessing",
	"CanIncludeInClipboardHistory",
	"CanUploadToCloudClipboard",
}

// Available reports whether the clipboard can be used. The Win32 clipboard is
// always present.
func Available() bool { return true }

func registerFormat(name string) (uint32, error) {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	r, _, err := procRegisterClipFormat.Call(uintptr(unsafe.Pointer(p)))
	if r == 0 {
		return 0, err
	}
	return uint32(r), nil //nolint:gosec // a registered format is a UINT, 0xC000 through 0xFFFF
}

// openClipboard retries because another process can hold the clipboard open
// for a moment; Windows offers no wait, so a short retry is the accepted way.
func openClipboard() error {
	var lastErr error
	for i := 0; i < 10; i++ {
		r, _, err := procOpenClipboard.Call(0)
		if r != 0 {
			return nil
		}
		lastErr = err
		windows.SleepEx(10, false)
	}
	return fmt.Errorf("clipboard: cannot open: %w", lastErr)
}

// allocGlobal copies data into a moveable global block, which is the
// ownership model SetClipboardData expects: on success the clipboard owns the
// block and we must not free it.
func allocGlobal(data []byte) (uintptr, error) {
	h, _, err := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if h == 0 {
		return 0, err
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		_, _, _ = procGlobalFree.Call(h)
		return 0, err
	}
	// go vet reports this uintptr to unsafe.Pointer conversion, and for Go
	// heap memory it would be right: the collector could move the object
	// between the two operations. This block comes from GlobalAlloc, so it
	// is owned by the Win32 heap, is pinned for the duration of the lock,
	// and is invisible to the collector. There is no safe alternative for
	// reaching HGLOBAL memory.
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data)), data) //nolint:govet // Win32 heap, not Go heap
	_, _, _ = procGlobalUnlock.Call(h)
	return h, nil
}

func write(value string) error {
	// The clipboard belongs to the thread that opened it, and Go is free to
	// move a goroutine to another thread between two calls. A CloseClipboard
	// from the wrong thread fails and leaves the clipboard open, which locks
	// every other program out of it until this process exits.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := openClipboard(); err != nil {
		return err
	}
	defer func() { _, _, _ = procCloseClipboard.Call() }()

	if r, _, err := procEmptyClipboard.Call(); r == 0 {
		return fmt.Errorf("clipboard: cannot empty: %w", err)
	}
	if value == "" {
		return nil
	}

	// Set the exclusion formats before the text, so a history monitor that
	// reacts to the text already sees them.
	for _, name := range excludeFormats {
		id, err := registerFormat(name)
		if err != nil {
			continue
		}
		h, err := allocGlobal(make([]byte, 4))
		if err != nil {
			continue
		}
		if r, _, _ := procSetClipboardData.Call(uintptr(id), h); r == 0 {
			_, _, _ = procGlobalFree.Call(h)
		}
	}

	utf16, err := windows.UTF16FromString(value)
	if err != nil {
		return err
	}
	h, err := allocGlobal(unsafe.Slice((*byte)(unsafe.Pointer(&utf16[0])), len(utf16)*2))
	if err != nil {
		return err
	}
	if r, _, err := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		_, _, _ = procGlobalFree.Call(h)
		return fmt.Errorf("clipboard: cannot set text: %w", err)
	}
	return nil
}

func read() (string, error) {
	// Same thread rule as write.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := openClipboard(); err != nil {
		return "", err
	}
	defer func() { _, _, _ = procCloseClipboard.Call() }()

	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		// An empty clipboard has no CF_UNICODETEXT at all.
		return "", nil
	}
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		return "", err
	}
	defer func() { _, _, _ = procGlobalUnlock.Call(h) }()
	// Same as allocGlobal: the clipboard's block lives on the Win32 heap
	// and is pinned while locked.
	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(p))), nil //nolint:govet // Win32 heap, not Go heap
}
