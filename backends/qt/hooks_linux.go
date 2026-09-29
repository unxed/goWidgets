//go:build linux

package qt

import "github.com/ebitengine/purego"

// Test hooks. A test drives the real widgets from outside the way the GTK
// tests do with gtk_* calls, but a test cannot link Qt either, and has to
// reach the very library the driver bound — so the driver lends its symbol
// lookup and its QString plumbing. An application has no use for any of it.

// Sym returns the address of the first of the mangled names found in the Qt
// the driver bound, or 0.
func Sym(names ...string) uintptr {
	for _, n := range names {
		if a, err := purego.Dlsym(libs.widgets, n); err == nil && a != 0 {
			return a
		}
	}
	return 0
}

// Call calls fn with integer arguments and returns the integer result.
func Call(fn uintptr, args ...uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, args...)
	return r
}

// CallWithString calls fn(this, const QString &s, extra...).
func CallWithString(fn, this uintptr, s string, extra ...uintptr) uintptr {
	var r uintptr
	qstr(s, func(q *qstring) {
		var f func(this uintptr, q *qstring, a, b uintptr) uintptr
		purego.RegisterFunc(&f, fn)
		var a, b uintptr
		if len(extra) > 0 {
			a = extra[0]
		}
		if len(extra) > 1 {
			b = extra[1]
		}
		r = f(this, q, a, b)
	})
	return r
}

// CallString calls a `QString f(this) const` getter and returns the text.
func CallString(fn, this uintptr) string {
	var f func(this uintptr) qstring
	purego.RegisterFunc(&f, fn)
	return takeString(f(this))
}

// FocusWidget is QApplication::focusWidget().
func FocusWidget() uintptr { return qFocusWidget() }

// Geometry is the widget's frameGeometry() — for a child widget, its
// geometry — as x, y, width, height.
func Geometry(w uintptr) (x, y, width, height int) {
	var f func(w uintptr) qRect
	purego.RegisterFunc(&f, Sym("_ZNK7QWidget13frameGeometryEv"))
	r := f(w)
	return int(r.X1), int(r.Y1), int(r.X2 - r.X1 + 1), int(r.Y2 - r.Y1 + 1)
}
