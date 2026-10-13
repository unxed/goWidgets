package main

import (
	"image/color"
	"testing"
)

func TestParseFrameSettings(t *testing.T) {
	width, got, err := parseFrameSettings(" 12   #fFcC00 ")
	if err != nil {
		t.Fatal(err)
	}
	if width != 12 || got != (color.NRGBA{R: 255, G: 204, B: 0, A: 255}) {
		t.Fatalf("parsed frame = %d, %#v; want 12, opaque #FFCC00", width, got)
	}
}

func TestParseFrameSettingsRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "0 #000000", "257 #000000", "x #000000", "8 red", "8 #12345", "8 #12345678"} {
		if _, _, err := parseFrameSettings(value); err == nil {
			t.Errorf("%q should be rejected", value)
		}
	}
}
