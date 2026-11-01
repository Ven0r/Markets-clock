// genicon draws the Market Clock icon at several sizes and writes icon.ico.
// Run from the project folder: go run ./tools/genicon
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

type rgba struct{ r, g, b, a float64 }

func hex(c uint32) rgba {
	return rgba{float64(c>>16&0xFF) / 255, float64(c>>8&0xFF) / 255, float64(c&0xFF) / 255, 1}
}

func roundedRect(x, y, inset, r float64) bool {
	x0, y0, x1, y1 := inset, inset, 1-inset, 1-inset
	cx := math.Max(x0+r, math.Min(x, x1-r))
	cy := math.Max(y0+r, math.Min(y, y1-r))
	return x >= x0 && x <= x1 && y >= y0 && y <= y1 && math.Hypot(x-cx, y-cy) <= r
}

func segDist(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/(dx*dx+dy*dy)))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

func draw(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	bg, border := hex(0x1E2433), hex(0x3A4152)
	ring, hand, green := hex(0x5B8DEF), hex(0xE0E3EB), hex(0x22C38E)
	thick := 0.075
	if size <= 20 {
		thick = 0.1
	}
	const ss = 6
	cx, cy, R := 0.47, 0.53, 0.29
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var acc rgba
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/ss) / float64(size)
					c := rgba{}
					if roundedRect(x, y, 0.02, 0.22) {
						c = border
					}
					if roundedRect(x, y, 0.02+0.035, 0.19) {
						c = bg
					}
					if d := math.Abs(math.Hypot(x-cx, y-cy) - R); d <= thick/2 {
						c = ring
					}
					if segDist(x, y, cx, cy, cx, cy-R*0.68) <= thick/2 || segDist(x, y, cx, cy, cx+R*0.5, cy) <= thick/2 {
						c = hand
					}
					dd := math.Hypot(x-0.775, y-0.225)
					if dd <= 0.15 && c.a > 0 {
						c = bg
					}
					if dd <= 0.105 {
						c = green
					}
					acc.r += c.r * c.a
					acc.g += c.g * c.a
					acc.b += c.b * c.a
					acc.a += c.a
				}
			}
			n := float64(ss * ss)
			out := rgba{}
			if acc.a > 0 {
				out = rgba{acc.r / acc.a, acc.g / acc.a, acc.b / acc.a, acc.a / n}
			}
			img.SetNRGBA(px, py, color.NRGBA{uint8(out.r*255 + .5), uint8(out.g*255 + .5), uint8(out.b*255 + .5), uint8(out.a*255 + .5)})
		}
	}
	return img
}

func main() {
	sizes := []int{16, 20, 24, 32, 40, 48, 64, 256}
	var pngs [][]byte
	for _, s := range sizes {
		var b bytes.Buffer
		png.Encode(&b, draw(s))
		pngs = append(pngs, b.Bytes())
		if s == 256 {
			os.WriteFile("icon-preview.png", b.Bytes(), 0o644)
		}
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := uint8(s)
		if s >= 256 {
			dim = 0
		}
		out.Write([]byte{dim, dim, 0, 0})
		binary.Write(&out, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&out, binary.LittleEndian, [2]uint32{uint32(len(pngs[i])), uint32(offset)})
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		out.Write(p)
	}
	if err := os.WriteFile("res/icon.ico", out.Bytes(), 0o644); err != nil {
		panic(err)
	}
}
