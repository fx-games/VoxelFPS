# Bullet chips

`chip1.png` .. `chip5.png` are GZDoom's own bullet-hole decal graphics, taken
verbatim from <https://github.com/ZDoom/gzdoom> (`wadsrc/static/graphics/`).
They are 7x5 to 7x9 pixel greyscale images, and the grey is *alpha*, not
colour: `decaldef.txt` gives each of BulletChip1..5 `shade "00 00 00"` and
`translucent 0.85`, so ZDoom paints the shape in flat black at 85% of whatever
the picture's own grey says.

`go run ./tools/gendecals` does that conversion and writes the five of them -
at this size, not blown up - into `weapons/_sprites`, the shared picture store
the game packs into one atlas at startup. It writes the blood splatter there
too, and deletes `weapons/_sprites.png` so the next run repacks.
