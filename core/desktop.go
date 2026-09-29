package core

import "strings"

// A Linux machine often has both toolkits installed, and which one looks
// native depends on the desktop, not on the distribution's package list:
// Kubuntu ships GTK too, but its applications are Qt. The desktop says so
// in XDG_CURRENT_DESKTOP (a colon-separated list, "KDE" on Plasma); older
// sessions only set KDE_FULL_SESSION or DESKTOP_SESSION.

// qtDesktops are the desktop environments built on Qt, as they name
// themselves in XDG_CURRENT_DESKTOP, upper-cased.
var qtDesktops = []string{"KDE", "LXQT", "DDE", "DEEPIN", "UKUI", "LUMINA", "LOMIRI"}

// QtDesktop reports whether the session is a Qt-based desktop, and which.
// env is os.Getenv, or a stand-in in tests.
func QtDesktop(env func(string) string) (string, bool) {
	for _, d := range strings.Split(env("XDG_CURRENT_DESKTOP"), ":") {
		u := strings.ToUpper(strings.TrimSpace(d))
		for _, q := range qtDesktops {
			if u == q {
				return d, true
			}
		}
	}
	if env("KDE_FULL_SESSION") == "true" {
		return "KDE", true
	}
	s := strings.ToLower(env("DESKTOP_SESSION"))
	for _, name := range []string{"plasma", "kde", "lxqt"} {
		if strings.Contains(s, name) {
			return env("DESKTOP_SESSION"), true
		}
	}
	return "", false
}

// SelectionOrder is DefaultOrder adjusted for the session: on a Qt desktop
// "qt" moves ahead of "gtk", everywhere else GTK stays first. why says what
// was decided, for Diagnostics; it is empty when nothing moved.
func SelectionOrder(env func(string) string) (order []string, why string) {
	order = append([]string(nil), DefaultOrder...)
	d, ok := QtDesktop(env)
	if !ok {
		return order, ""
	}
	gi, qi := -1, -1
	for i, n := range order {
		switch n {
		case "gtk":
			gi = i
		case "qt":
			qi = i
		}
	}
	if gi < 0 || qi < 0 || qi < gi {
		return order, ""
	}
	order = append(order[:qi], order[qi+1:]...)
	order = append(order[:gi], append([]string{"qt"}, order[gi:]...)...)
	return order, "desktop " + d + ": qt before gtk"
}
