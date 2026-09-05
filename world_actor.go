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
	// velocity is how it is moving. A spawned thing carries no physics body:
	// one would sweep the voxel world every frame for every puff of smoke, and
	// worse, it would be something the player bumps into. A shell casing is
	// scenery, and scenery does not block a doorway.
	velocity mgl32.Vec3
	// resting is set once it has landed and stopped moving.
	resting bool
	// seeking is where a homing missile is trying to get to, and homing says
	// whether it is trying.
	homing   bool
	seekTo   mgl32.Vec3
	seekRate float64
	// scale is the actor's own, on top of the picture's own size.
	scale mgl32.Vec2
	// world is what it asks whether there is ground under it.
	world *voxel.Map
	// master is what spawned it, when something did: a rocket's smoke trail
	// hangs off the rocket.
	master *WorldActor
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

// spawnPuffAt draws a hitscan's own puff exactly where it says: a spark, a
// splinter of wood, whatever the mod's own actor animates - in place of the
// engine's generic bullet hole. Reports whether it found one to draw.
func (g *Game) spawnPuffAt(weapon *Weapon, actorName string, pos, surfaceNormal mgl32.Vec3) *WorldActor {
	if weapon == nil || actorName == "" || g.sharedAtlas == nil {
		return nil
	}
	actor, known := weapon.Actors[lowerName(actorName)]
	if !known || actor.Stock {
		return nil
	}
	if _, hasSpawn := actor.States.Label("Spawn"); !hasSpawn {
		return nil
	}
	// Sat a hair off the surface it marks, facing along its normal, so it
	// does not fight the wall or floor it is drawn on for depth.
	return g.spawnActorAt(weapon, actor, wep.Op{}, pos.Add(surfaceNormal.Mul(0.01)))
}

// setMaster records what spawned this, so a warp knows what to sit on.
func (live *WorldActor) setMaster(master *WorldActor) {
	if live != nil {
		live.master = master
	}
}

// spawnActorAt is the rest of it, once where has been decided.
func (g *Game) spawnActorAt(weapon *Weapon, actor *wep.Actor, op wep.Op, position mgl32.Vec3) *WorldActor {
	_, direction := g.aim()

	sprite := quads.NewSprite(g.sharedAtlas, []uint16{0})
	sprite.Tint = mgl32.Vec4{1, 1, 1, float32(alphaOf(actor))}
	// Doom sprites never cast, and these move every frame: a hundred drifting
	// puffs that did would have every light redraw its shadow map every frame.
	sprite.NoShadow = true
	if actor.Render.Style == "add" || actor.Render.Style == "additive" {
		// An additive sprite is its own light as far as the bloom pass cares.
		// Scaled by its own alpha: a script that draws its smoke at 0.06 alpha
		// wants a haze, not the same bloom a full-strength muzzle flash gets.
		sprite.Emissive = additiveEmissive * float32(alphaOf(actor))
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

	live.showCurrentFrame(g)
	g.worldActors = append(g.worldActors, live)
	g.capWorldActors()
	return live
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
		// A Stop just freezes the animation; in GZDoom the actor itself keeps
		// falling until it touches something, which is what sends a missile-
		// flagged casing on to Death. Only drop it once it has both stopped
		// animating and come to rest, or its own lifetime runs out.
		if (live.cursor.Stopped() && live.resting) || !live.object.IsAlive() {
			live.remove(g)
			g.worldActors = append(g.worldActors[:i], g.worldActors[i+1:]...)
			continue
		}
		live.runtime.World = live
		live.publishState()
		live.move(g, elapsed)
		if !live.cursor.Stopped() {
			for _, op := range live.cursor.Update(live.runtime, elapsed) {
				live.apply(g, op)
			}
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

// move carries a spawned thing along: what falls falls, what homes turns
// towards where it is going, and everything stops at the first solid thing it
// meets rather than passing through the world.
func (live *WorldActor) move(g *Game, elapsed float64) {
	if live.resting {
		return
	}
	if live.homing {
		live.steerTowards(live.seekTo, elapsed)
	}
	if !live.data.NoGravity {
		live.velocity = live.velocity.Add(mgl32.Vec3{0, float32(-gravity * elapsed), 0})
	}

	transform := live.object.Transform()
	from := transform.GetPosition()
	to := from.Add(live.velocity.Mul(float32(elapsed)))

	if live.solidAt(to) {
		// It has arrived somewhere solid. A shell settles just above the
		// surface and lies there; anything with a speed of its own has hit
		// something, and its script decides what that means.
		live.velocity = mgl32.Vec3{}
		live.resting = true
		live.homing = false
		if live.data.Speed > 0 || live.data.Missile {
			live.hitSomething(g, from)
		}
		return
	}
	transform.SetPosition(to)
}

// hitSomething sends a missile to its Death state, which is where a script puts
// the explosion.
func (live *WorldActor) hitSomething(g *Game, at mgl32.Vec3) {
	if live.cursor.Goto("Death") {
		return
	}
	if live.cursor.Goto("XDeath") {
		return
	}
	live.object.Kill()
}

// steerTowards turns a homing missile's heading towards a point, by no more
// than its turn rate, which is what A_SeekerMissile does.
func (live *WorldActor) steerTowards(target mgl32.Vec3, elapsed float64) {
	live.velocity = steer(live.velocity,
		live.object.Transform().GetPosition(), target, live.seekRate*elapsed)
}

// steer turns a velocity towards a point without changing how fast it is going,
// by at most maxTurn radians. A missile that could turn as far as it liked
// would arrive in a straight line and there would be nothing to watch.
func steer(velocity, from, target mgl32.Vec3, maxTurn float64) mgl32.Vec3 {
	speed := velocity.Len()
	wanted := target.Sub(from)
	if speed < 0.0001 || wanted.Len() < 0.0001 || maxTurn <= 0 {
		return velocity
	}
	wanted = wanted.Normalize()
	heading := velocity.Mul(1 / speed)

	angle := math.Acos(float64(clampUnitF(heading.Dot(wanted))))
	if angle <= 0.0001 {
		return wanted.Mul(speed) // already pointed at it
	}
	if angle <= maxTurn {
		return wanted.Mul(speed) // near enough to finish the turn this step
	}

	// Turned about the axis the two headings share, by exactly the angle
	// allowed. Mixing the two directions and normalising instead would turn
	// short of it, by more the further apart they are, which makes a missile
	// with a long way to come round turn slower than its script says.
	axis := heading.Cross(wanted)
	if axis.Len() < 0.0001 {
		// Pointed the opposite way, so any axis across the heading will do.
		axis = heading.Cross(mgl32.Vec3{0, 1, 0})
		if axis.Len() < 0.0001 {
			axis = heading.Cross(mgl32.Vec3{1, 0, 0})
		}
	}
	turned := mgl32.QuatRotate(float32(maxTurn), axis.Normalize()).Rotate(heading)
	return turned.Normalize().Mul(speed)
}

func clampUnitF(v float32) float32 {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
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
	case wep.OpSeekerMissile:
		live.startSeeking(g, op)
	case wep.OpWarp:
		live.warp(g, op)
	case wep.OpRemove, wep.OpDie:
		live.cursor.Goto("@enginestop")
		live.object.Kill()
	}
}

// argsOf works out an op's numbers against this actor's own state.
func (live *WorldActor) argsOf(op wep.Op) []float64 {
	return op.ArgValues(live.runtime, live.runtime.Rand)
}

// startSeeking points a missile at something and says how sharply it may turn.
// A_SeekerMissile(threshold, maxturn, ...) writes both in ZDoom's angle units,
// where a whole turn is 256.
func (live *WorldActor) startSeeking(g *Game, op wep.Op) {
	values := live.argsOf(op)
	maxTurn := defaultSeekDegrees
	if len(values) >= 2 && values[1] > 0 {
		maxTurn = values[1] * degreesPerZDoomAngle
	}
	live.homing = true
	live.seekRate = maxTurn * wep.TicRate * math.Pi / 180
	live.seekTo = g.seekTarget()
}

// warp moves something to sit on whatever it belongs to. A script uses it to
// hang an effect off its master; with nothing to hang off, the player is what
// it came from, so that is where it goes.
func (live *WorldActor) warp(g *Game, op wep.Op) {
	values := live.argsOf(op)
	anchor := g.muzzlePosition()
	if live.master != nil && live.master.object.IsAlive() {
		anchor = live.master.object.Transform().GetPosition()
	}
	offset := mgl32.Vec3{}
	if len(values) >= 3 {
		// Doom's x and y are the ground plane and its z is up.
		offset = mgl32.Vec3{
			float32(values[0] * worldUnitsPerDoomUnit),
			float32(values[2] * worldUnitsPerDoomUnit),
			float32(values[1] * worldUnitsPerDoomUnit),
		}
	}
	live.object.Transform().SetPosition(anchor.Add(offset))
	live.resting = false
}

// How sharply a missile turns when the script does not say, and what one of
// ZDoom's angle units is worth: it counts a whole turn as 256.
const (
	defaultSeekDegrees   = 2.0
	degreesPerZDoomAngle = 360.0 / 256.0
)

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

func (live *WorldActor) currentVelocity() mgl32.Vec3 { return live.velocity }

func (live *WorldActor) setVelocity(velocity mgl32.Vec3) {
	live.velocity = velocity
	live.resting = velocity.Len() < 0.0001 && live.resting
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
