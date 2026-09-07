package main

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"iter"
	"log"
	"os"
	"sync"
	"time"
	"unique"

	"github.com/xuri/excelize/v2"
)

type RGB struct {
	R, G, B uint8
}

type colorRequest struct {
	color    unique.Handle[RGB]
	response chan int
}

func main() {
	start := time.Now()
	image := loadImage(os.Args[1])
	f := excelize.NewFile()
	maxX := image.Bounds().Dx()
	colorChan := processBackgroundColor(f)
	wg := &sync.WaitGroup{}

	endCol, _ := excelize.ColumnNumberToName(maxX)
	f.SetColWidth("Sheet1", "A", endCol, 0.01)

	if err := f.SetSheetProps("Sheet1", &excelize.SheetPropsOptions{
		DefaultRowHeight: new(1.0),
		CustomHeight:     new(true),
	}); err != nil {
		log.Fatalf("failed to set sheet props: %v", err)
	}

	for y, row := range ImageRows(image) {
		for x, colorHandle := range row {
			wg.Add(1)
			go func() {
				defer wg.Done()
				responseChan := make(chan int)
				colorChan <- colorRequest{color: colorHandle, response: responseChan}
				styleID := <-responseChan
				cell, err := excelize.CoordinatesToCellName(x+1, y+1)
				if err != nil {
					log.Fatalf("failed to get CellName: %v", err)
				}
				f.SetCellStyle("Sheet1", cell, cell, styleID)
			}()
		}
	}
	wg.Wait()
	f.SaveAs("output.xlsx")
	log.Printf("Execution time: %s\n", time.Since(start))
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

func getPixelColor(img image.Image, x, y int) unique.Handle[RGB] {
	r, g, b, a := img.At(x, y).RGBA()
	if a > 0 {
		r = r * 0xffff / a
		g = g * 0xffff / a
		b = b * 0xffff / a
	}
	return unique.Make(RGB{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
	})
}

func processBackgroundColor(f *excelize.File) chan colorRequest {
	styles := make(map[unique.Handle[RGB]]int)
	colorChan := make(chan colorRequest)
	go func() {
		for req := range colorChan {
			styleID, exists := styles[req.color]
			if !exists {
				c := req.color.Value()
				hexColor := fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
				style := &excelize.Style{
					Fill: excelize.Fill{
						Type:    "pattern",
						Pattern: 1,
						Color:   []string{hexColor},
					},
				}
				var err error
				styleID, err = f.NewStyle(style)
				if err != nil {
					log.Fatalf("failed to NewStyle: %v", err)
				}
				styles[req.color] = styleID
			}
			req.response <- styleID
		}
	}()
	return colorChan
}

// ImageRows returns an iterator yielding (y, rowColors) for each row of the image.
func ImageRows(img image.Image) iter.Seq2[int, []unique.Handle[RGB]] {
	return func(yield func(int, []unique.Handle[RGB]) bool) {
		bounds := img.Bounds()
		width := bounds.Dx()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			row := make([]unique.Handle[RGB], width)
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				row[x-bounds.Min.X] = getPixelColor(img, x, y)
			}
			if !yield(y-bounds.Min.Y, row) {
				return
			}
		}
	}
}
