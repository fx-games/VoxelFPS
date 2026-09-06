package zdwep

import (
	"bufio"
	"bytes"
	"path"
	"strings"
)

// A TEXTURES lump gives a name to a picture that is not a lump of its own. It
// is how a mod makes an icon out of a sprite it already ships:
//
//	Graphic PLCHAING, 43, 13
//	{
//	    XScale 0.500
//	    YScale 0.500
//	    Patch PCHGZ0, 0, 0
//	}
//
// Final Doomer's whole weapon HUD is built that way - eighty-one of these, one
// per weapon, each a half-scale copy of that weapon's own pickup sprite - and
// `Inventory.Icon "PLCHAING"` names one. Without reading this, the icon a
// weapon declares is a name for nothing.
//
// Only the first patch matters here. A texture is a canvas with pictures
// stamped on it and can hold several; every one of these is one sprite copied
// at half size, and a composite icon is not something this can draw anyway.

// readTextures maps a texture name to the lump its first patch names, for every
// TEXTURES lump in the sources. GZDoom reads any lump called TEXTURES whatever
// extension it carries, which is how one mod ships TEXTURES.hud beside
// TEXTURES.weapons.
func readTextures(src *Source) map[string]string {
	textures := make(map[string]string)
	for _, name := range src.Names() {
		base := path.Base(strings.ToLower(name))
		if stem, _, _ := strings.Cut(base, "."); stem != "textures" {
			continue
		}
		data, err := src.ReadFile(name)
		if err != nil {
			continue
		}
		readTexturesFrom(data, textures)
	}
	return textures
}

// readTexturesFrom reads one lump. A definition already seen is left alone, so
// the first TEXTURES lump to name a texture keeps it - the same first-one-wins
// the sprite index uses, and for the same reason: the layers are stacked mod
// first.
func readTexturesFrom(data []byte, textures map[string]string) {
	var current string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if comment := strings.Index(line, "//"); comment >= 0 {
			line = strings.TrimSpace(line[:comment])
		}
		keyword, rest, _ := strings.Cut(line, " ")
		switch strings.ToLower(keyword) {
		case "graphic", "sprite", "texture", "walltexture", "flat":
			current = strings.ToUpper(textureField(rest))
		case "patch":
			if current == "" {
				continue
			}
			if _, taken := textures[current]; !taken {
				textures[current] = strings.ToUpper(textureField(rest))
			}
			current = ""
		}
	}
}

// textureField is the name at the head of a definition line, which is followed
// by the size or the offsets: `PLCHAING, 43, 13`.
func textureField(rest string) string {
	name, _, _ := strings.Cut(rest, ",")
	return strings.Trim(strings.TrimSpace(name), `"`)
}
