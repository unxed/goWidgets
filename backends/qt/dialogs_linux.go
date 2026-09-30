//go:build linux

package qt

import (
	"strings"
	"unsafe"

	"github.com/unxed/goWidgets/core"
)

var (
	qMsgInformation       func(parent uintptr, title, text *qstring, buttons, def int32) int32
	qMsgQuestion          func(parent uintptr, title, text *qstring, buttons, def int32) int32
	qGetOpenFileName      func(parent uintptr, caption, dir, filter *qstring, selected uintptr, options int32) qstring
	qGetSaveFileName      func(parent uintptr, caption, dir, filter *qstring, selected uintptr, options int32) qstring
	qGetExistingDirectory func(parent uintptr, caption, dir *qstring, options int32) qstring
	qModalWidget          func() uintptr

	qTranslatorCtor func(this unsafe.Pointer, parent uintptr)
	qTranslatorLoad func(tr unsafe.Pointer, locale *qvalue, name, prefix, dir, suffix *qstring) bool
	qInstallTransl  func(tr unsafe.Pointer) bool
	qLocaleCtor     func(l *qvalue)
	qLocaleDtor     func(l *qvalue)
)

func (r *resolver) bindDialogs() {
	const msgArgs = "EP7QWidgetRK7QStringS4_6QFlagsINS_14StandardButtonEES6_"
	r.fn(&qMsgInformation, "_ZN11QMessageBox11information"+msgArgs)
	r.fn(&qMsgQuestion, "_ZN11QMessageBox8question"+msgArgs)
	const fileArgs = "EP7QWidgetRK7QStringS4_S4_PS2_6QFlagsINS_6OptionEE"
	r.fn(&qGetOpenFileName, "_ZN11QFileDialog15getOpenFileName"+fileArgs)
	r.fn(&qGetSaveFileName, "_ZN11QFileDialog15getSaveFileName"+fileArgs)
	r.fn(&qGetExistingDirectory, "_ZN11QFileDialog20getExistingDirectoryEP7QWidgetRK7QStringS4_6QFlagsINS_6OptionEE")
	r.fn(&qModalWidget, "_ZN12QApplication17activeModalWidgetEv")

	r.fn(&qTranslatorCtor, "_ZN11QTranslatorC1EP7QObject")
	r.fn(&qTranslatorLoad, "_ZN11QTranslator4loadERK7QLocaleRK7QStringS5_S5_S5_")
	r.fn(&qInstallTransl, "_ZN16QCoreApplication17installTranslatorEP11QTranslator")
	r.fn(&qLocaleCtor, "_ZN7QLocaleC1Ev")
	r.fn(&qLocaleDtor, "_ZN7QLocaleD1Ev")
}

// withQStrings runs f with temporary QStrings for ss, in order.
func withQStrings(ss []string, f func(qs []*qstring)) {
	qs := make([]*qstring, len(ss))
	for i, s := range ss {
		qs[i] = newQString(s)
	}
	f(qs)
	for _, q := range qs {
		q.free()
	}
}

// loadTranslations installs Qt's own catalogue for the user's language, so
// the standard buttons of a message box read "Да/Нет" on a Russian desktop,
// as GTK's do through g_dgettext. A Qt application does not get this for
// free: without a QTranslator every standard button is English. QLocale()
// is the system locale, and QTranslator::load walks its UI languages the way
// Qt applications normally do. No catalogue installed — English it is.
func loadTranslations() {
	dir := takeString(qLibraryPath(lay.translations))
	tr := cxxNew(64) // sizeof(QTranslator) is 16
	qTranslatorCtor(tr, 0)
	var loc qvalue
	qLocaleCtor(&loc)
	ok := false
	withQStrings([]string{"qtbase", "_", dir, ".qm"}, func(q []*qstring) {
		ok = qTranslatorLoad(tr, &loc, q[0], q[1], q[2], q[3])
	})
	qLocaleDtor(&loc)
	if ok {
		qInstallTransl(tr)
	}
}

// QMessageBox::StandardButton values.
const (
	sbOK     = 0x00000400
	sbYes    = 0x00004000
	sbNo     = 0x00010000
	sbCancel = 0x00400000
)

// Dialog is QMessageBox's static functions: the platform's box, its icon and
// its translated buttons. exec() spins a nested loop, and our posted
// wake-ups are delivered in it, so the application's queue keeps draining.
// Escape and the title-bar cross come back as the escape button — Cancel or
// No — or as no button at all; either way not an accept.
func (w *window) Dialog(kind core.DialogKind, title, text string) core.DialogResult {
	var r int32
	withQStrings([]string{title, text}, func(q []*qstring) {
		switch kind {
		case core.DialogConfirm:
			r = qMsgQuestion(w.handle, q[0], q[1], sbOK|sbCancel, sbOK)
		case core.DialogYesNo:
			r = qMsgQuestion(w.handle, q[0], q[1], sbYes|sbNo, sbYes)
		default:
			r = qMsgInformation(w.handle, q[0], q[1], sbOK, sbOK)
		}
	})
	switch kind {
	case core.DialogConfirm:
		if r == sbOK {
			return core.DialogOK
		}
		return core.DialogCancel
	case core.DialogYesNo:
		if r == sbYes {
			return core.DialogYes
		}
		return core.DialogNo
	}
	return core.DialogOK
}

// FileDialog is QFileDialog's static functions: the desktop's own dialog
// where the platform theme provides one (KDE's, the portal's), Qt's
// otherwise. The save dialog asks before overwriting by itself.
func (w *window) FileDialog(save bool, title, suggested string, filters []core.FileFilter) (string, bool) {
	var parts []string
	for _, f := range filters {
		parts = append(parts, f.Name+" ("+strings.Join(f.Patterns, " ")+")")
	}
	var path string
	withQStrings([]string{title, suggested, strings.Join(parts, ";;")}, func(q []*qstring) {
		if save {
			path = takeString(qGetSaveFileName(w.handle, q[0], q[1], q[2], 0, 0))
		} else {
			path = takeString(qGetOpenFileName(w.handle, q[0], q[1], q[2], 0, 0))
		}
	})
	return path, path != ""
}

// FolderDialog uses QFileDialog's native existing-directory picker.
func (w *window) FolderDialog(title, initial string) (string, bool) {
	var path string
	withQStrings([]string{title, initial}, func(q []*qstring) {
		path = takeString(qGetExistingDirectory(w.handle, q[0], q[1], 1)) // ShowDirsOnly
	})
	return path, path != ""
}

// ModalWidget returns the modal dialog on screen, or 0. A test hook: a test
// answers the box through it.
func ModalWidget() uintptr { return qModalWidget() }
