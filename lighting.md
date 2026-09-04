# Lighting

How a fragment in this engine ends up the colour it is, and where each knob lives.

Four things light a surface, and they are deliberately different in cost:

| source | count affordable | cost per frame | shadows |
|---|---|---|---|
| Ambient | 1 | nothing | no |
| Baked voxel light | dozens of emitters | nothing (a texel already fetched) | occlusion is baked in |
| Emissive texels | any | nothing | n/a, it is not a light |
| Shadow-mapped spotlights | ~3 today, 16 hard cap | a full scene redraw each, when dirty | yes |

The design follows from that table. Spotlights are the expensive, dramatic light and
there are a handful. Everything static and cheap is baked. Nothing is a spotlight
because it happens to be a lamp — it is a spotlight because it moves, or because its
shadow is the point.

Everything below meets in one function, [`finalColor`](quads/shader_header.go), which
every scene shader calls:

```glsl
vec3 litColor = (ambientLightColor + baked + spotLightColor) * surfaceColor.rgb + emissive;
```

Three terms multiply the surface, one is added after. That split is the whole model:
`emissive` is a surface that *emits*, so no amount of shadow or distance can darken it,
while the other three are light *arriving*, so they scale what the texture reflects.

---

## 1. Ambient

A flat floor under everything, so an unlit corner is dark rather than black.

`globalLightColor` in the main UBO: `rgb` the colour, `a` the strength. Default
`{0.2, 0.2, 0.2}` at strength 1 ([gl_application.go:343](quads/gl_application.go:343)),
set at runtime with `SetAmbientLight(colour, strength)`.

The shadow test rig (`-shadowtest`) drops it to `{0.35, 0.4, 0.55}` at 0.08 and pushes
the fog out to 60–120 ([main.go:393](main.go:393)). With the shipped values the light
pool and the fog fade were the same shade of brown and the shadow could not be told from
either — the rig is a microscope, so everything that dims a surface for a reason other
than shadow is moved out of the way.

## 2. Shadow-mapped spotlights

### The data

A light is a [`LightInfo`](quads/lights_ubo.go): a `*core.PerspectiveTransform` plus an
RGBA colour. **The transform is not just for the shadow pass — it is the light's
definition.** Its position is the light's position, its forward is the beam axis, its
FOV is the cone angle, its far plane is the reach. Which means the region a light
*lights* and the region it has *depth information for* are the same region by
construction, and cannot drift apart.

That is packed into three `vec4`s and a `mat4` by `LightInfo.packed()`, using the spare
components that would otherwise be padding:

```
position.xyz   world position        position.w    reach (the far plane)
color.rgb      colour                color.w       intensity
direction.xyz  beam axis             direction.w   cos(half FOV) — the cone edge
projView                             the light's projection * view
```

Before this, reach was read from `fogRange` and the cone was a hardcoded
`smoothstep(0.94, 0.98)` — every light in the world had identical reach and identical
cone, and moving the fog slider relit the level.

`LightsUBO` holds them in a std140 uniform block of `MaxLights = 16`. `SetLights`
uploads the whole array; `updateLightAt(i)` re-uploads one 28-float stride, which is
what the flashlight uses every frame.

**Slot 0 is the flashlight and nothing else.** It is parented to the camera, updated by
index every frame, and belongs to the player rather than to any map. `SetStaticLights`
therefore rebuilds the list *around* slot 0, and `AddLights` appends rather than
replaces — replacing the whole list would unhook it
([gl_application.go:362](quads/gl_application.go:362)).

Off is `{0,0,0,1}`, not a removal: a light's index is also its layer in the shadow
atlas, so dropping one out of the list would hand every light after it someone else's
cached depth map.

### Intensity is not a multiplier

`color.a` is the brightness at `LIGHT_REF_DIST` (2.5 units), not a scale on the final
colour. Falloff is inverse square:

```glsl
float distFactor = dist / LIGHT_REF_DIST;
float falloff = (1.0 / (1.0 + distFactor * distFactor))
              * (1.0 - smoothstep(range * 0.7, range, dist));
```

Hence the flashlight's `{0.7, 0.7, 0.7, 8.0}` — an intensity of 8 is normal, not a bug.
Half brightness lands at 1.7x the reference distance, quarter at 2.6x. The second factor
fades the light out over the last 30% of its reach, because past the far plane there is
no shadow map for it and a light that reached further than its shadow would light
through walls.

Inverse square was not always there. Without it, a wall one unit away and a wall eight
units away were both at full brightness and only the diffuse angle separated them —
which is why a corridor's near side walls went black while the far wall the light
pointed at stayed bright.

### The shadow pass

Once per frame, before the scene ([gl_application.go:432](quads/gl_application.go:432)):

1. Viewport to `shadowResolution` (512), bind the depth-only framebuffer.
2. For each light: skip if `IsOff()`, skip if `!NeedsShadowUpdate(sceneSignature)`.
3. Otherwise select its layer, clear depth, and redraw the whole scene from the light.

Three things make this affordable:

**Culling uses the light, not the camera.** `draw3dScene` takes an `observer` and every
drawer culls against it. When this was hardcoded to `a.Camera`, anything behind or
beside the player was skipped while rendering the light's depth map, so its shadow
silently vanished — turn around and shadows popped.

**A depth map is only redrawn when it is stale.** `NeedsShadowUpdate` compares the
light's own `projView` and a `sceneSignature` — the map's mesh generation counter mixed
with an FNV hash of every caster's position
([`shadowSceneSignature`](quads/gl_application.go)). Two static lights over a still
scene cost zero passes. `ShadowPasses()` reports the count, and the benchmark line
prints it (`2 of 2 lights redrew their shadow map`).

**A light that is off is skipped entirely** — no depth pass, and `finalColor`'s loop
skips it too. Turning the flashlight off used to cost full price: black light, full
shadow map, full `calcLight` and `ShadowCalculation` per fragment.

### Shadow sampling and bias

The depth array is a `sampler2DArrayShadow` with `TEXTURE_COMPARE_MODE =
COMPARE_REF_TO_TEXTURE` and `LINEAR` filtering
([texture_array.go:63](quads/glhf/texture_array.go:63)), so **every fetch is already a
2x2 PCF tap in hardware**. `ShadowCalculation` does 3x3 of those by hand — GLSL has no
`textureOffset` overload for a shadow array sampler — which softens 512² edges cheaply.

The bias is **normal-offset, not depth bias**, and the reason is the floor:

> Both static lights sit at height 2 pointing roughly horizontally, so floors are lit at
> 10–20° of grazing incidence while walls are lit close to head-on. A slope-scaled depth
> bias is near its maximum on the floor and near its minimum on the walls, so the value
> that keeps walls free of acne erases floor shadows completely. The window between
> "acne over the whole floor" and "no floor shadow at all" was too narrow to sit in.

So the sample position is pushed along the surface normal in **world space** before
being projected into light space, and the push is measured in shadow texels *as they
measure at that fragment*:

```glsl
float texelWorldSize = 2.0 * dist * tanHalfFov / float(textureSize(shadowMap, 0).x);
float grazing = clamp(1.0 - dot(normal, lightDir), 0.0, 1.0);
vec3 samplePos = fragWorldPos + normal * (texelWorldSize * (1.0 + 2.0 * grazing) * shadowParams.x);
```

`tanHalfFov` comes from the cosine already packed in `direction.w`. One number therefore
means the same thing at any distance, any cone angle and any shadow resolution. What is
left of the depth bias is a flat `0.0005` against depth-buffer quantisation.

Two more guards earn their place:

- **`w <= 0`**: fragments behind the light's near plane divide through a negative `w` and
  land on coordinates that look perfectly valid inside the map. Nothing behind a light is
  shadowed by it.
- **Spotlight near plane 0.5**, not the camera's 0.15
  ([transform_perspective.go:56](core/transform_perspective.go:56)). 0.15-to-15 is a
  100:1 range that spent its depth resolution on the first metre; nothing casts within
  half a unit of a light bulb.

Tune with `-shadow-offset` / `-shadow-bias`, or the two F1 sliders. Judge with
`-shadowtest`, which is a rig built for exactly this: one caster, one wall, one light.

### Omni lights *(uncommitted, in the working tree)*

`LightInfo.Omni` makes a point light: lights in every direction, casts no shadow. The
shader recognises it by a cone cosine of `-1` — a value a real cone cannot reach, since
that would be a 360° spotlight — and skips both the cone test and the shadow lookup. The
shadow pass skips it outright.

`AttachLightTo(obj, …)` parents one to a game object so it moves with it and switches off
when the object dies. There is one tracked slot for the whole application; a second
simultaneous one would need a slot pool and a light index that is no longer a shadow
atlas layer.

## 3. Baked voxel light

The cheap half of the lighting: **dozens of static sources for four spare bits in a
buffer the shader already reads.**

### What emits

Not a light entity — the emissive texture set. Every face using one of those textures
lights the air in front of it. This reuses authored data that already says which faces
glow, so what glows and what lights the room cannot disagree:

```go
g.CubeMap.SetEmissiveTextures([]uint16{19, 26, 27, 29, 30, 45, 46, 47, 49})
```

([main.go:350](main.go:350) — cyan tube panels, lit strips, console screens.)

### The flood fill

`Map.bakeEmissiveTorchlight` walks the grid, seeds `EmissiveTorchLight` (12) into the
air block in front of every emissive face, and floods from there — one level lost per
step, stopping at solid blocks, 4 bits per block (`MaxTorchLight` 15). Solid blocks hold
no light; it lives in air, which is why it is per *face* of the wall that sees it.

Below the maximum on purpose: a wall fitting is fill light, and the headroom keeps a
hand-placed `FillTorchlight` brighter than the scenery.

Two bugs it was carrying while it sat unused: `nodeLight - 1` on a `byte` underflowed to
255 at level 0, which would have spread full brightness across the entire map, and a
`println` fired on every spread step.

On `raymap.ldtk`: **680 emitting faces, ~8ms**, producing 2084 lit lookup entries.

### Where the level lives — and why not in the vertices

In the spare bits of the voxel lookup buffer, the `uint16` per block face that the cube
shader **already fetches once per fragment** to find its texture index:

```
bit 15      emissive
bits 14-11  baked torch light (0..15)
bits 10-0   texture index
```

(`Map.packFaceLookup`, mirrored by `EMISSIVE_TEXTURE_BIT` / `TORCH_LIGHT_MASK` /
`TORCH_LIGHT_SHIFT` / `EMISSIVE_TEXTURE_MASK` in the shader, pinned by
`TestEmissiveMaskCoversAtlasIndices`.) Blocks carry a byte texture id, so 11 bits is 8x
the range the data model can produce.

It went into the vertex bits first, and that is the instructive part. A vertex-borne
level forces `EqualForMerge` to compare light levels, which stops greedy meshing from
folding a lit face together with a dark one — because a merged quad carries **one**
level for all four vertices. Measured on raymap:

| merge key | triangles | median frame |
|---|---|---|
| visibility only, no baked light | 1324 | 0.631 ms |
| visibility + light (light in vertices) | 3448 | 0.619 ms |
| never merge (bound) | 15152 | 0.729 ms |
| **visibility only, light in lookup buffer** | **1324** | **0.550 ms** |

Greedy meshing still kept 77% of its reduction in the vertex version, and this map is
fragment-bound rather than vertex-bound, so it was survivable. But the buffer gives an
*identical picture* — both are per-block-per-face — for no triangles and no extra fetch.
Vertex bits 20-31 are free again as a result.

### When it re-bakes

`DrawWithShader` re-floods whenever the lookup buffer is dirty, before packing it. That
one trigger covers everything: a panel switched on, a wall repainted, a block placed
next to a lamp. Placing a wall now changes the light it blocks — with the level in the
vertices, the old fill stood until something touched the emissive set.

It costs a full re-flood, ~8ms on raymap, in a frame that was already walking every
block and re-uploading the buffer. Marked `ponytail:` with the per-chunk upgrade path if
painting ever hitches.

### Turning a level into light

```glsl
const vec3 BAKED_LIGHT_COLOR = vec3(0.85, 0.92, 1.0);
const float BAKED_LIGHT_STRENGTH = 0.55;

vec3 bakedLight(float lightLevel) {
    return BAKED_LIGHT_COLOR * BAKED_LIGHT_STRENGTH * (lightLevel / 15.0);
}
```

Cool white because what emits it here is cyan tubes and console screens. Below 1.0
because it is fill: it lifts a room off pure ambient without competing with a spotlight.
Unshadowed and directionless on purpose — **the level already encodes how far the light
got around corners**, which is the expensive part a shadow map would otherwise buy.

Constants rather than uniforms: the UBO is shared by every shader and this has not
needed to move at runtime. If it wants a slider it joins `shadowParams` in `mainUBO`.

## 4. Emissive texels

Not a light — a surface that emits. It lights nothing else directly; it feeds the baked
fill (§3) and bloom (§5).

The top bit of the lookup entry marks a face as emissive, but **marking a texture does
not light the whole tile.** A texel glows only if it is both bright *and* colourful:

```glsl
float emissiveMask = smoothstep(emissiveCutoff, emissiveCutoff + 0.1, luminance)
                   * smoothstep(emissiveSaturation, emissiveSaturation + 0.1, saturation);
vec3 emissive = surfaceColor.rgb * isEmissive * emissiveMask * emissiveStrength;
```

That is what tells a cyan tube or a blue screen readout apart from the grey metal panel
it is mounted on, which is just as bright. Soft edges on both terms so the boundary does
not crawl as the texture minifies.

Defaults: strength 7.0, cutoff 0.35, saturation 0.35
([shader_cubes.go:36](quads/shader_cubes.go:36)). Strength above 1.0 is the point — see
below.

Press **G** at any wall to toggle its texture emissive at runtime. Keyed on the texture,
not the block, so marking one screen panel lights every screen panel in the level.

## 5. Where the light goes after `finalColor`

The order matters and is not obvious:

1. **`finalColor` returns linear HDR.** No tone mapping per fragment. Anything the
   flashlight hits head-on runs past 1.0 and *stays* past 1.0.
2. **Fog** is applied inside `finalColor`, after the lighting multiply. Fog distance is
   measured on the horizontal plane only, Minecraft-style, so tilting the head does not
   shift fog across the world. `fogColor` is authored in display range, so it is lifted
   through `hdrFromDisplay` first — otherwise tone mapping would darken it on the way
   out. The same applies to the clear colour, which is lifted in Go by `HDRFromDisplay`;
   `TestHDRFromDisplayRoundTrips` keeps the two curves in step.
3. **Bloom** thresholds the HDR scene buffer. This is why emissive strength defaults to
   7.0 and why per-fragment tone mapping had to go: tone mapping clipped the >1.0
   information away before anything could use it.
4. **Tone map once**, in the post pass, with exposure and gamma from `toneMapParams`.

## 6. Authoring

**Lights are map data.** `core.MapLight` is the authored form — position, yaw, pitch,
colour, intensity, cone angle, reach — carrying no GL types, so a map format never has
to know how a shadow frustum is built. `quads.LightFromMap` turns one into a
`LightInfo`; `LoadVoxelMap` **replaces** the scene's lights on every load, so a hot
reload does not leave the last map's lights burning.

In `raymap.ldtk`, a `Light` entity with fields `Height`, `Yaw`, `Pitch`, `Intensity`,
`Angle`, `Reach`. Grid position plus half a tile centres it. Defaults if unset or ≤0:
angle 45°, reach 15, intensity 1. `TestLDtkMapCarriesItsLights` fails if a save from the
editor drops them.

Generated maps use `MapLight.LookAt(target)` — they know what they want lit rather than
what angle that works out to. `TestLookAtAndForwardAreTheSameRotation` checks both
directions agree.

**F1 → Drop Light Here** places a light where the camera stands, pointing where it
looks, and toasts the yaw and pitch to copy into LDtk. It is a way to *find* the numbers,
not a way to save them — nothing writes a light back to the map file, and placement in
the running engine is a scratchpad.

**Runtime knobs (F1):** Exposure, Bloom Threshold, Bloom Strength, Emissive
Strength/Cutoff/Saturation, Shadow Normal Offset, Shadow Depth Bias, Fog Start/End, Sky
Horizon Fade.

**CLI:** `-shadowtest`, `-shadow-offset`, `-shadow-bias`, `-stress-lights`, `-bench`.

## 7. Budgets and known gaps

- **16 lights hard cap** (`MaxLights`). Not free headroom: the shadow atlas allocates all
  16 layers of 512² depth whether used or not.
- **Forward shading, loop over all active lights per fragment**, each with a shadow
  sample. Fine at 3, painful at 20+. The ladder from here: baked light for statics
  (done) → per-light range culling on the CPU → clustered forward. Deferred fights the
  `texelFetch` atlas and should be the last resort.
- **No directional/sun light**, and no unshadowed fill spotlight. A `type` field would
  let cheap decorative lights skip the depth pass entirely — that is what makes
  many-light scenes affordable.
- **Baked light is flat per face**, no gradient across a quad. That is the resolution
  greedy meshing leaves and the same resolution the emissive flag has always had. Smooth
  would need a level per vertex.
- **`BAKED_LIGHT_STRENGTH = 0.55` has never been judged by eye.** The wiring is verified;
  the value is a guess.
- **`updateLightAt(0)` runs unconditionally every frame** — a `glBufferSubData` on the UBO
  even when the camera has not moved (finding #20).
- **No sun/skylight flood.** `Block.SetSunLight` / `GetSunLight` hold the upper 4 bits
  of the same byte the torch light uses, and `GetSunLight` has no callers — nothing
  reads sun light anywhere. `toVoxelMap` used to seed both to 14 on every block in the
  map; `bakeEmissiveTorchlight` clears before it floods so it never showed, but a load
  where the bake had not run yet would have come out uniformly bright. Removed.
