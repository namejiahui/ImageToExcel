package processor

import (
	"image"
	"log"

	"github.com/namejiahui/ImageToExcel/xlsxwriter"
)

// Process converts an image into an Excel palette and pixel style ID array.
// It prioritizes hardware WebGPU compute shaders, automatically falling back to CPU if unavailable.
func Process(img image.Image) (palette []xlsxwriter.RGB, pixelStyles []int) {
	palette, pixelStyles, devName, ok := processGPU(img)
	if ok {
		log.Printf("GPU acceleration enabled: %s (%d unique colors)\n", devName, len(palette))
		return palette, pixelStyles
	}

	log.Println("Using CPU processing (GPU unavailable)")
	return processCPU(img)
}
