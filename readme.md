# ImageToExcel (i2e)

A fun tool that converts an image into an Excel spreadsheet as pixel art.

## Usage

```bash
# Build
go build -o i2e .

# Run
./i2e <input-image> [output.xlsx]
```

## About Quantization

- **Why?**
  The OpenXML / Microsoft Excel specification imposes a hard limit of **64,000 unique cell styles** (`cellXfs` / formatting combinations) per `.xlsx` workbook (see Microsoft official documentation: [Too many different cell formats in Excel](https://learn.microsoft.com/zh-cn/troubleshoot/microsoft-365-apps/excel/too-many-different-cell-formats-in-excel)). High-resolution photos easily contain hundreds of thousands of subtle gradient shades, which exceed this ceiling and cause Excel to fail or report corrupted files.
- **Strategy**:
  Each RGB channel is uniformly quantized to **5 bits (RGB555, 32 levels per channel)**:
  - **Upper Bound**: Total unique colors in any image are strictly capped at $32^3 = 32,768$, safely below Excel's 64,000 ceiling.
  - **Full Dynamic Range**: Bit replication (`(v & 0xF8) | (v >> 5)`) maps `0x00 -> 0x00` and `0xF8 -> 0xFF`, ensuring pure whites and blacks are preserved without darkening.

## Example


