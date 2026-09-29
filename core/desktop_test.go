package core

import (
	"reflect"
	"testing"
)

func TestSelectionOrderFollowsTheDesktop(t *testing.T) {
	cases := []struct {
		env     map[string]string
		wantQt  bool
		comment string
	}{
		{map[string]string{"XDG_CURRENT_DESKTOP": "KDE"}, true, "Plasma, Kubuntu"},
		{map[string]string{"XDG_CURRENT_DESKTOP": "LXQt"}, true, "Lubuntu"},
		{map[string]string{"XDG_CURRENT_DESKTOP": "ubuntu:GNOME"}, false, "Ubuntu"},
		{map[string]string{"XDG_CURRENT_DESKTOP": "X-Cinnamon"}, false, "Mint"},
		{map[string]string{"XDG_CURRENT_DESKTOP": "XFCE"}, false, "Xubuntu"},
		{map[string]string{"KDE_FULL_SESSION": "true"}, true, "old KDE session"},
		{map[string]string{"DESKTOP_SESSION": "plasmawayland"}, true, "session name only"},
		{map[string]string{}, false, "nothing known: GTK first, as before"},
	}
	for _, c := range cases {
		order, why := SelectionOrder(func(k string) string { return c.env[k] })
		gi, qi := indexOf(order, "gtk"), indexOf(order, "qt")
		if gotQt := qi < gi; gotQt != c.wantQt {
			t.Errorf("%s (%v): order %v, want qt first = %v", c.comment, c.env, order, c.wantQt)
		}
		if c.wantQt == (why == "") {
			t.Errorf("%s: why = %q", c.comment, why)
		}
		if len(order) != len(DefaultOrder) {
			t.Errorf("%s: order %v lost or gained drivers", c.comment, order)
		}
	}
	// DefaultOrder itself is never touched.
	if !reflect.DeepEqual(DefaultOrder, []string{"win32", "cocoa", "web", "gtk", "qt", "ebiten", "headless"}) {
		t.Errorf("DefaultOrder mutated: %v", DefaultOrder)
	}
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}
