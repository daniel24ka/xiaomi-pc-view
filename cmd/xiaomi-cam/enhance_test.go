package main

import (
	"os/exec"
	"testing"
	"time"
)

// The GPU checks run in the background; a player never waits longer than probeWait.
func TestGPUProbe(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	start := time.Now()
	probeGPU()
	for name, g := range map[string]*gpuFeature{"vulkan": vulkanScale, "cuda": cudaDecode, "opencl": openclSharpen} {
		ok := g.available()
		t.Logf("%s: %v after %v", name, ok, time.Since(start).Round(10*time.Millisecond))
	}

	// a check that doesn't end in time (here: 10 s of video read in real time)
	slow := newGPUFeature("-re", "-f", "lavfi", "-i", "nullsrc=s=64x64:d=10")
	start = time.Now()
	if slow.available() {
		t.Error("an unfinished check reported the GPU path")
	}
	if d := time.Since(start); d > probeWait+time.Second {
		t.Errorf("waited %v for an unfinished check", d)
	}
}
