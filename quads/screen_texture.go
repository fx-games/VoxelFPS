package quads

import (
	"fmt"
	"os"

	"github.com/memmaker/goray/quads/glhf"
)

// ScreenAnimation swaps one material slot of a mesh on a timer - a computer
// screen playing a flipbook of frames instead of showing one static image.
// It rides entirely on CompoundMesh.SetTexture; there is no new draw path,
// just a clock deciding which already-loaded texture is bound this frame.
type ScreenAnimation struct {
	frames       []*glhf.Texture
	textureIndex int
	frameSeconds float64
	elapsed      float64
	frame        int
	started      bool
}

// NewScreenAnimation builds a looping flipbook over frames, advancing one
// frame every frameSeconds and binding it to textureIndex - the material slot
// the model's screen face was authored with.
func NewScreenAnimation(textureIndex int, frameSeconds float64, frames []*glhf.Texture) *ScreenAnimation {
	return &ScreenAnimation{textureIndex: textureIndex, frameSeconds: frameSeconds, frames: frames}
}

// LoadScreenFrames reads a numbered sequence of images - "0.png", "1.png",
// ... - out of dir, stopping at the first missing number. Point it at a
// "screen" folder next to a piece's mesh.gltf, the same way a scenery piece
// already keeps everything it needs beside its model.
func LoadScreenFrames(dir string) ([]*glhf.Texture, error) {
	var frames []*glhf.Texture
	for i := 0; ; i++ {
		file, err := os.Open(fmt.Sprintf("%s/%d.png", dir, i))
		if err != nil {
			break
		}
		texture, texErr := glhf.NewTextureFromReader(file)
		file.Close()
		if texErr != nil {
			return nil, fmt.Errorf("screen frame %d: %w", i, texErr)
		}
		frames = append(frames, texture)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no numbered frames (0.png, 1.png, ...) found in %s", dir)
	}
	return frames, nil
}

// tick advances the clock and, whenever it crosses into a new frame - one or
// several, if elapsed was large enough to skip more than one - rebinds mesh's
// texture slot to match. The very first tick always binds frame 0 immediately
// rather than waiting a full frameSeconds to show anything.
func (s *ScreenAnimation) tick(mesh *CompoundMesh, elapsed float64) {
	if !s.started {
		s.started = true
		mesh.SetTexture(s.textureIndex, s.frames[0])
	}
	s.elapsed += elapsed
	for s.elapsed >= s.frameSeconds {
		s.elapsed -= s.frameSeconds
		s.frame = (s.frame + 1) % len(s.frames)
		mesh.SetTexture(s.textureIndex, s.frames[s.frame])
	}
}
