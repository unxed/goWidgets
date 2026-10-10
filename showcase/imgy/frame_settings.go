package main

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

func parseFrameSettings(value string) (int, color.NRGBA, error) {
	fields := strings.Fields(value)
	if len(fields) != 2 {
		return 0, color.NRGBA{}, fmt.Errorf("expected width and #RRGGBB")
	}
	width, err := strconv.Atoi(fields[0])
	if err != nil || width < 1 || width > 256 {
		return 0, color.NRGBA{}, fmt.Errorf("width must be from 1 to 256")
	}
	text := fields[1]
	if len(text) != 7 || text[0] != '#' {
		return 0, color.NRGBA{}, fmt.Errorf("color must be #RRGGBB")
	}
	rgb, err := strconv.ParseUint(text[1:], 16, 24)
	if err != nil {
		return 0, color.NRGBA{}, fmt.Errorf("color must be #RRGGBB")
	}
	return width, color.NRGBA{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: 255}, nil
}
