package main

import (
	"testing"

	"github.com/unxed/goWidgets/showcase/zipgui/engine"
)

func TestHumanSize(t *testing.T) {
	for _, tc := range []struct {
		n    int64
		want string
	}{
		{0, "0 Б"},
		{1023, "1023 Б"},
		{1024, "1.0 КБ"},
		{1536, "1.5 КБ"},
		{5 << 20, "5.0 МБ"},
		{3 << 30, "3.0 ГБ"},
		{2 << 40, "2.0 ТБ"},
		{2048 << 40, "2048.0 ТБ"},
	} {
		if got := humanSize(tc.n); got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestDescribe(t *testing.T) {
	for _, tc := range []struct {
		s    engine.Summary
		want string
	}{
		{engine.Summary{Files: 2, Dirs: 1, Size: 11, Packed: -1}, "a.zip — файлов: 2, папок: 1, 11 Б"},
		{engine.Summary{Files: 2, Dirs: 1, Size: 2048, Packed: 512}, "a.zip — файлов: 2, папок: 1, 2.0 КБ, сжато: 512 Б (25 %)"},
		{engine.Summary{Dirs: 1}, "a.zip — файлов: 0, папок: 1, 0 Б"},
	} {
		if got := describe("a.zip", tc.s); got != tc.want {
			t.Errorf("describe(%+v) = %q, want %q", tc.s, got, tc.want)
		}
	}
}
