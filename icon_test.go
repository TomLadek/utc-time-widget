//go:build windows

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testSizes = []int{16, 20, 24, 32, 40}

// Every minute of the day must fit: both rows inside their bands, nothing in
// the gap between them, and some ink in each row.
func TestRenderLayout(t *testing.T) {
	r := newIconRenderer(fontFace, fontWeight)
	defer r.close()
	white := color.NRGBA{255, 255, 255, 255}
	for _, size := range testSizes {
		rowH := rowHeight(size)
		for h := 0; h < 24; h++ {
			for m := 0; m < 60; m++ {
				img, err := r.render(size, fmt.Sprintf("%02d", h), fmt.Sprintf("%02d", m), white)
				if err != nil {
					t.Fatal(err)
				}
				top := inkBounds(img, image.Rect(0, 0, size, rowH))
				gap := inkBounds(img, image.Rect(0, rowH, size, size-rowH))
				bottom := inkBounds(img, image.Rect(0, size-rowH, size, size))
				if top.Empty() || bottom.Empty() || !gap.Empty() {
					t.Fatalf("size %d, %02d:%02d: top=%v gap=%v bottom=%v", size, h, m, top, gap, bottom)
				}
			}
		}
	}
	icon, err := createIcon(image.NewNRGBA(image.Rect(0, 0, 16, 16)))
	if err != nil || icon == 0 {
		t.Fatalf("createIcon: %v", err)
	}
	destroyIcon(icon)
}

func inkBounds(img *image.NRGBA, area image.Rectangle) image.Rectangle {
	ink := image.Rectangle{}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			if img.NRGBAAt(x, y).A > 0 {
				ink = ink.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return ink
}

// TestPreview writes enlarged previews of candidate fonts on dark and light
// taskbar colors. Run with ICON_PREVIEW_DIR set to an output directory.
func TestPreview(t *testing.T) {
	dir := os.Getenv("ICON_PREVIEW_DIR")
	if dir == "" {
		t.Skip("ICON_PREVIEW_DIR not set")
	}
	fonts := []struct {
		face   string
		weight int
	}{
		{"Segoe UI", 700},
		{"Segoe UI", 600},
		{"Segoe UI Variable Display", 600},
		{"Segoe UI Variable Display", 700},
		{"Bahnschrift", 600},
		{"Arial", 700},
	}
	times := [][2]string{{"00", "00"}, {"13", "47"}, {"23", "59"}, {"08", "11"}, {"19", "26"}}
	themes := []struct{ bg, fg color.NRGBA }{
		{color.NRGBA{0x1c, 0x1c, 0x1c, 255}, color.NRGBA{255, 255, 255, 255}},
		{color.NRGBA{0xee, 0xee, 0xee, 255}, color.NRGBA{0, 0, 0, 255}},
	}
	const scale, pad = 6, 8

	for _, f := range fonts {
		r := newIconRenderer(f.face, f.weight)
		width := pad
		for _, s := range testSizes {
			width += len(times) * (s*scale + pad)
		}
		height := pad
		for range themes {
			height += 2 * (testSizes[len(testSizes)-1]*scale + pad)
		}
		sheet := image.NewNRGBA(image.Rect(0, 0, width, height))
		y := pad
		for _, th := range themes {
			draw.Draw(sheet, image.Rect(0, y-pad/2, width, y+2*(testSizes[len(testSizes)-1]*scale+pad)-pad/2), image.NewUniform(th.bg), image.Point{}, draw.Src)
			x := pad
			for _, s := range testSizes {
				for _, tm := range times {
					img, err := r.render(s, tm[0], tm[1], th.fg)
					if err != nil {
						t.Fatal(err)
					}
					blend(sheet, img, x, y, scale)               // enlarged
					blend(sheet, img, x, y+s*scale+pad, 1)       // actual size
					blend(sheet, img, x+s+pad, y+s*scale+pad, 2) // 2x
					x += s*scale + pad
				}
			}
			y += 2 * (testSizes[len(testSizes)-1]*scale + pad)
		}
		r.close()
		name := strings.ReplaceAll(fmt.Sprintf("%s-%d.png", f.face, f.weight), " ", "_")
		out, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(out, sheet); err != nil {
			t.Fatal(err)
		}
		out.Close()
	}
}

// TestGenerateAssets regenerates assets/app-icon.png (the .exe icon) and
// docs/icon-preview.png. Run from the repository root with GENERATE_ASSETS=1.
func TestGenerateAssets(t *testing.T) {
	if os.Getenv("GENERATE_ASSETS") == "" {
		t.Skip("GENERATE_ASSETS not set")
	}
	r := newIconRenderer(fontFace, fontWeight)
	defer r.close()
	white := color.NRGBA{255, 255, 255, 255}

	// App icon: the clock face on a dark rounded square.
	const appSize, inset, radius = 256, 32, 48
	appIcon := image.NewNRGBA(image.Rect(0, 0, appSize, appSize))
	bg := color.NRGBA{0x20, 0x20, 0x20, 255}
	for y := 0; y < appSize; y++ {
		for x := 0; x < appSize; x++ {
			if insideRoundedRect(x, y, appSize, radius) {
				appIcon.SetNRGBA(x, y, bg)
			}
		}
	}
	face, err := r.render(appSize-2*inset, "12", "00", white)
	if err != nil {
		t.Fatal(err)
	}
	blend(appIcon, face, inset, inset, 1)
	writePNG(t, filepath.Join("assets", "app-icon.png"), appIcon)

	// README preview: 24 px icon (150 % scaling) on dark and light taskbars,
	// enlarged and at actual size.
	const size, scale, panelW, panelH = 24, 8, 260, 260
	preview := image.NewNRGBA(image.Rect(0, 0, 2*panelW, panelH))
	themes := []struct{ bg, fg color.NRGBA }{
		{color.NRGBA{0x1c, 0x1c, 0x1c, 255}, white},
		{color.NRGBA{0xee, 0xee, 0xee, 255}, color.NRGBA{0, 0, 0, 255}},
	}
	for i, th := range themes {
		x0 := i * panelW
		draw.Draw(preview, image.Rect(x0, 0, x0+panelW, panelH), image.NewUniform(th.bg), image.Point{}, draw.Src)
		img, err := r.render(size, "15", "14", th.fg)
		if err != nil {
			t.Fatal(err)
		}
		blend(preview, img, x0+(panelW-size*scale)/2, 16, scale)
		blend(preview, img, x0+(panelW-size)/2, 16+size*scale+(panelH-16-size*scale-size)/2, 1)
	}
	writePNG(t, filepath.Join("docs", "icon-preview.png"), preview)
}

func insideRoundedRect(x, y, size, radius int) bool {
	cx := min(max(x, radius), size-1-radius)
	cy := min(max(y, radius), size-1-radius)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= radius*radius
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// blend draws src scaled up by an integer factor (nearest neighbor) onto dst.
func blend(dst *image.NRGBA, src *image.NRGBA, x0, y0, scale int) {
	b := src.Bounds()
	for y := 0; y < b.Dy()*scale; y++ {
		for x := 0; x < b.Dx()*scale; x++ {
			s := src.NRGBAAt(x/scale, y/scale)
			d := dst.NRGBAAt(x0+x, y0+y)
			a := uint32(s.A)
			mix := func(sc, dc uint8) uint8 { return uint8((uint32(sc)*a + uint32(dc)*(255-a)) / 255) }
			dst.SetNRGBA(x0+x, y0+y, color.NRGBA{mix(s.R, d.R), mix(s.G, d.G), mix(s.B, d.B), 255})
		}
	}
}
