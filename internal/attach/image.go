package attach

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	_ "golang.org/x/image/webp"
)

// processImage decodes an image attachment, shrinks it so its long edge is at
// most maxEdge pixels, and re-encodes it. PNG keeps its MIME and alpha; jpeg
// stays jpeg; webp and gif are re-encoded as PNG (first frame for gif).
func processImage(data []byte, mime string, maxEdge int) (string, []byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", nil, fmt.Errorf("attach: decode image: %w", err)
	}
	thumb := resizeDown(src, maxEdge)

	outMIME := mime
	if mime == "image/jpg" {
		outMIME = "image/jpeg"
	}
	if mime == "image/webp" || mime == "image/gif" {
		outMIME = "image/png"
	}

	var buf bytes.Buffer
	switch outMIME {
	case "image/png":
		if err := png.Encode(&buf, thumb); err != nil {
			return "", nil, err
		}
	case "image/jpeg":
		// JPEG has no alpha channel; flatten onto white.
		rgba := image.NewRGBA(thumb.Bounds())
		draw.Draw(rgba, rgba.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		draw.Draw(rgba, rgba.Bounds(), thumb, thumb.Bounds().Min, draw.Over)
		if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: 85}); err != nil {
			return "", nil, err
		}
	default:
		return "", nil, fmt.Errorf("%w: image output %s", ErrUnsupported, outMIME)
	}
	return outMIME, buf.Bytes(), nil
}

// resizeDown returns an image whose long edge is at most maxEdge. If src is
// already within the limit it is returned unchanged. The resampler is a
// box average, sufficient for thumbnail generation and dependency free.
func resizeDown(src image.Image, maxEdge int) image.Image {
	if maxEdge <= 0 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	long := w
	if h > long {
		long = h
	}
	if long <= maxEdge {
		return src
	}
	scale := float64(maxEdge) / float64(long)
	nw := max(1, int(float64(w)*scale))
	nh := max(1, int(float64(h)*scale))
	return boxResize(src, b, nw, nh)
}

// boxResize performs an area-average downscale from src bounds b to nw×nh.
func boxResize(src image.Image, b image.Rectangle, nw, nh int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xscale := float64(b.Dx()) / float64(nw)
	yscale := float64(b.Dy()) / float64(nh)
	for dy := 0; dy < nh; dy++ {
		y0 := b.Min.Y + int(float64(dy)*yscale)
		y1 := b.Min.Y + int(float64(dy+1)*yscale)
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for dx := 0; dx < nw; dx++ {
			x0 := b.Min.X + int(float64(dx)*xscale)
			x1 := b.Min.X + int(float64(dx+1)*xscale)
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a uint64
			var n uint64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					cr, cg, cb, ca := src.At(x, y).RGBA()
					r += uint64(cr)
					g += uint64(cg)
					bl += uint64(cb)
					a += uint64(ca)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			dst.SetRGBA(dx, dy, color.RGBA{
				R: uint8(r / n >> 8),
				G: uint8(g / n >> 8),
				B: uint8(bl / n >> 8),
				A: uint8(a / n >> 8),
			})
		}
	}
	return dst
}
