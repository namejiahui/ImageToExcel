package xlsxwriter

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"strconv"
)

type RGB struct {
	R, G, B uint8
}

type Writer struct {
	zipWriter *zip.Writer
	sheetBuf  *bufio.Writer
	colNames  []string
	rowBuf    []byte
}

const (
	contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>`

	rootRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

	workbookXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="Sheet1" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`

	workbookRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`
)

func ToColName(col int) string {
	var buf [4]byte
	i := len(buf)
	for col > 0 {
		col--
		i--
		buf[i] = byte('A' + col%26)
		col /= 26
	}
	return string(buf[i:])
}

func New(w io.Writer, width, height int, palette []RGB) (*Writer, error) {
	zw := zip.NewWriter(w)

	staticFiles := map[string]string{
		"[Content_Types].xml":        contentTypesXML,
		"_rels/.rels":                rootRelsXML,
		"xl/workbook.xml":            workbookXML,
		"xl/_rels/workbook.xml.rels": workbookRelsXML,
	}
	for name, content := range staticFiles {
		f, err := zw.Create(name)
		if err != nil {
			return nil, fmt.Errorf("failed to create %s in zip: %w", name, err)
		}
		if _, err := io.WriteString(f, content); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", name, err)
		}
	}

	if err := writeStyles(zw, palette); err != nil {
		return nil, fmt.Errorf("failed to write styles.xml: %w", err)
	}

	sheetFile, err := zw.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		return nil, fmt.Errorf("failed to create sheet1.xml in zip: %w", err)
	}
	sheetBuf := bufio.NewWriterSize(sheetFile, 64*1024)

	sheetHeader := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetFormatPr defaultRowHeight="1" customHeight="1"/>
  <cols>
    <col min="1" max="%d" width="0.01" customWidth="1"/>
  </cols>
  <sheetData>
`, width)
	if _, err := sheetBuf.WriteString(sheetHeader); err != nil {
		return nil, fmt.Errorf("failed to write sheet header: %w", err)
	}

	colNames := make([]string, width)
	for i := range width {
		colNames[i] = ToColName(i + 1)
	}

	return &Writer{
		zipWriter: zw,
		sheetBuf:  sheetBuf,
		colNames:  colNames,
		rowBuf:    make([]byte, 0, width*24),
	}, nil
}

func (w *Writer) WriteRow(row1Based int, styleIDs []int) error {
	b := w.rowBuf[:0]

	b = append(b, `<row r="`...)
	b = strconv.AppendInt(b, int64(row1Based), 10)
	b = append(b, `" ht="1" customHeight="1">`...)

	for x, sID := range styleIDs {
		b = append(b, `<c r="`...)
		b = append(b, w.colNames[x]...)
		b = strconv.AppendInt(b, int64(row1Based), 10)
		b = append(b, `" s="`...)
		b = strconv.AppendInt(b, int64(sID), 10)
		b = append(b, `"/>`...)
	}

	b = append(b, `</row>`...)
	w.rowBuf = b

	_, err := w.sheetBuf.Write(b)
	return err
}

func (w *Writer) Close() error {
	if _, err := w.sheetBuf.WriteString("  </sheetData>\n</worksheet>"); err != nil {
		return fmt.Errorf("failed to write sheet footer: %w", err)
	}
	if err := w.sheetBuf.Flush(); err != nil {
		return fmt.Errorf("failed to flush sheet buffer: %w", err)
	}
	return w.zipWriter.Close()
}

func writeStyles(zw *zip.Writer, palette []RGB) error {
	f, err := zw.Create("xl/styles.xml")
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(f, 32*1024)

	numFills := 2 + len(palette)
	numStyles := 1 + len(palette)

	if _, err := fmt.Fprintf(bw, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="1">
    <font><sz val="11"/><name val="Calibri"/></font>
  </fonts>
  <fills count="%d">
    <fill><patternFill patternType="none"/></fill>
    <fill><patternFill patternType="gray125"/></fill>
`, numFills); err != nil {
		return err
	}

	for _, c := range palette {
		if _, err := fmt.Fprintf(bw, `    <fill><patternFill patternType="solid"><fgColor rgb="FF%02X%02X%02X"/></patternFill></fill>
`, c.R, c.G, c.B); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(bw, `  </fills>
  <borders count="1">
    <border><left/><right/><top/><bottom/><diagonal/></border>
  </borders>
  <cellStyleXfs count="1">
    <xf numFmtId="0" fontId="0" fillId="0" borderId="0"/>
  </cellStyleXfs>
  <cellXfs count="%d">
    <xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
`, numStyles); err != nil {
		return err
	}

	for i := range palette {
		if _, err := fmt.Fprintf(bw, `    <xf numFmtId="0" fontId="0" fillId="%d" borderId="0" xfId="0" applyFill="1"/>
`, i+2); err != nil {
			return err
		}
	}

	if _, err := bw.WriteString("  </cellXfs>\n</styleSheet>"); err != nil {
		return err
	}
	return bw.Flush()
}
