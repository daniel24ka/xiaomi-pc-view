package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Picture enhancement while zoomed in: a light temporal denoise and a sharpen on the frame
// before the digital zoom crops it. Measured on a C200 at 4x (the facade of a building):
// edge sharpness 22.5 -> 43.2 with the noise unchanged (0.60 -> 0.64); a sharpen alone
// doubled the noise. The sharpen runs on the GPU (OpenCL) when ffmpeg can: 52 fps at
// 1920x1080; the CPU fallback (CAS) reaches ~24 fps with 4 filter threads.
//
// The GPU also decodes and scales: without the zoom, Vulkan decodes the stream and
// libplacebo scales it (EWA Lanczos) and converts it to RGBA, so the CPU only copies the
// frames (1080p HEVC at 20 fps on a C200: 0.27 -> 0.08 of a core). While zoomed, CUDA
// decodes (the OpenCL sharpen can't take Vulkan frames): 0.41 -> 0.30. Each is checked once
// with a tiny test clip and turned off when it fails in real use (gpuError).

const (
	denoiseFilter    = "hqdn3d=2:1:6:6"
	gpuSharpenFilter = "format=yuv420p,hwupload,unsharp_opencl=lx=5:ly=5:la=0.7,hwdownload,format=yuv420p"
	cpuSharpenFilter = "cas=strength=0.5"
)

// gpuFeature - a GPU path of ffmpeg, checked once by running probe.
type gpuFeature struct {
	once  sync.Once
	ok    atomic.Bool
	probe []string
}

func (g *gpuFeature) available() bool {
	g.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := append([]string{"-hide_banner", "-loglevel", "error"}, g.probe...)
		g.ok.Store(toolCommand(ctx, "ffmpeg", append(args, "-f", "null", "-")...).Run() == nil)
	})
	return g.ok.Load()
}

func (g *gpuFeature) failed() { g.ok.Store(false) }

var (
	openclSharpen = gpuFeature{probe: []string{"-init_hw_device", "opencl=gpu:0.0", "-filter_hw_device", "gpu",
		"-f", "lavfi", "-i", "testsrc2=s=320x240", "-frames:v", "2", "-vf", gpuSharpenFilter}}
	vulkanScale = gpuFeature{probe: []string{"-init_hw_device", "vulkan=vk", "-filter_hw_device", "vk",
		"-f", "lavfi", "-i", "testsrc2=s=320x240", "-frames:v", "2", "-vf", placeboFilter(160, 120)}}
	cudaDecode = gpuFeature{probe: []string{"-init_hw_device", "cuda=cu", "-f", "lavfi", "-i", "nullsrc=s=64x64", "-frames:v", "1"}}
)

// placeboFilter scales and fits the picture into w x h (black bars) on the GPU and gives RGBA.
func placeboFilter(w, h int) string {
	return fmt.Sprintf("libplacebo=w=%d:h=%d:fit_mode=contain:fillcolor=black:format=rgba:downscaler=ewa_lanczos,"+
		"hwdownload,format=rgba", w, h)
}

// videoArgs returns the ffmpeg options before the input and the video filter of a player
// decoding to w x h; gpu - the GPU paths used (a GPU error turns them off: gpuError).
func videoArgs(w, h int, enhance bool) (input []string, filter string, gpu []*gpuFeature) {
	// the picture is fitted in w x h (IP cameras: the size may not be known yet)
	fit := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2", w, h, w, h)
	if !enhance {
		if vulkanScale.available() {
			return []string{"-init_hw_device", "vulkan=vk", "-filter_hw_device", "vk",
				"-hwaccel", "vulkan", "-hwaccel_output_format", "vulkan"}, placeboFilter(w, h), []*gpuFeature{&vulkanScale}
		}
		return nil, fit, nil
	}
	if cudaDecode.available() {
		input, gpu = []string{"-hwaccel", "cuda"}, []*gpuFeature{&cudaDecode}
	}
	if openclSharpen.available() {
		input = append(input, "-init_hw_device", "opencl=gpu:0.0", "-filter_hw_device", "gpu")
		return input, fit + "," + denoiseFilter + "," + gpuSharpenFilter, append(gpu, &openclSharpen)
	}
	return append(input, "-filter_threads", "4"), fit + "," + denoiseFilter + "," + cpuSharpenFilter, gpu
}

// gpuError reports whether ffmpeg's error is about the GPU (not the camera or the network,
// which also end a playback without a picture).
func gpuError(stderr string) bool {
	s := strings.ToLower(stderr)
	for _, w := range []string{"vulkan", "placebo", "opencl", "cuda", "hwaccel", "hwupload", "hwdownload", "device"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
