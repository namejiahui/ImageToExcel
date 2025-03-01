package main

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"
)

type colorRequest struct {
	hexColor string
	response chan int
}

func main() {
	start := time.Now()
	image := loadImage(os.Args[1])
	f := excelize.NewFile()
	maxX, maxY := image.Bounds().Max.X, image.Bounds().Max.Y
	colorChan := processBackgroundColor(f)
	wg := &sync.WaitGroup{}

	for y := 1; y <= maxY; y++ { // row
		f.SetRowHeight("Sheet1", y, 1)
		for x := 1; x <= maxX; x++ { //col
			wg.Add(1)
			go func(x, y int) {
				defer wg.Done()
				endCol, _ := excelize.ColumnNumberToName(maxX)
				f.SetColWidth("Sheet1", "A", endCol, 0.01)
				hexColor := getHexColor(image, x-1, y-1)
				responseChan := make(chan int)
				colorChan <- colorRequest{hexColor: hexColor, response: responseChan}
				styleID := <-responseChan
				cell, err := excelize.CoordinatesToCellName(x, y)
				if err != nil {
					log.Fatalf("failed to get CellName: %v", err)
				}
				f.SetCellStyle("Sheet1", cell, cell, styleID)
			}(x, y)
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

func getHexColor(img image.Image, x, y int) string {
	r, g, b, a := img.At(x, y).RGBA()
	if a > 0 {
		r = r * 0xffff / a
		g = g * 0xffff / a
		b = b * 0xffff / a
	}
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

func processBackgroundColor(f *excelize.File) chan colorRequest {
	styles := make(map[string]int)
	colorChan := make(chan colorRequest)
	go func() {
		for req := range colorChan {
			styleID, exists := styles[req.hexColor]
			if !exists {
				style := &excelize.Style{
					Fill: excelize.Fill{
						Type:    "pattern",
						Pattern: 1,
						Color:   []string{req.hexColor},
					},
				}
				var err error
				styleID, err = f.NewStyle(style)
				if err != nil {
					log.Fatalf("failed to NewStyle: %v", err)
				}
				styles[req.hexColor] = styleID
			}
			req.response <- styleID
		}
	}()
	return colorChan
}
