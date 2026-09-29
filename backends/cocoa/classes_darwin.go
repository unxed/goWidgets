//go:build darwin

package cocoa

import (
	"github.com/ebitengine/purego/objc"
	"github.com/unxed/goWidgets/core"
)

// registerClasses creates the three Objective-C classes the driver needs.
// There is no compiler here to write @implementation blocks with, so the
// classes are made at run time and their methods are Go functions.
//
//   - GoWidgetsTarget (NSObject) is every control's target and every
//     delegate and data source: one object, the way a small AppKit app often
//     has one controller. Which widget spoke is looked up from the sender.
//   - GoWidgetsView (NSView) is the content view, flipped so that its
//     origin is the top left corner, like core's coordinates.
//   - GoWidgetsWindow (NSWindow) sees every event of the window in
//     sendEvent:, which is where the keys are read (see keys_darwin.go).
func registerClasses() error {
	type (
		id  = objc.ID
		cmd = objc.SEL
	)
	_, err := objc.RegisterClass("GoWidgetsTarget", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
		{Cmd: sel("fire:"), Fn: func(self id, _ cmd, sender id) { onAction(sender) }},
		{Cmd: sel("open:"), Fn: func(self id, _ cmd, sender id) { onOpen(sender) }},
		{Cmd: sel("trayFire:"), Fn: func(self id, _ cmd, sender id) { onTray(sender) }},
		{Cmd: sel("menuFire:"), Fn: func(self id, _ cmd, sender id) { onMenuItem(sender) }},
		{Cmd: sel("windowShouldClose:"), Fn: func(self id, _ cmd, window id) bool {
			// Never close on our own: core decides and calls Close.
			emit(core.BackendEvent{Kind: core.EventCloseRequested})
			return false
		}},
		{Cmd: sel("windowDidResize:"), Fn: func(self id, _ cmd, note id) {
			if d := current; d != nil && d.win != nil {
				emit(core.BackendEvent{Kind: core.EventResized, Size: d.win.contentSize()})
			}
		}},
		{Cmd: sel("controlTextDidChange:"), Fn: func(self id, _ cmd, note id) { onTextChanged(msg(note, "object")) }},
		{Cmd: sel("numberOfRowsInTableView:"), Fn: func(self id, _ cmd, table id) int {
			if n := nodeOf(table); n != nil {
				return len(n.items)
			}
			return 0
		}},
		{Cmd: sel("tableView:objectValueForTableColumn:row:"), Fn: func(self id, _ cmd, table, column id, row int) id {
			if n := nodeOf(table); n != nil && row >= 0 && row < len(n.items) {
				return nsString(n.items[row])
			}
			return 0
		}},
		{Cmd: sel("tableViewSelectionDidChange:"), Fn: func(self id, _ cmd, note id) { onRowSelected(msg(note, "object")) }},
	})
	if err != nil {
		return err
	}
	_, err = objc.RegisterClass("GoWidgetsView", objc.GetClass("NSView"), nil, nil, []objc.MethodDef{
		{Cmd: sel("isFlipped"), Fn: func(self id, _ cmd) bool { return true }},
	})
	if err != nil {
		return err
	}
	windowClass, err = objc.RegisterClass("GoWidgetsWindow", objc.GetClass("NSWindow"), nil, nil, []objc.MethodDef{
		{Cmd: sel("sendEvent:"), Fn: func(self id, _ cmd, ev id) {
			if onWindowEvent(self, ev) {
				return
			}
			superSend(self, windowClass, "sendEvent:", ev)
		}},
	})
	return err
}

// windowClass is GoWidgetsWindow.
var windowClass objc.Class

// objcSuper is struct objc_super.
type objcSuper struct {
	receiver objc.ID
	class    objc.Class // the class whose superclass implements the method
}

var msgSendSuper2 func(s *objcSuper, sel objc.SEL, arg objc.ID)

// superSend is [super name:arg] from a method of class. It names the class
// itself rather than asking the object: AppKit gives windows dynamic
// subclasses of their own (key-value observing does), and "the superclass
// of the object's class" is then our own class again — the override called
// itself until the main thread's stack ran out (seen on the first CI run).
func superSend(self objc.ID, class objc.Class, name string, arg objc.ID) {
	msgSendSuper2(&objcSuper{receiver: self, class: class}, sel(name), arg)
}

// lookup finds the widget a view belongs to.
func lookup(view objc.ID) (core.Handle, *node) {
	d := current
	if d == nil || d.win == nil {
		return 0, nil
	}
	h, ok := d.win.byView[view]
	if !ok {
		return 0, nil
	}
	return h, d.win.nodes[h]
}

func nodeOf(view objc.ID) *node {
	_, n := lookup(view)
	return n
}

// onAction is a control's action: a click, a toggle, Enter in a field, a
// pick from a pop-up or a combo box's list.
func onAction(sender objc.ID) {
	h, n := lookup(sender)
	if n == nil {
		return
	}
	switch n.kind {
	case core.KindButton:
		emit(core.BackendEvent{Kind: core.EventClicked, H: h})
	case core.KindCheckBox:
		on := int64(msg(sender, "state")) == controlStateOn
		emit(core.BackendEvent{Kind: core.EventToggled, H: h, Bool: on})
	case core.KindEdit:
		emit(core.BackendEvent{Kind: core.EventActivated, H: h, Text: goString(msg(sender, "stringValue"))})
	case core.KindComboBox:
		i := int(int64(msg(sender, "indexOfSelectedItem")))
		if current.win.isPopUp(n) {
			if i >= 0 {
				emit(core.BackendEvent{Kind: core.EventSelected, H: h, Int: i, Text: current.win.itemText(h, i)})
			}
			return
		}
		// NSComboBox sends its action for a pick from the list and for
		// Enter alike; a pick is when the field shows the chosen item.
		text := goString(msg(sender, "stringValue"))
		if i >= 0 && i < len(n.items) && n.items[i] == text {
			emit(core.BackendEvent{Kind: core.EventSelected, H: h, Int: i, Text: text})
		}
	}
}

// onTextChanged is typing in a text field or a combo box's field.
func onTextChanged(field objc.ID) {
	h, n := lookup(field)
	if n == nil {
		return
	}
	emit(core.BackendEvent{Kind: core.EventTextChanged, H: h, Text: goString(msg(field, "stringValue"))})
}

// onRowSelected reports a list's selection. It also fires for the
// program's own selectRowIndexes:, like GTK's row-selected; core drops what
// it set itself by value.
func onRowSelected(table objc.ID) {
	h, n := lookup(table)
	if n == nil {
		return
	}
	i := int(int64(msg(table, "selectedRow")))
	emit(core.BackendEvent{Kind: core.EventSelected, H: h, Int: i, Text: current.win.itemText(h, i)})
}

// onOpen is a list's double action; Enter comes through onWindowEvent.
func onOpen(table objc.ID) {
	h, n := lookup(table)
	if n == nil {
		return
	}
	i := int(int64(msg(table, "clickedRow")))
	if i < 0 {
		i = int(int64(msg(table, "selectedRow")))
	}
	if i < 0 {
		return
	}
	emit(core.BackendEvent{Kind: core.EventItemActivated, H: h, Int: i, Text: current.win.itemText(h, i)})
}
