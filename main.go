package main

import (
	"cmp"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"time"

	"github.com/namejiahui/ImageToExcel/processor"
	"github.com/namejiahui/ImageToExcel/xlsxwriter"
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

	palette, pixelStyles := processor.Process(img)

	if err := exportExcel(outPath, width, height, palette, pixelStyles); err != nil {
		log.Fatalf("failed to export excel: %v", err)
	}

	log.Printf("Execution time: %s\n", time.Since(start))
}

func exportExcel(outPath string, width, height int, palette []xlsxwriter.RGB, pixelStyles []int) error {
	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	w, err := xlsxwriter.New(outFile, width, height, palette)
	if err != nil {
		return fmt.Errorf("failed to create xlsx writer: %w", err)
	}

	row := make([]int, width)
	for y := range height {
		rowStart := y * width
		copy(row, pixelStyles[rowStart:rowStart+width])
		if err := w.WriteRow(y+1, row); err != nil {
			return fmt.Errorf("failed to write row %d: %w", y+1, err)
		}
	}

	return w.Close()
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
