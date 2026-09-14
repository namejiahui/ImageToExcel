package processor

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/allbackends"

	"github.com/namejiahui/ImageToExcel/xlsxwriter"
)

const quantizeShaderWGSL = `
@group(0) @binding(0) var<storage, read> inputPixels: array<u32>;
@group(0) @binding(1) var<storage, read_write> outputColorIndex: array<u32>;
@group(0) @binding(2) var<storage, read_write> presenceBitset: array<atomic<u32>, 1024>;

struct Params {
    numPixels: u32,
}
@group(0) @binding(3) var<uniform> params: Params;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let idx = id.x;
    if (idx >= params.numPixels) {
        return;
    }

    let p = inputPixels[idx];
    var r = p & 0xFFu;
    var g = (p >> 8u) & 0xFFu;
    var b = (p >> 16u) & 0xFFu;
    let a = (p >> 24u) & 0xFFu;

    if (a > 0u && a < 255u) {
        r = min(255u, (r * 255u + a / 2u) / a);
        g = min(255u, (g * 255u + a / 2u) / a);
        b = min(255u, (b * 255u + a / 2u) / a);
    } else if (a == 0u) {
        r = 0u;
        g = 0u;
        b = 0u;
    }

    let colorIndex = ((r >> 3u) << 10u) | ((g >> 3u) << 5u) | (b >> 3u);

    let wordIdx = colorIndex / 32u;
    let bitIdx = colorIndex % 32u;
    atomicOr(&presenceBitset[wordIdx], 1u << bitIdx);

    outputColorIndex[idx] = colorIndex;
}
`

// processGPU orchestrates GPU initialization, compute execution, and palette decoding.
func processGPU(img image.Image) (palette []xlsxwriter.RGB, pixelStyles []int, devName string, ok bool) {
	bounds := img.Bounds()
	numPixels := bounds.Dx() * bounds.Dy()
	if numPixels == 0 {
		return nil, nil, "", false
	}

	gpu, err := initGPU()
	if err != nil {
		return nil, nil, "", false
	}
	defer gpu.release()

	inputBytes := extractRGBA(img, bounds, numPixels)
	bitsetBytes, outputBytes, err := gpu.runCompute(inputBytes, numPixels)
	if err != nil {
		return nil, nil, "", false
	}

	palette, pixelStyles = decodeResults(bitsetBytes, outputBytes, numPixels)
	return palette, pixelStyles, gpu.name, true
}

// gpuContext encapsulates WebGPU resources and adapter metadata.
type gpuContext struct {
	instance *wgpu.Instance
	adapter  *wgpu.Adapter
	device   *wgpu.Device
	name     string
}

func initGPU() (*gpuContext, error) {
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}

	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		instance.Release()
		return nil, fmt.Errorf("failed to request adapter: %w", err)
	}

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("failed to request device: %w", err)
	}

	return &gpuContext{
		instance: instance,
		adapter:  adapter,
		device:   device,
		name:     adapter.Info().Name,
	}, nil
}

func (g *gpuContext) release() {
	if g.device != nil {
		g.device.Release()
	}
	if g.adapter != nil {
		g.adapter.Release()
	}
	if g.instance != nil {
		g.instance.Release()
	}
}

// extractRGBA converts image pixels into a contiguous byte buffer of 32-bit RGBA values.
func extractRGBA(img image.Image, bounds image.Rectangle, numPixels int) []byte {
	data := make([]byte, numPixels*4)
	idx := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			data[idx] = c.R
			data[idx+1] = c.G
			data[idx+2] = c.B
			data[idx+3] = c.A
			idx += 4
		}
	}
	return data
}

// runCompute sets up buffers, dispatches the WGSL compute pass, and returns the result buffers.
func (g *gpuContext) runCompute(inputBytes []byte, numPixels int) (bitsetBytes []byte, outputBytes []byte, err error) {
	pixelBufSize := uint64(numPixels * 4)
	bitsetBufSize := uint64(1024 * 4)

	inputBuf, err := g.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "input", Size: pixelBufSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, nil, err
	}
	defer inputBuf.Release()

	outputBuf, err := g.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "output", Size: pixelBufSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		return nil, nil, err
	}
	defer outputBuf.Release()

	stagingOutputBuf, err := g.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "stagingOutput", Size: pixelBufSize,
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		return nil, nil, err
	}
	defer stagingOutputBuf.Release()

	bitsetBuf, err := g.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "bitset", Size: bitsetBufSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, nil, err
	}
	defer bitsetBuf.Release()

	stagingBitsetBuf, err := g.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "stagingBitset", Size: bitsetBufSize,
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		return nil, nil, err
	}
	defer stagingBitsetBuf.Release()

	uniformData := make([]byte, 4)
	binary.LittleEndian.PutUint32(uniformData, uint32(numPixels))
	uniformBuf, err := g.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "params", Size: 4,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, nil, err
	}
	defer uniformBuf.Release()

	queue := g.device.Queue()
	if err := queue.WriteBuffer(inputBuf, 0, inputBytes); err != nil {
		return nil, nil, err
	}
	if err := queue.WriteBuffer(uniformBuf, 0, uniformData); err != nil {
		return nil, nil, err
	}

	shader, err := g.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "quantize-shader", WGSL: quantizeShaderWGSL,
	})
	if err != nil {
		return nil, nil, err
	}
	defer shader.Release()

	bgLayout, err := g.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "quantize-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
			{Binding: 2, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
			{Binding: 3, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	defer bgLayout.Release()

	bindGroup, err := g.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label: "quantize-bg", Layout: bgLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: inputBuf, Size: pixelBufSize},
			{Binding: 1, Buffer: outputBuf, Size: pixelBufSize},
			{Binding: 2, Buffer: bitsetBuf, Size: bitsetBufSize},
			{Binding: 3, Buffer: uniformBuf, Size: 4},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	defer bindGroup.Release()

	plLayout, err := g.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label: "quantize-pl", BindGroupLayouts: []*wgpu.BindGroupLayout{bgLayout},
	})
	if err != nil {
		return nil, nil, err
	}
	defer plLayout.Release()

	pipeline, err := g.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label: "quantize-pipeline", Layout: plLayout, Module: shader, EntryPoint: "main",
	})
	if err != nil {
		return nil, nil, err
	}
	defer pipeline.Release()

	encoder, err := g.device.CreateCommandEncoder(nil)
	if err != nil {
		return nil, nil, err
	}
	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		return nil, nil, err
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bindGroup, nil)
	pass.Dispatch(uint32((numPixels+63)/64), 1, 1)
	if err := pass.End(); err != nil {
		return nil, nil, err
	}

	encoder.CopyBufferToBuffer(outputBuf, 0, stagingOutputBuf, 0, pixelBufSize)
	encoder.CopyBufferToBuffer(bitsetBuf, 0, stagingBitsetBuf, 0, bitsetBufSize)
	cmdBuf, err := encoder.Finish()
	if err != nil {
		return nil, nil, err
	}

	if _, err := queue.Submit(cmdBuf); err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Read staging bitset buffer
	if err := stagingBitsetBuf.Map(ctx, wgpu.MapModeRead, 0, bitsetBufSize); err != nil {
		return nil, nil, err
	}
	bitsetRng, err := stagingBitsetBuf.MappedRange(0, bitsetBufSize)
	if err != nil {
		_ = stagingBitsetBuf.Unmap()
		return nil, nil, err
	}
	bitsetCopy := make([]byte, bitsetBufSize)
	copy(bitsetCopy, bitsetRng.Bytes())
	_ = stagingBitsetBuf.Unmap()

	// Read staging output buffer
	if err := stagingOutputBuf.Map(ctx, wgpu.MapModeRead, 0, pixelBufSize); err != nil {
		return nil, nil, err
	}
	outputRng, err := stagingOutputBuf.MappedRange(0, pixelBufSize)
	if err != nil {
		_ = stagingOutputBuf.Unmap()
		return nil, nil, err
	}
	outputCopy := make([]byte, pixelBufSize)
	copy(outputCopy, outputRng.Bytes())
	_ = stagingOutputBuf.Unmap()

	return bitsetCopy, outputCopy, nil
}

// decodeResults parses the 4 KB bitset into an RGB palette and maps pixels to style IDs.
func decodeResults(bitsetBytes, outputBytes []byte, numPixels int) (palette []xlsxwriter.RGB, pixelStyles []int) {
	palette = make([]xlsxwriter.RGB, 0, 1024)
	colorIndexToStyleID := make([]int, 32768)
	for i := range colorIndexToStyleID {
		colorIndexToStyleID[i] = -1
	}

	for wordIdx := range 1024 {
		word := binary.LittleEndian.Uint32(bitsetBytes[wordIdx*4:])
		if word == 0 {
			continue
		}
		for bitIdx := range 32 {
			if (word & (1 << bitIdx)) != 0 {
				colorIndex := wordIdx*32 + bitIdx
				r5 := uint8((colorIndex >> 10) & 0x1F)
				g5 := uint8((colorIndex >> 5) & 0x1F)
				b5 := uint8(colorIndex & 0x1F)

				// Bit replication for full [0, 255] dynamic range
				rgb := xlsxwriter.RGB{
					R: (r5 << 3) | (r5 >> 2),
					G: (g5 << 3) | (g5 >> 2),
					B: (b5 << 3) | (b5 >> 2),
				}
				palette = append(palette, rgb)
				colorIndexToStyleID[colorIndex] = len(palette)
			}
		}
	}

	pixelStyles = make([]int, numPixels)
	for i := range numPixels {
		colorIndex := binary.LittleEndian.Uint32(outputBytes[i*4:])
		pixelStyles[i] = colorIndexToStyleID[colorIndex]
	}

	return palette, pixelStyles
}
