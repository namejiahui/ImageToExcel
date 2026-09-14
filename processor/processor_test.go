package processor

import (
	"image"
	"image/color"
	"testing"
)

// TestQuantizeAndDeduplicate verifies the fundamental business logic of the processor:
// RGB555 quantization, color deduplication, shared style IDs for identical colors,
// and strict adherence to Excel's 1-based cellXfs indexing.
func TestQuantizeAndDeduplicate(t *testing.T) {
	// Create a 2x2 test image with 3 distinct colors (one duplicate)
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	img.Set(1, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	img.Set(0, 1, color.RGBA{R: 0, G: 0, B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 0, B: 0, A: 255}) // Duplicate color

	palette, pixelStyles := processCPU(img)

	// 1. Deduplication invariant: 4 pixels, 3 distinct colors
	if len(palette) != 3 {
		t.Fatalf("expected 3 unique colors in palette, got %d", len(palette))
	}
	if len(pixelStyles) != 4 {
		t.Fatalf("expected 4 pixel styles, got %d", len(pixelStyles))
	}

	// 2. Consistency: identical pixels (0,0) and (1,1) must share the exact same style ID
	if pixelStyles[0] != pixelStyles[3] {
		t.Errorf("expected pixel (0,0) and (1,1) to share style ID, got %d vs %d", pixelStyles[0], pixelStyles[3])
	}

	// 3. Excel OpenXML boundary: style IDs must be in [1, len(palette)] (1-based index into cellXfs)
	for i, sID := range pixelStyles {
		if sID < 1 || sID > len(palette) {
			t.Errorf("pixel %d style ID %d out of bounds [1, %d]", i, sID, len(palette))
		}
	}
}

// TestProcessParity compares GPU and CPU processing results on the same image.
// If run in a headless environment without GPU (like CI), it gracefully skips using t.Skip.
func TestProcessParity(t *testing.T) {
	// Create an 8x8 gradient test image
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.Set(x, y, color.RGBA{
				R: uint8(x * 32),
				G: uint8(y * 32),
				B: uint8((x + y) * 16),
				A: 255,
			})
		}
	}

	gpuPalette, gpuStyles, devName, gpuOk := processGPU(img)
	if !gpuOk {
		t.Skip("Skipping GPU parity test: no WebGPU-compatible GPU/driver detected (e.g. headless CI environment)")
	}
	t.Logf("Running parity verification against GPU: %s", devName)

	cpuPalette, cpuStyles := processCPU(img)

	// 1. Verify total unique colors match exactly
	if len(gpuPalette) != len(cpuPalette) {
		t.Fatalf("unique color count mismatch: GPU found %d colors, CPU found %d colors",
			len(gpuPalette), len(cpuPalette))
	}

	// 2. Verify resolved RGB color matches on every single pixel
	numPixels := img.Bounds().Dx() * img.Bounds().Dy()
	for i := range numPixels {
		gpuRGB := gpuPalette[gpuStyles[i]-1]
		cpuRGB := cpuPalette[cpuStyles[i]-1]

		if gpuRGB != cpuRGB {
			t.Fatalf("pixel %d resolved color mismatch: GPU=%+v, CPU=%+v", i, gpuRGB, cpuRGB)
		}
	}
}

func createBenchImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / width),
				G: uint8((y * 255) / height),
				B: uint8(((x + y) * 255) / (width + height)),
				A: 255,
			})
		}
	}
	return img
}

func BenchmarkProcessCPU(b *testing.B) {
	img := createBenchImage(1024, 1024) // 1 Megapixel (~1,048,576 pixels)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		processCPU(img)
	}
}

func BenchmarkProcessGPU(b *testing.B) {
	img := createBenchImage(1024, 1024) // 1 Megapixel (~1,048,576 pixels)
	_, _, _, ok := processGPU(img)
	if !ok {
		b.Skip("GPU unavailable, skipping GPU benchmark")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		processGPU(img)
	}
}
