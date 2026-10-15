package export

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// noisy returns a deterministic textured image whose JPEG size depends on quality.
func noisy(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	seed := uint32(12345)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			seed = seed*1664525 + 1013904223
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(seed >> 24), G: uint8(seed >> 16), B: uint8(seed >> 8), A: 255})
		}
	}
	return img
}

func TestFormatFromPath(t *testing.T) {
	cases := map[string]Format{"a.png": PNG, "a.PNG": PNG, "dir/a.jpg": JPEG, "a.JPEG": JPEG}
	for path, want := range cases {
		got, err := FormatFromPath(path)
		if err != nil || got != want {
			t.Errorf("FormatFromPath(%q) = %v, %v; want %v", path, got, err, want)
		}
	}
	for _, path := range []string{"a.bmp", "a", "a.webp"} {
		if _, err := FormatFromPath(path); err == nil {
			t.Errorf("FormatFromPath(%q) must fail", path)
		}
	}
}

func TestEncodePNGRoundTripNonZeroOrigin(t *testing.T) {
	src := image.NewNRGBA(image.Rect(3, 4, 5, 5))
	src.SetNRGBA(3, 4, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	src.SetNRGBA(4, 4, color.NRGBA{R: 200, G: 100, B: 50, A: 128})

	var buf bytes.Buffer
	if err := Encode(&buf, src, PNG, 0); err != nil {
		t.Fatalf("Encode PNG: %v", err)
	}
	got, err := png.Decode(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Bounds().Dx() != 2 || got.Bounds().Dy() != 1 {
		t.Fatalf("decoded size = %v, want 2x1", got.Bounds())
	}
	nrgba := color.NRGBAModel
	for i := 0; i < 2; i++ {
		want := nrgba.Convert(src.At(3+i, 4))
		if have := nrgba.Convert(got.At(got.Bounds().Min.X+i, got.Bounds().Min.Y)); have != want {
			t.Errorf("pixel %d = %v, want %v", i, have, want)
		}
	}
}

func TestEncodeJPEGCompositesTransparencyOverWhite(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 16, 16)) // fully transparent
	var buf bytes.Buffer
	if err := Encode(&buf, src, JPEG, 100); err != nil {
		t.Fatalf("Encode JPEG: %v", err)
	}
	got, err := jpeg.Decode(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	r, g, b, a := got.At(8, 8).RGBA()
	if r>>8 < 250 || g>>8 < 250 || b>>8 < 250 || a != 0xffff {
		t.Errorf("transparent pixel = %d,%d,%d,%d, want opaque white", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestEncodeRejectsBadInput(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, nil, PNG, 0); err != ErrNoImage {
		t.Errorf("nil image error = %v, want ErrNoImage", err)
	}
	if err := Encode(&buf, image.NewNRGBA(image.Rectangle{}), JPEG, 90); err != ErrNoImage {
		t.Errorf("empty image error = %v, want ErrNoImage", err)
	}
	img := noisy(4, 4)
	for _, q := range []int{0, -1, 101} {
		if err := Encode(&buf, img, JPEG, q); err == nil {
			t.Errorf("JPEG quality %d must fail", q)
		}
	}
	if err := Encode(&buf, img, 0, 90); err == nil {
		t.Error("unknown format must fail")
	}
	// PNG ignores quality, even an invalid one.
	if err := Encode(&buf, img, PNG, -5); err != nil {
		t.Errorf("PNG must ignore quality: %v", err)
	}
}

func TestEncodedSizeMatchesEncodeAndGrowsWithQuality(t *testing.T) {
	img := noisy(64, 64)
	var buf bytes.Buffer
	if err := Encode(&buf, img, JPEG, 80); err != nil {
		t.Fatal(err)
	}
	size, err := EncodedSize(img, JPEG, 80)
	if err != nil || size != buf.Len() {
		t.Fatalf("EncodedSize = %d, %v; want %d", size, err, buf.Len())
	}
	variants, err := CompareJPEG(img, []int{10, 50, 95})
	if err != nil || len(variants) != 3 {
		t.Fatalf("CompareJPEG = %v, %v", variants, err)
	}
	for i, q := range []int{10, 50, 95} {
		if variants[i].Quality != q {
			t.Errorf("variant %d quality = %d, want %d", i, variants[i].Quality, q)
		}
	}
	if !(variants[0].Size < variants[1].Size && variants[1].Size < variants[2].Size) {
		t.Errorf("sizes must grow with quality, got %v", variants)
	}
	if _, err := CompareJPEG(img, []int{50, 0}); err == nil {
		t.Error("CompareJPEG must reject an invalid quality")
	}
}

func TestSaveWritesFileAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.jpg")
	if err := Save(path, noisy(32, 32), 85); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if cfg, err := jpeg.DecodeConfig(f); err != nil || cfg.Width != 32 || cfg.Height != 32 {
		t.Errorf("saved file config = %+v, %v; want 32x32 JPEG", cfg, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("directory has %d entries after Save, want only out.jpg", len(entries))
	}
}

func TestSaveFailureKeepsExistingFileAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keep.jpg")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, noisy(8, 8), 0); err == nil { // invalid JPEG quality
		t.Fatal("Save with invalid quality must fail")
	}
	if got, _ := os.ReadFile(path); string(got) != "original" {
		t.Errorf("existing file changed to %q after failed Save", got)
	}
	if err := Save(filepath.Join(dir, "x.bmp"), noisy(8, 8), 90); err == nil {
		t.Error("Save with unsupported extension must fail")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("directory has %d entries, want only keep.jpg", len(entries))
	}
}
