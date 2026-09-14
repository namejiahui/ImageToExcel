package processor

import (
	"image"
	"unique"

	"github.com/namejiahui/ImageToExcel/xlsxwriter"
)

// quantize reduces an 8-bit channel to 5 bits (32 levels per channel, RGB555).
// This mathematically bounds the total possible colors to 32,768 (32^3),
// strictly avoiding Microsoft Excel's hard limit of 64,000 unique cell styles.
// Bit replication (v >> 5) maps 0x00->0x00 and 0xF8->0xFF, preserving full [0, 255] range.
func quantize(v uint8) uint8 {
	return (v & 0xF8) | (v >> 5)
}

func getPixelColor(img image.Image, x, y int) unique.Handle[xlsxwriter.RGB] {
	r, g, b, a := img.At(x, y).RGBA()
	if a > 0 {
		r = r * 0xffff / a
		g = g * 0xffff / a
		b = b * 0xffff / a
	}
	return unique.Make(xlsxwriter.RGB{
		R: quantize(uint8(r >> 8)),
		G: quantize(uint8(g >> 8)),
		B: quantize(uint8(b >> 8)),
	})
}

// processCPU performs single-pass RGB555 quantization, color deduplication, and pixel style mapping.
func processCPU(img image.Image) ([]xlsxwriter.RGB, []int) {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	numPixels := width * height

	pixelStyles := make([]int, numPixels)
	styles := make(map[unique.Handle[xlsxwriter.RGB]]int)
	var palette []xlsxwriter.RGB

	idx := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := getPixelColor(img, x, y)
			sID, exists := styles[c]
			if !exists {
				palette = append(palette, c.Value())
				sID = len(palette) // 1-based style ID matching cellXfs (style 0 is default empty)
				styles[c] = sID
			}
			pixelStyles[idx] = sID
			idx++
		}
	}

	return palette, pixelStyles
}
