# ImageToExcel (i2e)

A fun, high-performance tool that converts an image into an Excel spreadsheet as pixel art, accelerated by WebGPU (Zero-CGO).

## Installation

```bash
go install github.com/namejiahui/ImageToExcel@latest
```

## Usage

```bash
# Via go install
ImageToExcel <input-image> [output.xlsx]

# Or build from source
go build -o i2e .
./i2e <input-image> [output.xlsx]
```

## About Quantization

- **Why?**
  The OpenXML / Microsoft Excel specification imposes a hard limit of **64,000 unique cell styles** (`cellXfs` / formatting combinations) per `.xlsx` workbook (see Microsoft official documentation: [Too many different cell formats in Excel](https://learn.microsoft.com/zh-cn/troubleshoot/microsoft-365-apps/excel/too-many-different-cell-formats-in-excel)). High-resolution photos easily contain hundreds of thousands of subtle gradient shades, which exceed this ceiling and cause Excel to fail or report corrupted files.
- **Strategy**:
  Each RGB channel is uniformly quantized to **5 bits (RGB555, 32 levels per channel)**:
  - **Upper Bound**: Total unique colors in any image are strictly capped at $32^3 = 32,768$, safely below Excel's 64,000 ceiling.
  - **Full Dynamic Range**: Bit replication (`(v & 0xF8) | (v >> 5)`) maps `0x00 -> 0x00` and `0xF8 -> 0xFF`, ensuring pure whites and blacks are preserved without darkening.

## Performance & GPU Acceleration

- **Zero-CGO WebGPU**: Powered by pure-Go WebGPU (`gogpu/wgpu`), color quantization and unique palette extraction are computed concurrently on the GPU via WGSL Compute Shaders without GCC, Clang, or external `.so` dependencies (`CGO_ENABLED=0`).
- **Automatic Fallback**: If no compatible GPU or Vulkan/Metal/DirectX driver is detected (e.g. in headless servers or CI), it seamlessly falls back to CPU processing without errors.
- **Speedup**: Achieves over **35% end-to-end speedup** on multi-megapixel images (~611ms vs ~944ms on 3.5M pixels).

## Example

*(Left: Generated Excel spreadsheet | Right: Original image)*

<img width="1331" height="615" alt="ImageToExcel Comparison" src="https://github.com/user-attachments/assets/ffd31fd1-5fb9-4460-a2fe-1ce99834358f" />


