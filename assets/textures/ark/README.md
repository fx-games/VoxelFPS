# ark

Textures the `ark` level needed and the library did not have. Each is made from
one already in it, by the command beside it, run from `assets/textures`.

- `ice.png` - the shield dome's water ice: `Space/_original/WATER_1A.PNG` with
  the colour taken out and put back as pale cyan.

  ```bash
  python3 -c "
  from PIL import Image, ImageOps
  g = ImageOps.autocontrast(Image.open('Space/_original/WATER_1A.PNG').convert('L'), cutoff=2)
  ImageOps.colorize(g, black=(60,110,135), white=(235,250,255)).save('ark/ice.png')"
  ```
