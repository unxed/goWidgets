// Command testfixture creates a deterministic image for CI screenshots.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: testfixture output.png")
	}
	const width, height = 640, 420
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(24 + x*180/width),
				G: uint8(36 + y*150/height),
				B: uint8(180 - x*120/width),
				A: 255,
			})
		}
	}
	for y := 110; y < 310; y++ {
		for x := 170; x < 470; x++ {
			if (x/30+y/30)%2 == 0 {
				img.SetRGBA(x, y, color.RGBA{R: 248, G: 188, B: 64, A: 255})
			}
		}
	}
	f, err := os.Create(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}
