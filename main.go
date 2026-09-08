package main

import (
	"cmp"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"iter"
	"log"
	"os"
	"time"
	"unique"

	"i2e/xlsxwriter"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: i2e <input-image> [output.xlsx]")
		os.Exit(1)
	}

	start := time.Now()
	imgPath := os.Args[1]
	outPath := "output.xlsx"
	if len(os.Args) > 2 {
		outPath = cmp.Or(os.Args[2], outPath)
	}

	img := loadImage(imgPath)
	width, height := img.Bounds().Dx(), img.Bounds().Dy()

	styles, palette := buildPalette(img)

	outFile, err := os.Create(outPath)
	if err != nil {
		log.Fatalf("failed to create output file: %v", err)
	}
	defer outFile.Close()

	w, err := xlsxwriter.New(outFile, width, height, palette)
	if err != nil {
		log.Fatalf("failed to create xlsx writer: %v", err)
	}

	rowStyleIDs := make([]int, width)
	for y, row := range ImageRows(img) {
		for x, c := range row {
			rowStyleIDs[x] = styles[c]
		}
		if err := w.WriteRow(y+1, rowStyleIDs); err != nil {
			log.Fatalf("failed to write row: %v", err)
		}
	}

	if err := w.Close(); err != nil {
		log.Fatalf("failed to close xlsx writer: %v", err)
	}

	log.Printf("Execution time: %s\n", time.Since(start))
}

func buildPalette(img image.Image) (map[unique.Handle[xlsxwriter.RGB]]int, []xlsxwriter.RGB) {
	styles := make(map[unique.Handle[xlsxwriter.RGB]]int)
	var palette []xlsxwriter.RGB

	for _, row := range ImageRows(img) {
		for _, c := range row {
			if _, exists := styles[c]; !exists {
				palette = append(palette, c.Value())
				styles[c] = len(palette)
			}
		}
	}
	return styles, palette
}

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

func ImageRows(img image.Image) iter.Seq2[int, []unique.Handle[xlsxwriter.RGB]] {
	return func(yield func(int, []unique.Handle[xlsxwriter.RGB]) bool) {
		bounds := img.Bounds()
		width := bounds.Dx()
		row := make([]unique.Handle[xlsxwriter.RGB], width)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				row[x-bounds.Min.X] = getPixelColor(img, x, y)
			}
			if !yield(y-bounds.Min.Y, row) {
				return
			}
		}
	}
}

func loadImage(imgPath string) image.Image {
	imgFile, err := os.Open(imgPath)
	if err != nil {
		log.Fatalf("failed to open image: %v", err)
	}
	defer imgFile.Close()
	img, _, err := image.Decode(imgFile)
	if err != nil {
		log.Fatalf("failed to decode image: %v", err)
	}
	return img
}
