//go:build windows

package main

import (
	"errors"
	"image"
	"image/color"
	"unsafe"

	"golang.org/x/sys/windows"
)

// iconRenderer draws two lines of digits (hour above minute) into a square
// icon. Glyphs are rasterized by GDI with grayscale anti-aliasing and the
// resulting coverage is used as the icon's alpha channel, so the icon blends
// cleanly onto any taskbar background.
type iconRenderer struct {
	face   string
	weight int

	size int            // icon size the cached font was fitted for
	font windows.Handle // cached font, valid for size
}

func newIconRenderer(face string, weight int) *iconRenderer {
	return &iconRenderer{face: face, weight: weight}
}

func (r *iconRenderer) close() {
	deleteObject(r.font)
	r.font, r.size = 0, 0
}

// rowHeight is the height of each text band; the two bands are separated by a
// gap of at least one pixel (more on large icons).
func rowHeight(size int) int {
	return (size - max(1, size/16)) / 2
}

// render returns the icon image with top and bottom centered in their bands.
func (r *iconRenderer) render(size int, top, bottom string, fg color.NRGBA) (*image.NRGBA, error) {
	c, err := newCanvas(4*size, 2*size)
	if err != nil {
		return nil, err
	}
	defer c.close()

	rowH := rowHeight(size)
	if r.font == 0 || r.size != size {
		if err := r.fit(c, size, rowH); err != nil {
			return nil, err
		}
	}

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < len(img.Pix); i += 4 {
		// Keep the color in fully transparent pixels too, so scaling by the
		// shell never bleeds black into the glyph edges.
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = fg.R, fg.G, fg.B
	}
	for i, text := range []string{top, bottom} {
		ink := c.draw(r.font, text)
		bandTop := 0
		if i == 1 {
			bandTop = size - rowH
		}
		dx := (size-ink.Dx())/2 - ink.Min.X
		dy := bandTop + (rowH-ink.Dy())/2 - ink.Min.Y
		for y := ink.Min.Y; y < ink.Max.Y; y++ {
			for x := ink.Min.X; x < ink.Max.X; x++ {
				if a := c.coverage(x, y); a > 0 {
					img.SetNRGBA(x+dx, y+dy, color.NRGBA{fg.R, fg.G, fg.B, uint8(uint16(a) * uint16(fg.A) / 255)})
				}
			}
		}
	}
	return img, nil
}

// fit picks the largest font whose digits fit into a band of rowH pixels and
// whose widest two-digit string fits into the icon width.
func (r *iconRenderer) fit(c *canvas, size, rowH int) error {
	r.close()
	for em := size; em >= 4; em-- {
		font := createFont(r.face, r.weight, em)
		if font == 0 {
			return errors.New("CreateFont failed for " + r.face)
		}
		fits := c.draw(font, "0123456789").Dy() <= rowH
		for d := '0'; fits && d <= '9'; d++ {
			fits = c.draw(font, string([]rune{d, d})).Dx() <= size
		}
		if fits {
			r.font, r.size = font, size
			return nil
		}
		deleteObject(font)
	}
	return errors.New("no font size fits the icon")
}

func createFont(face string, weight, em int) windows.Handle {
	h, _, _ := procCreateFontW.Call(
		uintptr(-em), // negative: character height (em) rather than cell height
		0, 0, 0,
		uintptr(weight),
		0, 0, 0,
		fontCharsetDef,
		fontOutTTPrecis,
		0,
		fontAntialiased,
		0,
		uintptr(unsafe.Pointer(utf16(face))),
	)
	return windows.Handle(h)
}

// canvas is an off-screen GDI surface that text is drawn onto in white on
// black, so any channel of a pixel equals its glyph coverage.
type canvas struct {
	dc     windows.Handle
	bmp    windows.Handle
	oldBmp uintptr
	pix    []byte
	w, h   int
}

func newCanvas(w, h int) (*canvas, error) {
	dc, _, err := procCreateCompatibleDC.Call(0)
	if dc == 0 {
		return nil, err
	}
	bmp, pix, err := createDIB(w, h)
	if bmp == 0 {
		procDeleteDC.Call(dc)
		return nil, err
	}
	old, _, _ := procSelectObject.Call(dc, uintptr(bmp))
	procSetBkMode.Call(dc, bkTransparent)
	procSetTextColor.Call(dc, 0x00FFFFFF)
	return &canvas{dc: windows.Handle(dc), bmp: bmp, oldBmp: old, pix: pix, w: w, h: h}, nil
}

func (c *canvas) close() {
	procSelectObject.Call(uintptr(c.dc), c.oldBmp)
	deleteObject(c.bmp)
	procDeleteDC.Call(uintptr(c.dc))
}

// draw clears the canvas, draws text and returns the bounding box of its ink.
func (c *canvas) draw(font windows.Handle, text string) image.Rectangle {
	clear(c.pix)
	oldFont, _, _ := procSelectObject.Call(uintptr(c.dc), uintptr(font))
	s, _ := windows.UTF16FromString(text)
	pad := c.h / 8
	procTextOutW.Call(uintptr(c.dc), uintptr(pad), uintptr(pad),
		uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)-1))
	procGdiFlush.Call()
	// Deselect so the font can be deleted independently of the canvas.
	procSelectObject.Call(uintptr(c.dc), oldFont)

	ink := image.Rectangle{}
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			if c.coverage(x, y) > 0 {
				ink = ink.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return ink
}

func (c *canvas) coverage(x, y int) uint8 {
	i := (y*c.w + x) * 4
	return max(c.pix[i], c.pix[i+1], c.pix[i+2])
}

// createIcon converts an image into an HICON with a per-pixel alpha channel.
// The caller owns the returned icon and must destroy it.
func createIcon(img *image.NRGBA) (windows.Handle, error) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	colorBmp, pix, err := createDIB(w, h)
	if colorBmp == 0 {
		return 0, err
	}
	defer deleteObject(colorBmp)
	for i := 0; i < len(pix); i += 4 {
		pix[i+0] = img.Pix[i+2] // B
		pix[i+1] = img.Pix[i+1] // G
		pix[i+2] = img.Pix[i+0] // R
		pix[i+3] = img.Pix[i+3] // A
	}

	// The AND mask is ignored for 32-bit icons with alpha, but must exist.
	maskBits := make([]byte, ((w+15)/16)*2*h)
	mask, _, err := procCreateBitmap.Call(uintptr(w), uintptr(h), 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
	if mask == 0 {
		return 0, err
	}
	defer deleteObject(windows.Handle(mask))

	ii := iconInfo{isIcon: 1, mask: windows.Handle(mask), color: colorBmp}
	hicon, _, err := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	if hicon == 0 {
		return 0, err
	}
	return windows.Handle(hicon), nil
}
