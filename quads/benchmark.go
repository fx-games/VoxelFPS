package quads

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// Benchmark turns the render loop into a measurement. It throws away a warm up,
// records the wall time of every frame after it, and quits when the run is over,
// printing one report to stdout.
//
// It exists because the alternative is reading an FPS counter off a screenshot, and
// that number is the last frame's, smoothed over sixty ticks, on a window that may
// or may not have been waiting for vblank. A before/after on the sprite path needs
// a distribution, not an impression - and it needs the same distribution twice.
//
// The measurement is only as repeatable as the frame is. Nothing must move: no
// mouse, no input, no walking NPC, a camera left exactly where the map spawned it.
// The stress map is built that way on purpose.
type Benchmark struct {
	// Warmup is thrown away. Shader compiles, the first chunk meshes, sprite batch
	// buffers growing into their final size and the driver's own first-use costs all
	// land in the opening second and none of them are the thing being measured.
	Warmup   float64
	Duration float64

	frameTimes []float64
	elapsed    float64
}

// DefaultBenchmarkWarmup and DefaultBenchmarkDuration: two seconds of warm up covers
// the buffer growth visible in the startup log, and ten seconds is a few hundred
// frames even on a slow configuration - enough that the median and the 95th
// percentile mean something.
const (
	DefaultBenchmarkWarmup   = 2.0
	DefaultBenchmarkDuration = 10.0
)

func NewBenchmark(duration float64) *Benchmark {
	if duration <= 0 {
		duration = DefaultBenchmarkDuration
	}
	return &Benchmark{Warmup: DefaultBenchmarkWarmup, Duration: duration}
}

// record takes one frame's wall time and reports whether the run is over.
func (b *Benchmark) record(frameTime float64) bool {
	b.elapsed += frameTime
	if b.elapsed < b.Warmup {
		return false
	}
	b.frameTimes = append(b.frameTimes, frameTime)
	return b.elapsed >= b.Warmup+b.Duration
}

// warm is whether the warm up is over and frames are being counted.
func (b *Benchmark) warm() bool {
	return b.elapsed >= b.Warmup
}

// percentile takes the sorted frame times. Frame time is the honest axis, not FPS:
// FPS is a reciprocal, so averaging it weights the fast frames and hides exactly the
// hitches anyone cares about. The p95 here is the 95th percentile *frame time* - the
// slow tail - so lower is better in every column.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(p * float64(len(sorted)-1))
	return sorted[index]
}

// Report is the whole measurement as one block of text. Scene is a caller supplied
// line describing what was on screen, because a frame time without a triangle count
// and a draw call count is not comparable to anything.
func (b *Benchmark) Report(label, scene string) string {
	if len(b.frameTimes) == 0 {
		return "benchmark: no frames recorded"
	}
	sorted := append([]float64(nil), b.frameTimes...)
	sort.Float64s(sorted)

	total := 0.0
	for _, frameTime := range sorted {
		total += frameTime
	}
	mean := total / float64(len(sorted))
	median := percentile(sorted, 0.5)

	var out strings.Builder
	fmt.Fprintf(&out, "\n=== benchmark: %s ===\n", label)
	fmt.Fprintf(&out, "%s\n", scene)
	fmt.Fprintf(&out, "frames        %d over %.1fs (%.1fs warm up discarded)\n",
		len(sorted), b.Duration, b.Warmup)
	fmt.Fprintf(&out, "frame time ms median %.3f  mean %.3f  p95 %.3f  p99 %.3f  min %.3f  max %.3f\n",
		median*1000, mean*1000,
		percentile(sorted, 0.95)*1000, percentile(sorted, 0.99)*1000,
		sorted[0]*1000, sorted[len(sorted)-1]*1000)
	// Quoted off the median rather than the mean, so one stalled frame does not move it.
	fmt.Fprintf(&out, "fps at median %.1f\n", 1.0/median)

	// A percentile table says how bad the tail is, never whether the slow frames
	// arrive on a cadence. FPSBENCH_DUMP writes the frames in the order they were
	// drawn so that question can be answered outside the process.
	if path := os.Getenv("FPSBENCH_DUMP"); path != "" {
		var dump strings.Builder
		for i, frameTime := range b.frameTimes {
			fmt.Fprintf(&dump, "%d,%.6f\n", i, frameTime*1000)
		}
		if err := os.WriteFile(path, []byte(dump.String()), 0o644); err != nil {
			fmt.Fprintf(&out, "frame dump failed: %v\n", err)
		} else {
			fmt.Fprintf(&out, "frame times written to %s\n", path)
		}
	}
	return out.String()
}
