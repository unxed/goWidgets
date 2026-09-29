//go:build darwin

package cocoa

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/ebitengine/purego/objc"
	"github.com/unxed/goWidgets/core"
)

// appKitString is one of AppKit's own translated button titles, so the box
// speaks the user's language as the system's own do; where AppKit has no
// such entry the English word comes back unchanged.
func appKitString(key string) objc.ID {
	bundle := msg(cls("NSBundle"), "bundleWithIdentifier:", nsString("com.apple.AppKit"))
	if bundle == 0 {
		return nsString(key)
	}
	return msg(bundle, "localizedStringForKey:value:table:", nsString(key), nsString(key), nsString("Common"))
}

var (
	dialogMu      sync.Mutex
	dialogCurrent objc.ID // the NSAlert or NSSavePanel on screen
)

func setDialog(d objc.ID) {
	dialogMu.Lock()
	dialogCurrent = d
	dialogMu.Unlock()
}

// runAlert is NSAlert's runModal: application-modal, a nested loop in
// NSModalPanelRunLoopMode — one of the common modes the main dispatch queue
// is served in, so the application's queue keeps draining. The first button
// is the default (Return); the second answers Escape, and Escape or any
// other way out reads as the cautious answer.
func runAlert(kind core.DialogKind, title, text string) core.DialogResult {
	a := alloc("NSAlert")
	defer msg(a, "release")
	msg(a, "setMessageText:", nsString(title))
	msg(a, "setInformativeText:", nsString(text))
	const informational, warning = 1, 0
	var first, second string
	switch kind {
	case core.DialogConfirm:
		first, second = "OK", "Cancel"
		msg(a, "setAlertStyle:", uint64(warning))
	case core.DialogYesNo:
		first, second = "Yes", "No"
		msg(a, "setAlertStyle:", uint64(warning))
	default:
		first = "OK"
		msg(a, "setAlertStyle:", uint64(informational))
	}
	msg(a, "addButtonWithTitle:", appKitString(first))
	if second != "" {
		b := msg(a, "addButtonWithTitle:", appKitString(second))
		msg(b, "setKeyEquivalent:", nsString("\x1b"))
	}

	setDialog(a)
	r := int64(msg(a, "runModal"))
	setDialog(0)

	accepted := r == alertFirstButton
	switch kind {
	case core.DialogConfirm:
		if accepted {
			return core.DialogOK
		}
		return core.DialogCancel
	case core.DialogYesNo:
		if accepted {
			return core.DialogYes
		}
		return core.DialogNo
	}
	return core.DialogOK
}

// runPanel is NSOpenPanel or NSSavePanel, run modally. A filter's patterns
// become file extensions ("*.txt" → "txt"); a pattern that is not a plain
// extension switches filtering off rather than hiding files wrongly. The
// save panel asks before overwriting by itself.
func runPanel(save bool, title, suggested string, filters []core.FileFilter) (string, bool) {
	var p objc.ID
	if save {
		p = msg(cls("NSSavePanel"), "savePanel")
		if suggested != "" {
			if dir := filepath.Dir(suggested); dir != "." {
				msg(p, "setDirectoryURL:", msg(cls("NSURL"), "fileURLWithPath:", nsString(dir)))
			}
			msg(p, "setNameFieldStringValue:", nsString(filepath.Base(suggested)))
		}
	} else {
		p = msg(cls("NSOpenPanel"), "openPanel")
		msg(p, "setCanChooseFiles:", true)
		msg(p, "setCanChooseDirectories:", false)
		msg(p, "setAllowsMultipleSelection:", false)
	}
	msg(p, "setTitle:", nsString(title))
	msg(p, "setMessage:", nsString(title)) // modern panels show no title bar
	if exts, ok := extensions(filters); ok {
		msg(p, "setAllowedFileTypes:", nsArray(exts))
	}

	setDialog(p)
	r := int64(msg(p, "runModal"))
	setDialog(0)
	if r != modalResponseOK {
		return "", false
	}
	path := goString(msg(msg(p, "URL"), "path"))
	return path, path != ""
}

func extensions(filters []core.FileFilter) ([]string, bool) {
	var exts []string
	for _, f := range filters {
		for _, pat := range f.Patterns {
			ext, ok := strings.CutPrefix(pat, "*.")
			if !ok || ext == "" || strings.ContainsAny(ext, "*?[") {
				return nil, false
			}
			exts = append(exts, ext)
		}
	}
	return exts, len(exts) > 0
}
