# station

Wall textures drawn from scratch by `go run ./tools/gentex` - see the comment at
the top of `tools/gentex/main.go`.

Nothing here is traced, sampled, or copied from anything. The generator reads a
reference folder only for two things per name: the size of the picture and its
colour scheme - five greys from dark to light plus a few saturated accents, each
rotated a few degrees around the colour wheel on the way in. Everything else -
plates, bolts, conduit runs, hazard stripes, corrosion, lava, vines, screens - is
drawn by the code, seeded by the texture's name, so rerunning gives the same set.

The names match the reference folder so the set drops in where it did. The
animated groups (`CMP*`, `PLN*`, `LOGO*`, ...) keep their frame numbering; the
frame number drives the phase of whatever is moving in them.
