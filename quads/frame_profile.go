package quads

import (
	"fmt"
	"os"
	"strings"

	"github.com/memmaker/voxelfps/platform/gl"
	"github.com/memmaker/voxelfps/platform/glfw"
)

// frameProfile splits a benchmark's frame into the passes it is made of, and
// measures each one twice:
//
//   - cpu is the wall time between one mark and the next: what this process
//     spent submitting the pass, including any time the driver made it wait
//     inside a GL call.
//   - gpu is a GL_TIME_ELAPSED query around the same stretch: what the GPU spent
//     executing it. Queries are read back frames later, without waiting, so the
//     measuring does not add the stalls it is looking for.
//
// The queries are not free - a begin, an end and a readback per section per
// frame, about 1.8 ms of a 10 ms ark frame - and that lands in the cpu column.
// FRAMEPROF_CPU=1 leaves them out, for cpu numbers that are only the frame's own.
//
// glFinish at every mark was tried as the other way to see GPU time and does not
// work here: macOS holds glFinish to the display's refresh, so every frame came
// out at 16.67 ms.
//
// Sections are flat, not nested - only one TIME_ELAPSED query may be active at a
// time - so a mark ends the section before it.
type frameProfile struct {
	on, timers bool

	names []string
	index map[string]int
	cpu   []float64
	gpu   []float64
	// gpuSamples counts the queries that came back per section: some are still
	// in flight when the run ends, so the gpu mean is taken over these.
	gpuSamples []int
	frames     int
	// resetFrame is the first frame counted; queries from before it, still
	// arriving after a reset, are dropped.
	frame, resetFrame int

	current int
	started float64
	query   uint32
	pending []pendingTimer
	free    []uint32
}

type pendingTimer struct {
	section, frame int
	query          uint32
}

func (p *frameProfile) start() {
	p.on = true
	p.timers = os.Getenv("FRAMEPROF_CPU") == ""
	p.index = make(map[string]int)
	p.current = -1
}

// reset forgets everything measured so far: the benchmark's warm up is not the
// thing being measured.
func (p *frameProfile) reset() {
	for i := range p.cpu {
		p.cpu[i], p.gpu[i], p.gpuSamples[i] = 0, 0, 0
	}
	p.frames = 0
	p.resetFrame = p.frame
}

// mark ends the running section and starts the one called name.
func (p *frameProfile) mark(name string) {
	if !p.on {
		return
	}
	p.stop()
	section, known := p.index[name]
	if !known {
		section = len(p.names)
		p.index[name] = section
		p.names = append(p.names, name)
		p.cpu, p.gpu, p.gpuSamples = append(p.cpu, 0), append(p.gpu, 0), append(p.gpuSamples, 0)
	}
	p.current = section
	if p.timers {
		if n := len(p.free); n > 0 {
			p.query, p.free = p.free[n-1], p.free[:n-1]
		} else {
			gl.GenQueries(1, &p.query)
		}
		gl.BeginQuery(gl.TIME_ELAPSED, p.query)
	}
	p.started = glfw.GetTime()
}

func (p *frameProfile) stop() {
	if p.current < 0 {
		return
	}
	p.cpu[p.current] += glfw.GetTime() - p.started
	if p.timers {
		gl.EndQuery(gl.TIME_ELAPSED)
		p.pending = append(p.pending, pendingTimer{p.current, p.frame, p.query})
	}
	p.current = -1
}

// endFrame closes the last section and collects whatever queries have finished.
func (p *frameProfile) endFrame() {
	if !p.on {
		return
	}
	p.stop()
	waiting := p.pending[:0]
	for _, timer := range p.pending {
		var ready uint32
		gl.GetQueryObjectuiv(timer.query, gl.QUERY_RESULT_AVAILABLE, &ready)
		if ready == 0 {
			waiting = append(waiting, timer)
			continue
		}
		var nanoseconds uint64
		gl.GetQueryObjectui64v(timer.query, gl.QUERY_RESULT, &nanoseconds)
		if timer.frame >= p.resetFrame {
			p.gpu[timer.section] += float64(nanoseconds) / 1e9
			p.gpuSamples[timer.section]++
		}
		p.free = append(p.free, timer.query)
	}
	p.pending = waiting
	p.frame++
	p.frames++
}

// report is one row per section, in the order the frame reaches them, with the
// mean per frame of both measurements and their sums.
func (p *frameProfile) report() string {
	if !p.on || p.frames == 0 {
		return ""
	}
	var out strings.Builder
	mode := "cpu and gpu timer queries"
	if !p.timers {
		mode = "cpu only, FRAMEPROF_CPU"
	}
	fmt.Fprintf(&out, "frame profile, mean ms per frame over %d frames (%s)\n", p.frames, mode)
	fmt.Fprintf(&out, "  %-32s %8s %8s\n", "section", "cpu", "gpu")
	var cpuSum, gpuSum float64
	for i, name := range p.names {
		cpu := p.cpu[i] / float64(p.frames) * 1000
		gpu := 0.0
		if p.gpuSamples[i] > 0 {
			gpu = p.gpu[i] / float64(p.gpuSamples[i]) * 1000
		}
		cpuSum, gpuSum = cpuSum+cpu, gpuSum+gpu
		fmt.Fprintf(&out, "  %-32s %8.3f %8.3f\n", name, cpu, gpu)
	}
	fmt.Fprintf(&out, "  %-32s %8.3f %8.3f\n", "sum", cpuSum, gpuSum)
	return out.String()
}
