package zdwep

import (
	"regexp"
	"strconv"
	"strings"
)

// A mod's scripts read its own options through ACS, and the mod ships those
// options' defaults in its CVARINFO lump. Reading them there is both more
// accurate and more honest than a table of guesses in this repo: the values are
// the mod author's, and a new version of the mod brings its own.

var cvarLine = regexp.MustCompile(`(?i)^\s*(?:server|user|nosave)\s+(?:noarchive\s+)?(int|bool|float|string)\s+(\w+)\s*=\s*([^;]+);`)

// LoadSettings reads a mod's CVARINFO on top of the built-in defaults.
//
// ACS hands a float cvar back in fixed point, sixteen bits of fraction, which is
// why the scripts divide by 65536. Ints and bools come back as themselves.
func LoadSettings(src *Source) Settings {
	settings := DefaultSettings()
	data, err := src.ReadFile("CVARINFO")
	if err != nil {
		return settings
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.SplitN(line, "//", 2)[0]
		match := cvarLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		kind, name, raw := strings.ToLower(match[1]), strings.ToLower(match[2]), strings.TrimSpace(match[3])
		value, ok := cvarValue(kind, raw)
		if !ok {
			continue
		}
		settings.ACS[name] = value
	}
	return settings
}

func cvarValue(kind, raw string) (float64, bool) {
	switch kind {
	case "bool":
		switch strings.ToLower(raw) {
		case "true", "1":
			return 1, true
		case "false", "0":
			return 0, true
		}
		return 0, false
	case "int":
		number, err := strconv.Atoi(raw)
		if err != nil {
			return 0, false
		}
		return float64(number), true
	case "float":
		number, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0, false
		}
		return number * 65536, true
	}
	return 0, false // a string cvar is not a number a weapon branches on
}
