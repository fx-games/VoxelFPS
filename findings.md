# Findings — renderer review, September 2026

A read-through of the render path with three questions in mind: realtime lighting,
shadow casting, engine performance. Ordered by payoff within each area. Line
references are to the state of `main` at the time of the review.

Status column: `[ ]` open, `[x]` done, `[~]` partially done.

---

## Performance

### [~] 1. `gl.GetError()` in the hot path

63 call sites. The damaging ones are per-draw and per-uniform:

- `quads/glhf/shader.go:267` — every `SetUniformAttr` call
- `quads/voxel/chunk.go:468` — every chunk draw
- `quads/sys_sprites.go:124,141,154,169,173` — twice per sprite

`glGetError` drains the driver command queue. With ~50 visible chunks x 4 passes
(3 shadow + 1 main), each doing a model-matrix uniform plus a draw, that is ~400
full pipeline syncs per frame from chunks alone, plus 5 per sprite per pass.

**Fix:** delete them from draw paths. Keep the ones at load/init time. For real
debugging use `glDebugMessageCallback` (GL 4.3) or gate the checks behind a
`debugGL` build tag.

**Second order:** `SetUniformAttr(uniform int, value interface{})` boxes every
`mgl32.Mat4` into an interface — a heap allocation per uniform set, thousands per
frame. Typed methods (`SetMat4`, `SetVec2`) remove both the allocation and the
type switch.

### [ ] 2. One draw call per sprite

`quads/sys_sprites.go:235` draws the same 4-vertex quad once per sprite, resetting
three uniforms each time. `glhf.SpriteBatch` exists but is only used for the GUI.

**Fix:** instanced rendering — a per-sprite buffer of (model matrix, texIndex,
originOffset) with `glVertexAttribDivisor`, one `glDrawElementsInstanced` per
atlas. `quads/glhf/vertex.go:236` already has divisor handling for matrix
attributes.

`quads/sys_meshes.go:84` has the same shape: one draw per node per object.

### [x] 3. Draw order comes from `map[*Component]bool` iteration

`quads/sys_sprites.go:39` and `quads/sys_meshes.go:12` key their draw lists by
pointer. Random iteration order means no state-change batching, non-deterministic
alpha/z artifacts frame to frame, pointer chasing, and no way to sort.

**Fix:** slices with swap-remove.

**Landed.** `spritesPerTexture` and `gameObjectsPerMesh` hold `[]*Component`.
`Remove` scans the list and swaps in the last element; `Update` walks backwards so
a removal cannot skip its neighbour. Fixed on the way: the transparent list
updated a sprite it had just removed. Covered by `quads/sys_meshes_test.go`.

### [x] 4. Frustum culling is broken in both directions

`quads/voxel/map.go:229` `chunkIsVisible` returns after the sphere test; the AABB
path below is unreachable (known, marked `// TODO: THIS DOES NOT WORK`).

The AABB path is also wrong on its own terms:
`core/transform_perspective.go:135` `IsCubeInViewFrustum` only tests whether a
*corner* is inside the frustum. A box straddling the frustum with no corner inside
— the common case for a large chunk near the camera — is culled.

**Fix:** extract the 6 frustum planes once, test the AABB's positive vertex
against each. ~20 lines, and it replaces `IsInViewFrustum`,
`IsCubeInViewFrustum` and `IsSphereInViewFrustum` with one correct primitive.

### [x] 5. Per-cell allocation in the physics broadphase

`quads/sys_physics.go:127` — `getCollidersAt` is a closure allocated per object per
frame, and it `make()`s a fresh slice for every grid cell visited.

**Fix:** reusable `[]LinkedAABB` scratch buffer, truncated per call.

**Landed.** `PhysicsSystem.colliderScratch`, truncated at the top of
`getCollidersAt`, and the closure is built once per step instead of once per
object. Safe because the collision test consumes the returned colliders before
asking for the next cell. Covered by `TestUpdateStopsAtObjectOnGrid`.

### [x] 6. Physics runs on a variable timestep

`PhysicsSystem.Update(deltaTime)` takes raw frame time, so jump height and fall
speed vary with framerate.

**Fix:** fixed accumulator (`for acc >= 1.0/60 { step(1.0/60); acc -= 1.0/60 }`).
Also makes the collision code reproducible in tests.

### [ ] 7. Voxel textures use `texelFetch` — no mips, no filtering

`quads/shader_cubes.go:264` does manual atlas UV math and `texelFetch`, so distant
walls alias and shimmer and the texture cache thrashes.

**Fix:** the texture-array plan already in `optimize.txt` — one `sampler2DArray`
layer per tile gives free mipmaps, correct filtering and no atlas bleed. Worth
doing for image quality as much as for speed.

Related, same file `[x]`: `mapSizeX/Y/Z` were hardcoded `64/32/64` at
`quads/shader_cubes.go:222`. Any map of a different size silently sampled the
wrong texture indices. Now a `mapSize` uniform, fed from `Map.VoxelDimensions()`,
which is also the one place the lookup buffer's stride is computed.

### [x] 8. `go chunk.GreedyMeshOnOtherThread()` is spawned inside the draw loop

`quads/voxel/map.go:216` — an unbounded goroutine started mid-render, reading
`c.data` while the main thread may be writing it through the editor.

**Fixed, but not that way.** A worker goroutine does not help: the race is on
`chunk.data` and the chunk's shared `chunkHelper`, which the mesher reads while the
editor writes them. Meshing now runs on the main thread in `remeshDirtyChunks`,
before the draw, two chunks per frame. `GreedyMeshOnOtherThread` is gone. If
painting large areas hitches, the next step is meshing from a copy of `chunk.data`,
not putting the goroutine back.

---

## Shadow casting

### [x] 9. The shadow pass culls against the camera, not the light

`quads/gl_application.go:449`:

```go
a.CubeMap.DrawWithShader(a.Camera, chunkShader) // camera only used for frustum culling
```

`draw3dScene` takes `observer core.Projector` and uses it for the view matrix, but
hardcodes `a.Camera` for culling in all three drawers. Anything behind or beside
the player is skipped when rendering the light's depth map, so its shadow silently
disappears. Turn around and shadows pop.

The hardcoding is structural: `quads/gameobject.go:170`
`IsInViewFrustum(camera *core.FPSCamera)` takes a concrete type.
`core.PerspectiveTransform` already implements all three frustum methods.

**Fix:** widen that parameter to a small interface and thread `observer` through.

### [x] 10. Static lights re-render their shadow map every frame

`quads/gl_application.go:351` loops all lights unconditionally. Two of the three
lights never move and nothing in the test map animates.

**Fix:** a `dirty` flag on `LightInfo`, set when the light moves or a chunk
re-meshes. Cuts two full scene redraws per frame.

### [x] 11. The flashlight costs full price while switched off

"Off" sets the color to black (`quads/gl_application.go:743`). The shadow map is
still rendered and the fragment shader still runs `calcLight` plus
`ShadowCalculation` for it.

**Fix:** skip zero-color lights in the shadow loop and in the shader's light loop.

### [x] 12. Shadow quality left on the table

`quads/shader_header.go:76` — `ShadowCalculation` computes a slope-scaled `bias`
and then ignores it in favour of a hardcoded `0.001` on the next line. Dead code
that was presumably the fix for the acne the constant compensates for. Single
depth sample, so edges are hard-aliased at 512x512.

**Fix, cheap:** set `TEXTURE_COMPARE_MODE = COMPARE_REF_TO_TEXTURE` plus `LINEAR`
filtering on the depth array (`quads/glhf/texture_array.go:57`) and switch the
sampler to `sampler2DArrayShadow` — hardware 2x2 PCF for free. A 3x3
`textureOffset` loop on top if softer edges are wanted.

**Fix, also cheap:** enable front-face culling during the depth pass. Removes most
acne at the source and lets the bias hack go.

### [x] 13. The shadow atlas is always 64 layers

`quads/glhf/frame.go:153` allocates 64 x 512^2 x 4 bytes = 64 MB of VRAM for the
three lights that exist.

**Fix:** size it to the light count.

---

## Realtime lighting

### [ ] 14. There is no way to place a light

`GlApplication.SetLights` at `quads/gl_application.go:862` is an empty stub. The
three lights are hardcoded in the constructor, the LDtk loader has no light
entity, and the editor cannot spawn one. This is the actual blocker for the
feature; everything below is secondary.

### [x] 15. Light range is the fog range

`quads/shader_header.go:107` — `falloff` uses `fogRange.x/y`, and the cone is a
hardcoded `smoothstep(0.94, 0.98)`. Every light in the world has identical reach
and identical cone angle, and moving the fog slider changes all the lighting.

**Fixed.** Reach, cone and intensity now come from the light, packed into the spare
vec4 components by `LightInfo.packed()`: `position.w` reach, `direction.w` the
cosine of the half FOV, `color.w` intensity. Reach and cone are read off the light's
own shadow frustum, so the region it lights and the region it has depth information
for cannot drift apart. Falloff is full to half the reach, then out to nothing at
the far plane. Covered by `TestPackedCarriesFrustumReachAndCone`.

### [ ] 16. Every light is a shadow-casting spotlight

No point lights, no directional/sun, no unshadowed fill lights.

**Fix:** a `type` field, so cheap decorative lights skip the depth pass entirely.
That is what makes many-light scenes affordable.

### [ ] 17. The baked voxel lighting is computed and thrown away

A complete Minecraft-style flood-fill light system exists — `FillTorchlight`
(`quads/voxel/map.go:585`), per-block light levels, propagated to faces during
greedy meshing, packed into the vertex bits (`quads/voxel/mesh_buffer.go:222`),
interpolated to the fragment shader — and then:

```glsl
vec3 floodLight = vec3(2.0, 2.0, 2.0) * (VertLightLevel / 15.0) * camDirection.xyz;
```

`quads/shader_cubes.go:268` — assigned, never used in `finalColor`. It also
multiplies by `camDirection`, which is meaningless. Meanwhile `FillTorchlight` has
zero callers, so nothing sets a light level in the first place.

This is the cheapest realtime-lighting upgrade available: baked voxel light gives
dozens of static lights at zero per-frame cost, exactly the budget the
shadow-mapped spotlights cannot fit.

**Fix:** either wire it up (call `FillTorchlight` from the map loader, use
`VertLightLevel` as an additive term in `finalColor`) or delete the whole path.
`FillTorchlight` needs two fixes on the way in: a `println` inside the flood fill,
and a `nodeLight - 1` byte underflow when the level is 0.

### [ ] 18. Forward shading with a fixed 64-light loop

`quads/shader_header.go:135` iterates all active lights per fragment with a
shadow-map sample each. Fine at 3, painful at 20+.

**Fix ladder:** baked voxel light for statics (#17) -> per-light range culling on
the CPU -> clustered forward. Deferred is listed in `optimize.txt` but it fights
the `texelFetch` atlas and should be the last resort.

---

## Correctness bugs found along the way

### [x] 19. Texture binder leak in the transparent sprite path

`quads/sys_sprites.go:196` — `sprite.AtlasTexture.Begin()` then `continue` on the
frustum check without calling `End()`. The binder's saved state is never restored,
corrupting the texture binding for later sprites.

**Fix:** move the `Begin()` after the cull test.

### [ ] 20. `updateLightAt(0)` runs unconditionally every frame

`quads/gl_application.go:342` — a `glBufferSubData` on the UBO per frame even when
the camera has not moved.

### [x] 21. `GetProjectionViewMatrix()` rebuilds the projection every call

`mgl32.Perspective` from scratch, several times per light per frame.

**Fix:** cache, invalidate on FOV/aspect change.

---

## Suggested order

1. Delete the hot-path `gl.GetError()` calls (#1) — biggest speedup, smallest
   diff, zero risk.
2. Fix shadow-pass culling to use the light (#9) — biggest visual correctness win.
3. Skip static and black lights in the shadow pass (#10, #11) — two fewer full
   scene passes.
4. Depth-compare sampling and front-face culling in the depth pass (#12) — better
   shadows, less code.
5. Plane-based frustum culling (#4) — unblocks the "do better frustum culling"
   item in `optimize.txt`.
6. Then the structural work: instanced sprites (#2), light placement API (#14),
   per-light range and cone (#15).


### [ ] 22. Floor surfaces get no shadows at all

Reported from a screenshot: shadows show up on walls but never on the floor.
Confirmed, and it is not a floor-specific code path - it is the depth bias.

What the evidence shows:

- Dumping the shadow maps (`TextureArray.SaveAsPNGs`, layers 1 and 2) shows the
  floor filling most of each light's depth map with the barrels, desk, sink and
  scientist recorded as clean silhouettes standing on it. Casting and receiving
  are both fine.
- Rebuilding with the depth bias dropped to 0.00002 turns the whole floor dark:
  it self-shadows everywhere. So floor fragments do run the shadow lookup and can
  be shadowed - the bias is what suppresses them.
- The window between "acne over the whole floor" and "no floor shadow at all" is
  therefore very narrow, and the shipped bias sits on the no-shadow side.

Why it lands on floors and not walls: both static lights sit at height 2 and point
horizontally, so floors are lit at roughly 10-20 degrees of grazing incidence
while walls are lit close to head on. The slope-scaled term
`0.0015 * (1 - dot(normal, lightDir))` is near its maximum on the floor and near
its minimum on the walls. On top of that, the shadow frustum runs near 0.15 to far
15, a 100:1 ratio that leaves little depth resolution at the 8-15 units where the
floor is - so the depth offset the bias buys is worth close to a metre of world
space out there, comfortably more than the gap between a barrel and its own
shadow.

**Proposed fix, written but NOT verified.** Two parts:

1. Normal-offset bias: nudge the sample position along the surface normal in world
   space before projecting into light space, instead of offsetting in depth. A
   world-space offset stays the same size at any distance and any angle, which is
   exactly where depth bias fails.
2. Push the shadow near plane from 0.15 to about 0.5. Nothing casts within half a
   unit of a light, and the depth distribution improves several-fold.

Both were implemented and built. The first run still showed a dark (self-shadowed)
floor, so the offset as tuned was not covering the error, and the machine's
display went to sleep before the follow-up test - an exaggerated offset, to
separate "wrong tuning" from "wrong mechanism" - could be read. Rather than leave
an unverified change that visibly darkens the scene, both parts were reverted; the
shader carries a NOTE pointing here.

To pick this up: apply the two changes above, then raise the normal offset well
past any plausible value (say `1.5 * grazing`) and look at the floor. Bright means
the mechanism works and only the constant needs tuning; still dark means something
else is wrong, and the next suspect is the missing `fragPosLightSpace.w <= 0`
guard in `ShadowCalculation` - fragments behind the light's near plane divide
through a negative w and can land on valid-looking coordinates.

Worth noting either way: with lights this low and this horizontal, floor shadows
would be long grazing smears even once the bias is right. A light angled downwards
would show the fix off far better than the two that ship in the test map.

---

# Work log — first pass

Everything below was built, `go test ./...` passes (except the pre-existing
`zdwep` build failure documented in ISSUES.md), and the engine was run and
rendered correctly with the new shaders.

## Landed

**#1 `gl.GetError()` out of the hot path `[~]`** — removed from `SetUniformAttr`,
`Chunk.Draw`, `SpriteDrawer.Draw`, `Map.DrawWithShader`, `SpriteBatch.End`/
`RenderSlice`, `UBO.SetAttribute` and `Frame.SetDepthLayer`. Replaced with
`glhf.CheckGLError`, a no-op unless built with `-tags debugGL`, plus one call per
frame at the end of the render loop. Load-time checks were left alone.
Still open: the `interface{}` boxing in `SetUniformAttr`.

**#4 Plane-based frustum culling `[x]`** — `FrustumPlanes` (Gribb-Hartmann) on
`PerspectiveTransform`, and `IsInViewFrustum`, `IsCubeInViewFrustum` and
`IsSphereInViewFrustum` all rewritten against it. The corner-only cube test and
the `sphereFactor` oval approximation are gone. `chunkIsVisible` now uses the AABB
path that was previously unreachable. Covered by `core/transform_perspective_test.go`,
including the box-enclosing-the-camera case the old code got wrong.

**#9 Shadow pass culls against the light `[x]`** — added `core.Culler` and
`core.Observer`; `GameObject.IsInViewFrustum`, `MeshDrawer.Draw`,
`SpriteDrawer.Draw` and `Map.DrawWithShader` take an interface instead of
`*core.FPSCamera`, and `draw3dScene` passes the observer through. Casters behind
the player now make it into the light's depth map.

**#11 Lights that are off cost nothing `[x]`** — `LightInfo.IsOff`; skipped in the
shadow loop and `continue`d in the fragment shader's light loop. Confirmed in the
running engine: the debug overlay reports 2 lit lights, not 3, with the flashlight
off (its default).

**#12 Shadow sampling `[x]`** — the depth array now has
`TEXTURE_COMPARE_MODE = COMPARE_REF_TO_TEXTURE` and LINEAR filtering, the sampler
is a `sampler2DArrayShadow`, so every tap is hardware 2x2 PCF; 3x3 taps on top.
The slope-scaled bias that was computed and discarded is now the one in use.
Front-face culling in the depth pass was tried and reverted - billboard sprites
are single quads and would have dropped out of the shadow map entirely. Marked
with a `ponytail:` comment.

Note: GLSL has no `textureOffset` overload for a shadow array sampler, so the PCF
taps are offset by hand via `textureSize`. The driver caught this, not the build.

**#13 Shadow atlas sized to a cap `[x]`** — `quads.MaxLights = 16` now drives the
depth array layers, the UBO size and the GLSL light array. 64 MB of VRAM down to
16 MB.

**#19 Texture binder leak `[x]`** — the transparent sprite path binds its atlas
only after the cull and visibility tests, so the early `continue`s can no longer
skip the matching `End()`.

**#10 Static light shadow caching `[x]`** — `LightInfo` caches the
projection-view matrix and a scene hash it last rendered from, and skips the depth
pass when neither changed. Cache logic covered by `quads/lights_ubo_test.go`.

The first run showed `2/2 lit`, both lights still redrawing every frame, which
looked like a broken cache. It is not: instrumenting the caster positions showed
one sprite in the test map genuinely moving every frame (a patrolling entity,
z going 11.5 -> 9.83) while every other caster stayed put. A moving caster
correctly invalidates every light's depth map, so the cache is doing exactly what
it should. My earlier assumption that nothing in the test map animates was wrong.

Two new debug rows make this visible rather than guesswork:

- `Shadows  N/M lit  gen G` - lights redrawn out of lights switched on, plus the
  world mesh generation.
- `Shadow key XXXX` - the caster hash. Changing while `gen` is steady means
  something is moving.

**Consequence, and the next step:** because the hash is global, any single mover
invalidates every light. In a map with one patrolling NPC that means the cache
never engages, which is the current behaviour. The real fix is the standard split:
render static casters into a cached depth map per light and only redraw dynamic
casters on top. That is proper work, not a quick win, and it is the thing to do
before adding more lights.

## Measurement

Ran the unmodified `main` and the reworked build side by side on the default test
map, camera idle, six samples of the engine's own running FPS average each:

| build | avg FPS samples | median |
|---|---|---|
| baseline | 152, 161, 168, 152, 154, 150 | ~153 |
| reworked | 163, 156, 158, 155, 153, 152 | ~156 |

**No meaningful difference, and that is the honest result.** The test map draws
816 triangles in 2-3 draw calls. The costs removed here scale with draw call and
object count, and there is almost nothing to draw, so there is nothing for them to
show up against. The earlier claim that removing `gl.GetError` would be "the
biggest speedup" was reasoning about scale, not a measurement on this scene - it
stands as a prediction for a busy scene, not a demonstrated win here.

Getting a real number needs a scene that actually loads the renderer: many sprites
(where the one-draw-call-per-sprite issue #2 lives), more chunks, and more than
two lights. Worth building a stress map before doing more performance work, so the
remaining items can be judged on numbers rather than argument.

**Built.** `go run . -stress` (see `stress_map.go`): 128x128 tiles, 16 chunks, a
pillar every 6 tiles so greedy meshing cannot collapse the floor, about a thousand
barrel sprites, and 8 spotlights pointing down. Deterministic, so two builds can be
compared on the same frame, and the knobs are flags. The lights point down on
purpose - the two in the test map are horizontal, which is the worst case to judge
floor shadows against (#22).

## Not started

#2 instanced sprites, #7 texture arrays, #14 light placement API, #16 light types,
#17 baked voxel light, #18 many-light shading, #20 unconditional UBO write,
#21 projection matrix cache, #22 floor shadows (diagnosed, fix drafted and
reverted pending verification).

## Second pass

**Shader map size `[x]`, meshing goroutine `[x]`, per-light range and cone `[x]`** -
see the three items above. Built, `go test ./...` passes (except the pre-existing
`zdwep` build failure), and the engine was run: all three shaders compile and the
test map renders.

Symptom that led to #15: with the flashlight on, walls seen at a sharp angle read as
unlit. Two angle-correlated causes stacked. Lambert with the light sitting on the
eye gives `dot(lightDir, normal) = sin(grazing angle)` - 0.26 at 15 degrees off the
wall - and the specular term collapses the same way because `halfwayDir` is then
almost `lightDir`. On top of that the beam lands 8-15 units away at that angle, deep
into the old `fogRange` falloff (4 to 22), costing another 0.13 to 0.65. Result is
about 0.18 against an ambient of 0.2: dimmer than no light at all.

Wrap lighting was considered and rejected. Half-Lambert lifts grazing surfaces but
puts a 0.25 floor under every surface in the cone, including ones facing away from
the light - and a back-facing surface is not in shadow, so nothing suppresses the
glow. It is nearly free for a flashlight, whose light is at the eye, and wrong for
the static lamps, which share the same `calcLight`. Fixing the range instead keeps
the shading honest; the flashlight's `color.w` intensity (2.0) is the knob that
makes grazing walls read.

---

# Second review pass — the render loop and the custom shaders

A read-through of `StartEventLoop`, the six shader pairs, the shared GLSL header and
the drawer/UBO plumbing. Numbering continues from above.

## Landed in this pass

**#23 `spotLightColor` was uninitialised `[x]`** — `finalColor` in
`quads/shader_header.go` declared `vec3 spotLightColor;` and immediately `+=`'d into
it. GLSL does not zero locals, so every lit fragment in the game accumulated onto an
undefined value. It worked only because the drivers in use happen to hand back zeros.
Now `= vec3(0.0)`.

**#24 Transparent sprites were drawn nearest-first `[x]`** — `quads/sys_sprites.go`
sorted the list farthest-first and then iterated it backwards, so the nearest
translucent sprite drew first, wrote depth, and every sprite behind it failed `LEQUAL`
and disappeared outright. Not a blending artefact, a visibility bug. The loop now runs
forwards.

**#25 Transparent sprites blended against the clear colour `[x]`** — the skybox was
drawn after the whole of `draw3dScene`, so anything translucent in front of open sky
had already blended with `CeilingColor` and came out with a halo. `SpriteDrawer.Draw`
is now split, with `DrawTransparent` as its own pass, and `draw3dScene` takes a
`betweenPasses` callback that the main render block uses to put the skybox between the
two. The shadow pass passes `nil`: it has no sky and writes depth only.

**#26 The frame delta was unclamped `[x]`** — dragging the window, hitting a
breakpoint or alt-tabbing handed the next frame a delta of several seconds, and the
physics integrated straight through the level in one step. Capped at
`maxFrameDelta = 0.1`. This is a floor under the symptom, not a substitute for the
fixed timestep in #6, which is still open and still the real fix.

**#27 HDR and bloom `[x]`** — this replaces the "delete the identity post pass" item
that was proposed before HDR was on the table; the pass now earns its keep.

- The scene framebuffer is `RGBA16F` (`glhf.NewFrameBufferHDR`,
  `glhf.NewTextureHDR`), so values past 1.0 survive to the end of the frame.
- Tone mapping moved out of `finalColor` and into the post shader, where it runs once
  per pixel instead of once per lit fragment. Keeping it per fragment was what clipped
  the highlight information that bloom needs before anything could use it.
- Exposure and gamma live in the main UBO as `toneMapParams`, so the scene shaders,
  the skybox and the post pass all read the same two numbers.
- Anything that mixes an authored display-range colour into the HDR buffer has to lift
  it first, or tone mapping would darken it on the way out: `applyFog` and the skybox
  do that with `hdrFromDisplay`, and the clear colour does it in Go with
  `HDRFromDisplay`. The Go and GLSL curves are twins that cannot share code;
  `TestHDRFromDisplayRoundTrips` is what keeps them in step.
- Bloom is a half-resolution extract-and-blur chain (`quads/shader_bloom.go`): one
  shader with an extract mode and a blur mode, ping-ponging between two HDR frames,
  two horizontal+vertical pairs. Added in linear space before the tone map, so a glow
  rolls off with everything else instead of being pasted onto a compressed image.
- Knobs: `SetExposure`, `SetGamma`, `SetBloom`, and a `Bloom` toggle plus Exposure,
  Bloom Threshold and Bloom Strength sliders in the config menu. The defaults
  (threshold 1.1, strength 0.35) are deliberately restrained; the chain was verified by
  running it at threshold 0.15 and strength 3.0, where the lit floor glows
  unmistakably, then setting the constants back.

**#28a Emissive voxel textures `[x]`** — added so bloom can be tested against an
actual glowing surface rather than only against a blown-out flashlight.

The emissive flag rides in the top bit of each `uint16` entry of the voxel lookup
buffer (`EmissiveTextureBit`, `Map.withEmissiveBit`). Texture indices are a few hundred
at most, so the bit was free: no new buffer, no vertex format change, no mesher change,
and emissive stays per face, which is the resolution greedy meshing leaves anyway. The
cube fragment shader splits the entry back into index and flag with the same two
constants, and `TestEmissiveBitLeavesTextureIndexRecoverable` pins the encoding both
sides depend on.

`finalColor` gained an `emissive` parameter, added after the lighting multiply and
before the fog: a glowing surface does not care how lit it is, but it does still fade
into the distance. The emissive term is the surface's own colour times
`emissiveStrength`, so a marked panel keeps its artwork instead of becoming a flat
bright rectangle.

To use it: look at a wall and press **G**. Every face in the level using that texture
starts glowing, and G again turns it off. Keyed on the texture rather than the block,
so one keypress lights every screen panel of that kind - which is both what you usually
want and what makes it a one-key way to see the bloom threshold working. `Emissive
Strength` is a config slider; `Map.SetEmissiveTextures` is there for a map loader to
call once emissive tiles are authored rather than toggled by hand.

The default strength is 1.5. 3.0 was tried first and the tone map rolled the pipe
panels to flat white - visually striking, but it destroyed the texture, which is the
opposite of what an emissive surface should look like.

**#28b Emissive masked by colour, and the bloom recalibrated `[x]`** — the first cut of
both was wrong in ways that only showed up on screen.

*The mask.* Marking a texture lit the whole tile, which is not what a glowing screen
looks like. It now lights only the texels that are both bright and colourful. Brightness
alone cannot do it on this art: polished grey metal is as bright as a cyan tube, and a
luminance-only mask took 63% of tex 19 instead of its light strips. Lit fittings here are
saturated and the metal is not, so gating on saturation as well scores the grey panels at
zero however bright they are - measured on the wall sheet, the tubes and console readouts
survive and tex 32/38/48 go to exactly 0.00.

*The bloom.* There was no visible bleeding because the numbers were nonsense. An emissive
surface reached 1.2 HDR, the 1.1 threshold took all but 0.1 of it, and the 0.35 strength
divided that by three: a glow of about 0.03. The blur was also far too tight to read as
light. Now: threshold 1.5 and strength 0.6, calibrated against what the scene actually
produces (the static lamps run at intensity 6, so a threshold at white blooms ordinary
lit geometry), the chain runs at quarter resolution instead of half, and each of its
three blur pairs doubles the tap spacing - a poor man's mip chain that reaches roughly
32 texels for six small passes.

*The map.* The one texture the test map had with screens on it (47) was too dark for any
sane cutoff. `assets/base/map_64x64.png` slot 49 - blank filler in the sheet's last row -
now holds a copy of it with the coloured readouts lifted, `assets/bstone_tex.png` gets
the same tile so the LDtk editor is not misleading, and `raymap.ldtk` points its two
console walls at 49. Seeding lives in `Game.Init` next to the map load.

Verified on screen: the console screens read as lit in a dark room with the glow spilling
onto the panel around them, while the metal frame stays dark.

**#28 Screenshots come from the screen `[x]`** — `SaveScreenshot` read the scene
framebuffer, which is now linear HDR; reading it back as bytes would clip every
highlight and skip the tone map. It now reads the default framebuffer, so a screenshot
is the finished image, GUI included.

Verified by running the engine: all shaders compile, the test map renders, and a
`-tags debugGL` run reports no GL errors.

## Open — correctness

### [x] 29. The std140 table conflates size with alignment

`quads/glhf/attr.go:141` — `Std140BaseAlignment()` is used both as the alignment in
`NewUBOWithAttributes` and as the byte count in `SetAttribute`'s `BufferSubData`. The
two only coincide for some types. For `Vec3` it returns 16 against a 12-byte
`mgl32.Vec3`, and for `Mat3` it returns 64 against 36: an out-of-bounds read past the
value and a 28-byte overwrite of whatever member follows it.

Latent, not live — the UBOs in use only contain `Vec2`, `Vec4` and `Mat4`, where the
numbers agree. It becomes a real memory bug the first time someone adds a `Vec3`
uniform to a UBO, which is an easy and reasonable thing to do.

**Fix:** split it into `Std140Alignment()` and `Std140Size()`.

Same file, cosmetic: `NewUBOWithAttributes` prints the whole layout with `println` on
every startup.

### [x] 30. Unsigned underflow in the voxel texture lookup

`quads/shader_cubes.go` — `uint(floor(fragWorldPos.x)) - normOffsetX` wraps to roughly
four billion for a face at grid coordinate 0 with a negative normal, and the
`texelFetch` into `voxelLookup` then reads whatever is there. Invisible today only
because the test map has a solid border, so no face sits at 0. Clamp the grid
coordinate, or do the arithmetic signed and clamp before the cast.

## Open — performance and structure

### [x] 31. The depth-pass shaders are built by string surgery

`prefixFilter` (`quads/shader_quads.go`) builds each depth-pass vertex shader by
deleting lines from the main one whose *text* starts with `"out vec3"`, `"Vert"` and
so on, and each of the three call sites passes a different hand-tuned prefix list.
Rename a varying or reflow a line and the depth shader silently stops matching; the
failure surfaces as a driver compile error at startup, far from the edit.

**Fix:** one source with `#ifdef DEPTH_PASS` and a `#define` prepended for the depth
variant. Shorter than the filter, and it cannot rot.

### [ ] 32. `CULL_FACE` is toggled per sprite

`quads/sys_sprites.go` — `drawSprite` calls `gl.Disable(gl.CULL_FACE)` and
`gl.Enable(gl.CULL_FACE)` around the draw for every static sprite, inside the innermost
loop. Partition the draw list by draw mode and toggle twice per frame instead. Ranks
below #2 (instanced sprites), which would subsume it.

### [x] 33. Blending is enabled for the whole 3D pass

`gl.Enable(gl.BLEND)` is set once at init and never turned off, so every opaque chunk
and mesh fragment goes through the blend unit for nothing. Enable it around the
transparent sprite pass only — which is now its own pass, so there is somewhere
obvious to put it.

**Landed.** Blend is off at init and turned on around three places: the opaque
sprite pass, the transparent sprite pass and the 2D GUI block. Opaque sprites
keep it because the atlases have soft edges and the shader only discards texels
with `a == 0`; chunks and meshes, which are the bulk of the fragments, no longer
touch the blend unit. `renderBloom` no longer has to toggle it.

### [ ] 34. `shadowSceneSignature()` runs twice per frame with the overlay up

Once in the render loop and once via `ShadowCacheState` for the debug rows. Hash once
per frame and hand the debug overlay the value.

### [x] 35. `SetLights` is an empty stub with callers

`quads/gl_application.go` — `func (a *GlApplication) SetLights(infos []LightInfo) {}`.
Callers get silence rather than lights. Related to #14: this is the API the light
placement work needs, and right now it lies about working.

## What the review found in good shape

Recorded so it does not get "cleaned up" by someone who does not know why it is
like that:

- The shared `fragmentShaderSharedFunctions` header, so cubes, meshes and sprites are
  lit by one `finalColor` by construction rather than by three copies kept in sync.
- `UniformBufferObject.GetShaderBlockSource` generating the GLSL block from the Go
  layout: the camera, fog and tone map uniforms cannot drift between the two sides.
- `LightInfo.packed()` reading reach and cone off the light's own shadow frustum, so
  the region a light lights and the region it has depth for cannot diverge.
- The shadow cache keyed on proj-view plus a summed caster hash — summed, not chained,
  because the drawers iterate maps.
- `sampler2DArrayShadow` with 3x3 manual taps: every tap is already hardware 2x2 PCF,
  so this is 6x6 effective coverage for nine fetches.
- Greedy-mesh UVs from `fract(VertPos)`, which tiles a merged quad's texture with no
  per-quad UV data at all.
- The skybox drawn after the opaque geometry with `DepthMask(false)` and `pos.xyww`,
  so it costs only the pixels that are actually sky.
