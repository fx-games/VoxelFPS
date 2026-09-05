package main

import (
	"log"
	"math"
	"math/rand"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
	"github.com/memmaker/goray/wep"
)

// A weapon throws things into the world: muzzle smoke, an ejected shell, a
// rocket. They are actors, compiled the same way weapons are, and they run the
// same stepper. What is different is that nobody is holding them, so they have
// a position, they move, and they stop when their script says to.
//
// Their frames all come out of one atlas, shared by every weapon, so drawing
// forty casings and a rocket at once is one texture and one batch.

// WorldActor is one of those things while it exists.
type WorldActor struct {
	data    *wep.Actor
	cursor  *wep.Cursor
	runtime *wep.Runtime
	object  *quads.GameObject
	sprite  *quads.SpriteComponent
	// physics is only given to what has to hit something: a shell that lands on
	// the floor, a rocket that hits a wall. Smoke drifts and is drawn, and a
	// weapon makes a great deal of it, so it moves by arithmetic here instead
	// of by a body sweeping the voxel world every frame.
	physics  *quads.PhysicsComponent
	velocity mgl32.Vec3
	// scale is the actor's own, on top of the picture's own size.
	scale mgl32.Vec2
	// world is what it asks whether there is ground under it.
	world *voxel.Map
}

// worldUnitsPerDoomUnit converts the numbers a script writes into ours. Doom
// measures in units where a wall panel is 64 and a player is 56 tall; a voxel
// here is one, and a wall panel is one.
const worldUnitsPerDoomUnit = 1.0 / 64.0

// spritePixelsPerUnit is how big a sprite is drawn: a Doom sprite is authored
// against that same 64-unit wall, so its pixels convert the same way.
const spritePixelsPerUnit = 64.0

// spawnWorldActor puts one into the world in front of the player.
func (g *Game) spawnWorldActor(weapon *Weapon, op wep.Op) {
	actor, known := weapon.Actors[lowerName(op.Actor)]
	if !known || actor.Stock || g.sharedAtlas == nil {
		// Nothing compiled to draw: a stock actor the engine is meant to
		// provide, or a folder with no shared pictures in it.
		return
	}
	if _, hasSpawn := actor.States.Label("Spawn"); !hasSpawn {
		return
	}

	origin, direction := g.aim()
	g.spawnActorAt(weapon, actor, op, g.spawnPosition(origin, direction, op))
}

// spawnActorAt is the rest of it, once where has been decided.
func (g *Game) spawnActorAt(weapon *Weapon, actor *wep.Actor, op wep.Op, position mgl32.Vec3) {
	_, direction := g.aim()

	sprite := quads.NewSprite(g.sharedAtlas, []uint16{0})
	sprite.Tint = mgl32.Vec4{1, 1, 1, float32(alphaOf(actor))}
	// Doom sprites never cast, and these move every frame: a hundred drifting
	// puffs that did would have every light redraw its shadow map every frame.
	sprite.NoShadow = true
	if actor.Render.Style == "add" || actor.Render.Style == "additive" {
		// An additive sprite is its own light as far as the bloom pass cares.
		sprite.Emissive = additiveEmissive
	}

	object := quads.NewGameObject("Spawned:" + actor.Name)
	object.Transform().SetPosition(position)
	object.SetComponent(sprite)
	object.SetLifeTime(worldActorLifetime)

	live := &WorldActor{
		data:    actor,
		cursor:  wep.NewCursor(&actor.States, "Spawn"),
		runtime: &wep.Runtime{Inventory: wep.Inventory{}, Caps: wep.Caps{}, Vars: map[string]float64{}, Rand: weapon.runtime.Rand},
		object:  object,
		sprite:  sprite,
		scale: mgl32.Vec2{
			float32(actor.Render.Scale[0]),
			float32(actor.Render.Scale[1]),
		},
		world: g.CubeMap,
	}

	g.Sprites.Add(sprite)
	live.velocity = g.launchVelocity(actor, op, direction)

	if needsCollision(actor) {
		physics := quads.NewPhysicsComponent(false,
			voxel.NewAABB(mgl32.Vec3{-0.05, -0.05, -0.05}, mgl32.Vec3{0.05, 0.05, 0.05}))
		object.SetComponent(physics)
		g.physicsSystem.AddObject(physics)
		physics.SetVelocity(live.velocity)
		physics.ApplyGravity = !actor.NoGravity
		live.physics = physics
	}

	live.showCurrentFrame(g)
	g.worldActors = append(g.worldActors, live)
	g.capWorldActors()
}

// maxWorldActors is how many spawned things may be in the air at once. A
// weapon that fires fast makes several a shot, and while their own scripts do
// end them, a ceiling means a pathological one cannot fill the world.
//
// ponytail: the oldest goes, which is right for smoke and casings and would be
// wrong for a rocket in flight. Nothing distinguishes them yet because nothing
// a rocket does depends on it arriving.
const maxWorldActors = 192

func (g *Game) capWorldActors() {
	for len(g.worldActors) > maxWorldActors {
		g.worldActors[0].remove(g)
		g.worldActors = g.worldActors[1:]
	}
}

// needsCollision says whether a spawned thing has to be stopped by the world.
// Something that falls has to land, and something with a speed of its own is
// going somewhere and can arrive at a wall. Smoke does neither.
func needsCollision(actor *wep.Actor) bool {
	return !actor.NoGravity || actor.Speed > 0
}

// spawnPosition puts it where the script asked, relative to the player: out
// along the line of fire, and up or down by the spawn height.
func (g *Game) spawnPosition(origin, direction mgl32.Vec3, op wep.Op) mgl32.Vec3 {
	sideways := direction.Cross(mgl32.Vec3{0, 1, 0}).Normalize()
	offset := op.SpawnOffset
	if op.SpawnOffsetTo > op.SpawnOffset {
		offset += rand.Float64() * (op.SpawnOffsetTo - op.SpawnOffset)
	}
	return origin.
		Add(direction.Mul(spawnDistanceInFront)).
		Add(sideways.Mul(float32(offset * worldUnitsPerDoomUnit))).
		Add(mgl32.Vec3{0, float32(op.SpawnHeight * worldUnitsPerDoomUnit), 0})
}

// launchVelocity is how fast it leaves. A missile has a speed of its own and
// goes where the player is looking; anything else drifts, so that smoke does
// not sit in a rigid dot.
func (g *Game) launchVelocity(actor *wep.Actor, op wep.Op, direction mgl32.Vec3) mgl32.Vec3 {
	if actor.Speed > 0 {
		angled := direction
		if op.Angle != 0 {
			angled = rotateAroundY(direction, op.Angle)
		}
		return angled.Mul(float32(actor.Speed * worldUnitsPerDoomUnit * wep.TicRate))
	}
	return mgl32.Vec3{
		float32(rand.NormFloat64()) * driftSpeed,
		float32(rand.Float64()) * driftSpeed,
		float32(rand.NormFloat64()) * driftSpeed,
	}
}

func rotateAroundY(direction mgl32.Vec3, degrees float64) mgl32.Vec3 {
	radians := degrees * math.Pi / 180
	sin, cos := float32(math.Sin(radians)), float32(math.Cos(radians))
	return mgl32.Vec3{
		direction.X()*cos - direction.Z()*sin,
		direction.Y(),
		direction.X()*sin + direction.Z()*cos,
	}.Normalize()
}

// warnMissingSprite complains once about a frame the shared atlas does not
// hold, rather than every tic something tries to show it.
func (g *Game) warnMissingSprite(name string) {
	if g.warnedSprites == nil {
		g.warnedSprites = make(map[string]bool)
	}
	if g.warnedSprites[name] {
		return
	}
	g.warnedSprites[name] = true
	log.Printf("[weapons] no picture called %s in %s", name, wep.SharedAtlasFile)
}

// LiveActorCount is how many spawned things are in flight, for the overlay.
func (g *Game) LiveActorCount() int { return len(g.worldActors) }

// updateWorldActors steps everything in flight and clears out what has stopped.
func (g *Game) updateWorldActors(elapsed float64) {
	for i := len(g.worldActors) - 1; i >= 0; i-- {
		live := g.worldActors[i]
		if live.cursor.Stopped() || !live.object.IsAlive() {
			live.remove(g)
			g.worldActors = append(g.worldActors[:i], g.worldActors[i+1:]...)
			continue
		}
		live.runtime.World = live
		live.publishState()
		live.move(g, elapsed)
		for _, op := range live.cursor.Update(live.runtime, elapsed) {
			live.apply(g, op)
		}
		live.showCurrentFrame(g)
	}
}

// publishState puts the actor's own properties where a shipped expression can
// read them. A smoke puff shrinks itself with `scalex * frandom(...)`, so what
// scalex is right now decides how the animation looks.
func (live *WorldActor) publishState() {
	velocity := live.currentVelocity()
	position := live.object.Transform().GetPosition()
	set := live.runtime.SetVar
	set("scalex", float64(live.scale.X()))
	set("scaley", float64(live.scale.Y()))
	set("scale", float64(live.scale.X()))
	set("alpha", float64(live.sprite.Tint.W()))
	// Doom's x and y are the ground plane and z is up; ours has y up.
	set("velx", float64(velocity.X())/worldUnitsPerDoomUnit/wep.TicRate)
	set("vely", float64(velocity.Z())/worldUnitsPerDoomUnit/wep.TicRate)
	set("velz", float64(velocity.Y())/worldUnitsPerDoomUnit/wep.TicRate)
	set("momx", live.runtime.Vars["velx"])
	set("momy", live.runtime.Vars["vely"])
	set("momz", live.runtime.Vars["velz"])
	set("z", float64(position.Y())/worldUnitsPerDoomUnit)
	set("height", float64(live.sprite.Size.Y())/worldUnitsPerDoomUnit)
}

// move carries along whatever has no body of its own: gravity if it falls, and
// then the step.
func (live *WorldActor) move(g *Game, elapsed float64) {
	if live.physics != nil {
		return // its body is doing this, and stopping it at walls
	}
	if !live.data.NoGravity {
		live.velocity = live.velocity.Add(mgl32.Vec3{0, float32(-gravity * elapsed), 0})
	}
	transform := live.object.Transform()
	transform.SetPosition(transform.GetPosition().Add(live.velocity.Mul(float32(elapsed))))
}

// Below answers A_CheckFloor: whether there is solid ground right under it,
// which is how a shell casing knows to stop bouncing and lie still.
func (live *WorldActor) Below() bool {
	position := live.object.Transform().GetPosition()
	return live.solidAt(position.Add(mgl32.Vec3{0, -groundProbe, 0}))
}

// Above answers A_CheckCeiling.
func (live *WorldActor) Above() bool {
	position := live.object.Transform().GetPosition()
	return live.solidAt(position.Add(mgl32.Vec3{0, groundProbe, 0}))
}

func (live *WorldActor) solidAt(position mgl32.Vec3) bool {
	if live.world == nil {
		return false
	}
	grid := voxel.PositionToGridInt3(position)
	return live.world.IsSolidBlockAt(grid.X, grid.Y, grid.Z)
}

// The rest of what a script can ask about its surroundings needs monsters,
// which this game has none of, so it answers the way an empty world does.
func (live *WorldActor) CanSee() bool               { return false }
func (live *WorldActor) TargetWithin(float64) bool  { return false }
func (live *WorldActor) TargetHealthBelow(int) bool { return false }
func (live *WorldActor) TargetHas(string, int) bool { return false }
func (live *WorldActor) HasFlag(name string) bool   { return false }

// groundProbe is how far under something to look for a floor, and gravity is
// how fast what has no body of its own falls.
const (
	groundProbe = 0.08
	gravity     = 9.8
)

// showCurrentFrame points the sprite at whatever the script is showing, and
// sizes the quad to the picture, which a packed atlas needs because its frames
// are all different.
func (live *WorldActor) showCurrentFrame(g *Game) {
	step := live.cursor.Step()
	if step == nil || step.Sprite == "" {
		live.sprite.Hide()
		return
	}
	index, known := g.sharedSpriteIndex[step.Sprite]
	if !known {
		// Loading checks every frame a compiled actor names, so this is a
		// weapons folder somebody has edited. Said once per frame name rather
		// than once per tic.
		g.warnMissingSprite(step.Sprite)
		live.sprite.Hide()
		return
	}
	live.sprite.Show()
	live.sprite.TextureIndices = []uint16{uint16(index)}
	left, top, right, bottom := g.sharedAtlas.GetUVInPx(index)
	live.sprite.Size = mgl32.Vec2{
		float32(right-left) / spritePixelsPerUnit * live.scale.X(),
		float32(bottom-top) / spritePixelsPerUnit * live.scale.Y(),
	}
}

// apply is what one of these asks the world for. It is a smaller set than a
// weapon's: nobody is holding it, so there is no trigger, no ammo and no view.
func (live *WorldActor) apply(g *Game, op wep.Op) {
	switch op.Op {
	case wep.OpPlaySound:
		g.playSoundAt(op, live.object.Transform().GetPosition())
	case wep.OpExplode:
		g.explodeAt(live.object.Transform().GetPosition(), op)
	case wep.OpSetAlpha:
		if values := live.argsOf(op); len(values) > 0 {
			live.sprite.Tint = mgl32.Vec4{1, 1, 1, float32(values[0])}
		}
	case wep.OpFadeTo:
		live.fade(op)
	case wep.OpSetScale:
		if values := live.argsOf(op); len(values) >= 2 {
			live.scale = mgl32.Vec2{float32(values[0]), float32(values[1])}
		}
	case wep.OpChangeVelocity:
		if values := live.argsOf(op); len(values) >= 3 {
			// Doom's y is our z, and its z is our up.
			live.setVelocity(mgl32.Vec3{
				float32(values[0] * worldUnitsPerDoomUnit * wep.TicRate),
				float32(values[2] * worldUnitsPerDoomUnit * wep.TicRate),
				float32(values[1] * worldUnitsPerDoomUnit * wep.TicRate),
			})
		}
	case wep.OpScaleVelocity:
		if values := live.argsOf(op); len(values) > 0 {
			live.setVelocity(live.currentVelocity().Mul(float32(values[0])))
		}
	case wep.OpSpawnActor, wep.OpSpawnItem, wep.OpFireProjectile:
		// Something a spawned thing spawns in turn: a rocket's smoke trail.
		// It goes where this one is rather than where the player is looking.
		g.spawnFrom(live, op)
	case wep.OpRemove, wep.OpDie:
		live.cursor.Goto("@enginestop")
		live.object.Kill()
	}
}

// argsOf works out an op's numbers against this actor's own state.
func (live *WorldActor) argsOf(op wep.Op) []float64 {
	return op.ArgValues(live.runtime, live.runtime.Rand)
}

// fade walks the sprite's alpha towards where the script wants it.
func (live *WorldActor) fade(op wep.Op) {
	values := live.argsOf(op)
	if len(values) < 2 {
		return
	}
	target, step := float32(values[0]), float32(values[1])
	alpha := live.sprite.Tint.W()
	if alpha < target {
		alpha = min32(target, alpha+step)
	} else {
		alpha = max32(target, alpha-step)
	}
	live.sprite.Tint = mgl32.Vec4{1, 1, 1, alpha}
}

func (live *WorldActor) currentVelocity() mgl32.Vec3 {
	if live.physics != nil {
		return live.physics.Velocity()
	}
	return live.velocity
}

func (live *WorldActor) setVelocity(velocity mgl32.Vec3) {
	live.velocity = velocity
	if live.physics != nil {
		live.physics.SetVelocity(velocity)
	}
}

func (live *WorldActor) remove(g *Game) {
	live.object.Kill()
	live.sprite.Hide()
}

// How long a spawned thing may hang around whatever its script says, how far in
// front of the player it appears, how fast smoke drifts, and how far past white
// an additive sprite is drawn.
const (
	worldActorLifetime   = 8.0
	spawnDistanceInFront = 0.5
	driftSpeed           = float32(0.35)
	additiveEmissive     = float32(3)
)

func alphaOf(actor *wep.Actor) float64 {
	if actor.Render.Alpha <= 0 || actor.Render.Alpha > 1 {
		return 1
	}
	return actor.Render.Alpha
}

func lowerName(name string) string {
	out := []byte(name)
	for i := range out {
		if out[i] >= 'A' && out[i] <= 'Z' {
			out[i] += 'a' - 'A'
		}
	}
	return string(out)
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// sharedAtlasIndex maps a frame name to where it is in the shared atlas.
func sharedAtlasIndex(atlas *glhf.TextureAtlasPacked) map[string]int {
	indices := make(map[string]int)
	for i := 0; i <= atlas.LastValidIndex(); i++ {
		indices[atlas.NameOfFrame(i)] = i
	}
	return indices
}
