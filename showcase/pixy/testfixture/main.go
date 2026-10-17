// Command testfixture creates a deterministic image and seven colored siblings
// for CI screenshots of folder browsing and the thumbnail grid.
package main

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) == 4 && os.Args[1] == "-verify-save" {
		verifySave(os.Args[2], os.Args[3])
		return
	}
	if len(os.Args) != 2 {
		log.Fatal("usage: testfixture output.png | testfixture -verify-save input.png output.jpg")
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
	ext := filepath.Ext(os.Args[1])
	stem := os.Args[1][:len(os.Args[1])-len(ext)]
	for n := 2; n <= 8; n++ {
		variant := image.NewRGBA(img.Bounds())
		copy(variant.Pix, img.Pix)
		tint := uint8(n * 19)
		for i := 0; i < len(variant.Pix); i += 4 {
			variant.Pix[i] = variant.Pix[i] + tint
			variant.Pix[i+2] = variant.Pix[i+2] - tint
		}
		path := fmt.Sprintf("%s-%02d%s", stem, n, ext)
		out, err := os.Create(path)
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(out, variant); err != nil {
			out.Close()
			log.Fatal(err)
		}
		if err := out.Close(); err != nil {
			log.Fatal(err)
		}
	}
}

func verifySave(inputPath, outputPath string) {
	input, err := os.Open(inputPath)
	if err != nil {
		log.Fatal(err)
	}
	inputImage, _, err := image.Decode(input)
	if closeErr := input.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		log.Fatal(err)
	}

	output, err := os.Open(outputPath)
	if err != nil {
		log.Fatal(err)
	}
	outputImage, format, err := image.Decode(output)
	if closeErr := output.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		log.Fatal(err)
	}
	if format != "jpeg" {
		log.Fatalf("saved image format = %q, want jpeg", format)
	}
	inputBounds, outputBounds := inputImage.Bounds(), outputImage.Bounds()
	if outputBounds.Empty() || outputBounds.Size() == inputBounds.Size() {
		log.Fatalf("saved image bounds = %v, want a non-empty edited size different from input %v", outputBounds, inputBounds)
	}
	fmt.Printf("decoded edited JPEG: %s (%dx%d; input %dx%d)\n", outputPath, outputBounds.Dx(), outputBounds.Dy(), inputBounds.Dx(), inputBounds.Dy())
}
