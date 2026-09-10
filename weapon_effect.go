package main

import (
	"log"
	"math"
	"math/rand"
	"os"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
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
	data    *wep.Effect
	cursor  *wep.Cursor
	runtime *wep.Runtime
	object  *quads.GameObject
	sprite  *quads.SpriteComponent
	// velocity is how it is moving. A spawned thing carries no physics body:
	// one would sweep the voxel world every frame for every puff of smoke, and
	// worse, it would be something the player bumps into. A shell casing is
	// scenery, and scenery does not block a doorway.
	velocity mgl32.Vec3
	// facing is which way it is pointed, on the floor plane, which is what
	// decides which of a frame's eight views is drawn. ZDoom keeps it as the
	// actor's angle: set when the thing is launched and kept afterwards, so a
	// casing that has come to rest still points the way it was thrown rather
	// than snapping to whatever the last frame's velocity happened to be.
	facing mgl32.Vec2
	// resting is set once it has landed and stopped moving, and dead once
	// something has killed it - A_Die does nothing to a thing that is already
	// dead, which is what lets a Death state run past its own A_Die.
	resting bool
	dead    bool
	// drawn is set once the thing has been through a frame, which is what
	// makes the point blank test below a first-frame test and nothing else.
	drawn bool
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
	// owner is the weapon that put this into the world, which for anything a
	// weapon spawns is ZDoom's target pointer: the player who fired it. A
	// thrown charge tells its owner that it exists and watches the owner's
	// inventory for the word to go off.
	owner *wep.Runtime
	// origin is what this thing's sound channels belong to. A ZDoom channel is
	// the origin's channel, not the world's: a puff playing a ricochet on its
	// own CHAN_WEAPON must not stop the gun that spawned it playing its shot
	// on the player's. Zero is the player, so these start at one.
	origin int
	// soundScope is the weapon that spawned this, whose sounds it plays -
	// see cueKey in the audio package. A rocket plays its own cues by the
	// launcher's ID even once the launcher is no longer the weapon in hand.
	soundScope string
}

// worldUnitsPerDoomUnit converts the numbers a script writes into ours. Doom
// measures in units where a wall panel is 64 and a player is 56 tall; a voxel
// here is one, and a wall panel is one.
const worldUnitsPerDoomUnit = 1.0 / 64.0

// debugActors makes every spawned effect say what it drew and how, once a
// frame. What a muzzle smoke or a shell casing looks like is decided by five
// numbers that come out of five different places - the picture, the actor's
// scale, its alpha, its render style and the light where it stands - and a
// screenshot of it looking wrong says which of them, only after you can see
// all five. DEBUG_ACTORS=1 in the environment.
var debugActors = os.Getenv("DEBUG_ACTORS") != ""

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

	g.spawnActorAt(weapon, actor, op, g.spawnPosition(op), "Spawn", weapon.runtime)
}

// spawnPuffAt draws a hitscan's own puff exactly where it says: a spark, a
// splinter of wood, whatever the mod's own actor animates. It is drawn as well
// as the bullet hole, not instead of it - ZDoom spawns the puff and the decal
// from the same hit and neither knows about the other.
//
// A puff that hit no actor starts in its Crash state when it has one, which is
// P_SpawnPuff's own rule and not a detail: Final Doomer's puff answers its
// Spawn state by playing a flesh-hit sound and showing nothing, and its wall
// hit - the sparks, the ricochet, the little cloud of smoke - is all in Crash.
// Started in Spawn on a wall, every hitscan in that mod landed invisibly.
func (g *Game) spawnPuffAt(weapon *Weapon, actorName string, pos, surfaceNormal mgl32.Vec3) *WorldActor {
	if weapon == nil || actorName == "" || g.sharedAtlas == nil {
		return nil
	}
	actor, known := weapon.Actors[lowerName(actorName)]
	if !known || actor.Stock {
		return nil
	}
	label := "Spawn"
	if _, hasCrash := actor.States.Label("Crash"); hasCrash {
		label = "Crash"
	} else if _, hasSpawn := actor.States.Label("Spawn"); !hasSpawn {
		return nil
	}
	// Sat a hair off the surface it marks, facing along its normal, so it
	// does not fight the wall or floor it is drawn on for depth.
	return g.spawnActorAt(weapon, actor, wep.Op{}, pos.Add(surfaceNormal.Mul(0.01)), label, weapon.runtime)
}

// setMaster records what spawned this, so a warp knows what to sit on.
func (live *WorldActor) setMaster(master *WorldActor) {
	if live != nil {
		live.master = master
	}
}

// spawnJitterStep and spawnJitterCycle turn a spawn's own sequence number into
// a few centimetres of vertical offset - enough to break an exact tie, small
// enough that nobody notices a puff of smoke sitting a hair off where it was
// asked to be.
//
// What matters is the difference between two consecutive spawns, not the
// distance either one ends up from its script's own position, so the offsets
// straddle zero and the cycle is short: the whole range is a centimetre either
// side, under a single Doom unit, and it stays under two even when a nested
// spawn carries its parent's offset as well as its own. A long cycle counted
// from zero instead put every spawn up to 22 steps - fourteen Doom units, a
// quarter of the player's height - above where it was asked to be, and the
// nesting stacked on top of that: a gun's muzzle smoke, spawned by a spawner
// that was itself already lifted, drew a clear distance over the barrel and
// jumped from shot to shot as the counter advanced.
const (
	spawnJitterStep  = float32(0.005)
	spawnJitterCycle = 7 // coprime with the small counts effects spawn in, e.g. 5
)

// spawnJitter is why two effects that read the same X, Y, Z off the same
// script call - a frag grenade's own fireball and the BarrelExplosionSmokeColumn
// it spawns at the same instant - do not end up bit-identical in world space.
// GZDoom's software renderer sorts things and draws them with no shared depth
// buffer at all, so two Doom things standing on the same spot never compete for
// the same pixel the way two textured quads sharing a Z buffer do; ZDoom's own
// hardware renderer additionally biases every masked sprite by the same fixed
// offset (see spriteDepthBias), which moves a sprite off the wall behind it but
// does nothing to separate two sprites tied with each other, because both get
// the identical bias. This engine's polygon-offset tiers (additive, opaque,
// translucent - see sys_sprites.go) settle most such ties, but two quads at the
// literal same point are still coincident in view space, and depth-buffer
// precision is coarser the farther out that view space point is: a few dozen
// polygon-offset units that reliably separate two ties up close can still
// leave them interleaving in a fine Moire pattern ten-odd units out, which is
// what a screenshot of exactly this pairing showed. A real, if tiny, position
// difference sidesteps the precision question entirely instead of trying to
// out-guess it.
func spawnJitter(position mgl32.Vec3, sequence int) mgl32.Vec3 {
	step := sequence%spawnJitterCycle - spawnJitterCycle/2
	return position.Add(mgl32.Vec3{0, spawnJitterStep * float32(step), 0})
}

// spawnActorAt is the rest of it, once where has been decided. The label is
// where the new thing's script starts, which is Spawn for everything but a
// puff that landed on a wall.
// spawner is whatever ran the call, so a velocity written as `velx` reads off
// the thing that did the spawning rather than off nothing at all.
func (g *Game) spawnActorAt(weapon *Weapon, actor *wep.Effect, op wep.Op, position mgl32.Vec3, label string, spawner *wep.Runtime) *WorldActor {
	_, direction := g.aim()

	sprite := quads.NewSprite(g.sharedAtlas, []uint16{0})
	sprite.Tint = mgl32.Vec4{1, 1, 1, float32(alphaOf(actor))}
	// Doom sprites never cast, and these move every frame: a hundred drifting
	// puffs that did would have every light redraw its shadow map every frame.
	sprite.NoShadow = true
	if actor.Render.Style == "add" || actor.Render.Style == "additive" {
		sprite.Additive = true
	}

	g.lastActorOrigin++
	object := quads.NewGameObject("Spawned:" + actor.Name)
	object.Transform().SetPosition(spawnJitter(position, g.lastActorOrigin))
	object.SetComponent(sprite)
	object.SetLifeTime(worldActorLifetime)

	live := &WorldActor{
		origin:     g.lastActorOrigin,
		soundScope: weapon.Data.ID,
		data:       actor,
		cursor:     wep.NewCursor(&actor.States, label),
		runtime:    &wep.Runtime{Inventory: wep.Inventory{}, Caps: wep.Caps{}, Vars: map[string]float64{}, Rand: weapon.runtime.Rand},
		object:     object,
		sprite:     sprite,
		scale: mgl32.Vec2{
			float32(actor.Render.Scale[0]),
			float32(actor.Render.Scale[1]),
		},
		world: g.CubeMap,
		owner: weapon.runtime,
	}

	g.Sprites.Add(sprite)
	live.velocity = g.launchVelocity(actor, op, direction, spawner)
	live.face(live.velocity, mgl32.Vec2{direction.X(), direction.Z()})

	live.showCurrentFrame(g)
	// ZDoom plays a missile's SeeSound from P_SpawnMissile, which is why it is
	// on the actor and in none of its states. Only for a missile actually
	// fired: A_SpawnItem drops a thing into the world and makes no such noise.
	if op.Op == wep.OpFireProjectile && actor.SeeSound != "" {
		g.playSoundAt(wep.Op{Sound: actor.SeeSound, Volume: 1, Pitch: 1, Channel: seeSoundChannel},
			position, live.origin, live.soundScope)
	}
	g.worldActors = append(g.worldActors, live)
	g.capWorldActors()
	return live
}

// seeSoundChannel is where a missile's launch noise goes. ZDoom uses
// CHAN_VOICE for it, which is channel 2, and it is the missile's own channel
// rather than the player's - a stream of plasma balls each keep their own.
const seeSoundChannel = 2

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

// spawnPosition puts a spawned thing where the weapon that made it is
// pointing. The offsets are ZDoom's own - a forward reach, a sideways one and
// a height above the player's feet - but they are measured along the line of
// aim rather than along a flat facing with a world-vertical height, so what a
// weapon throws out rides the gun up and down.
//
// ZDoom rotates them by the yaw alone, and a muzzle flash spawned that way
// hangs at one fixed height whatever the player is looking at: fire at the
// floor and the flash stays up at chest height, well above the barrel it is
// meant to be leaving. That is faithful and it looks broken from behind a gun
// this engine draws with a pitch.
//
// The two families still differ in what they measure from. A_SpawnItem and
// A_SpawnItemEx place a thing outright, up from the player's feet. A missile -
// A_FireCustomMissile, A_CustomMissile - has no forward offset, because it
// leaves from where the shooter stands and gets anywhere under its own speed;
// its height is measured from the middle of the shooter rather than the feet.
//
// Getting the offsets themselves wrong is not subtle. Everything used to be
// put half a voxel in front of the eye whatever it asked for, so the
// chainsaw's idle exhaust - two puffs of smoke a second, each about a voxel
// across, which in ZDoom leave from inside the player and drift away behind
// him - hung in front of the camera instead and filled the screen with grey.
func (g *Game) spawnPosition(op wep.Op) mgl32.Vec3 {
	_, aim := g.aim()

	height := float32(op.SpawnHeight * worldUnitsPerDoomUnit)
	if op.Op != wep.OpSpawnItem {
		height += playerCenterHeight + attackOffset
	}
	eye := g.playerFeet().Add(mgl32.Vec3{0, playerEyeHeight, 0})
	return eye.Add(aimOffset(aim,
		float32(rollRange(op.SpawnForward, op.SpawnForwardTo)*worldUnitsPerDoomUnit),
		float32(rollRange(op.SpawnOffset, op.SpawnOffsetTo)*worldUnitsPerDoomUnit),
		height-playerEyeHeight))
}

// aimOffset is a spawn's three offsets as one step from the eye: forward along
// the line of aim, sideways to its right, and up across the two. Measured that
// way rather than along a flat facing, a height stays a height above the gun
// however the gun is pointed.
func aimOffset(aim mgl32.Vec3, forward, sideways, up float32) mgl32.Vec3 {
	across := aim.Cross(mgl32.Vec3{0, 1, 0})
	if across.Len() < 0.001 {
		// Straight up or down, where there is no right hand side to speak of.
		across = aim.Cross(mgl32.Vec3{0, 0, 1})
	}
	right := across.Normalize()
	return aim.Mul(forward).
		Add(right.Mul(sideways)).
		Add(right.Cross(aim).Normalize().Mul(up))
}

// playerFeet is where the player stands, which is what ZDoom measures a spawn
// from. The camera sits an eye height above it.
func (g *Game) playerFeet() mgl32.Vec3 {
	if g.player == nil {
		return g.Camera.GetPosition().Sub(mgl32.Vec3{0, playerEyeHeight, 0})
	}
	return g.player.GameObject.Transform().GetPosition()
}

// playerEyeHeight is how far above its own feet the player looks out, and
// playerCenterHeight is the middle of it, which is where ZDoom launches a
// missile from. Doom's player is 56 units tall and this one is 0.9 of a voxel,
// which is the same player.
const (
	playerEyeHeight    = float32(41 * worldUnitsPerDoomUnit)
	playerHeight       = float32(56 * worldUnitsPerDoomUnit)
	playerCenterHeight = playerHeight / 2
	// attackOffset is what ZDoom adds on top of the middle before it launches
	// a missile: AttackZOffset is 8 and P_SpawnPlayerMissile takes 4 back off
	// it. Small, and it is the difference between a tracer that leaves level
	// with the gun and one that leaves visibly under it.
	attackOffset = float32(4 * worldUnitsPerDoomUnit)
)

// launchVelocity is how fast it leaves. A missile has a speed of its own and
// goes where the player is looking; anything else drifts, so that smoke does
// not sit in a rigid dot.
func (g *Game) launchVelocity(actor *wep.Effect, op wep.Op, direction mgl32.Vec3, spawner *wep.Runtime) mgl32.Vec3 {
	if velocity, ok := explicitVelocity(op, direction, spawner); ok {
		return velocity
	}
	// Speed is a missile's speed, and only a missile is launched with it.
	// P_SpawnMissile works the velocity out from Speed along the line of fire;
	// A_SpawnItem and A_SpawnItemEx call none of that - they put an actor
	// somewhere and leave it there, and any velocity it has is the one the
	// call wrote out itself. Handing every spawn its Speed set Brutal Doom's
	// BigBlackSmoke drifting: it is `Speed 1` with +MISSILE on it, spawned by
	// A_SpawnItemEx where a flame lands, and the last cloud over the scorch
	// mark wandered a voxel and a half off it instead of sitting still.
	if actor.Speed > 0 && op.Op == wep.OpFireProjectile {
		heading := pitchedBy(turnedBy(direction, rollRange(op.Angle, op.AngleTo)),
			rollRange(op.AimPitch, op.AimPitchTo), op.AbsolutePitch)
		return heading.Mul(float32(actor.Speed * worldUnitsPerDoomUnit * wep.TicRate))
	}
	// Nothing said to move it, so it does not move. ZDoom spawns a thing where
	// it is told and leaves it there; the random drift that used to be here
	// meant a muzzle flash wandered away from the barrel it belongs to, and it
	// was also what kept every one of them from ever coming to rest.
	return mgl32.Vec3{}
}

// explicitVelocity is A_SpawnItemEx's own xvel/yvel/zvel: a real push the
// script hands the spawn, rather than the drift or facing-speed every other
// spawner falls back to. Each axis is rolled once here the same way
// SpawnOffset already is, not folded to a fixed number at migration time, so
// a scatter of pickups actually scatters. X is along the direction it
// spawned facing, Y sideways across it, Z straight up.
//
// The call's own angle turns that pair, exactly as it turns the position
// offsets - ZDoom rotates both by the same angle, and a spawn that says
// "seven units a tic, ninety two degrees round" means seven units a tic
// sideways. Dropping the angle here left every ejected shell casing flying
// straight down the line of fire to land in front of the player.
func explicitVelocity(op wep.Op, direction mgl32.Vec3, spawner *wep.Runtime) (mgl32.Vec3, bool) {
	forward, sideways, up, written := spawnVelocity(op, spawner)
	if !written {
		return mgl32.Vec3{}, false
	}
	var velocity mgl32.Vec3
	if op.AbsoluteVelocity {
		// SXF_ABSOLUTEVELOCITY: the numbers are Doom's own world axes, not
		// forward and sideways from a facing. Doom's x and y are the ground
		// plane and its z is up; ours has y up, which is the mapping the
		// spawner's own velx/vely/velz were published under, so reading them
		// back this way hands a spawn exactly the throw it inherited.
		velocity = mgl32.Vec3{float32(forward), float32(up), float32(sideways)}
	} else {
		heading := turnedBy(direction, rollRange(op.Angle, op.AngleTo))
		side := heading.Cross(mgl32.Vec3{0, 1, 0}).Normalize()
		velocity = heading.Mul(float32(forward)).
			Add(side.Mul(float32(sideways))).
			Add(mgl32.Vec3{0, float32(up), 0})
	}
	return velocity.Mul(float32(worldUnitsPerDoomUnit * wep.TicRate)), true
}

// spawnVelocity is the three numbers themselves: the call's own expressions
// when it wrote any, read against the actor doing the spawning, and the rolled
// ranges otherwise. A call that wrote nothing at all reports so, because a
// spawn with no velocity of its own is left to whatever launchVelocity decides
// rather than pinned to a standstill.
func spawnVelocity(op wep.Op, spawner *wep.Runtime) (forward, sideways, up float64, written bool) {
	if len(op.VelExprs) == 3 && spawner != nil {
		forward = op.VelExprs[0].Eval(spawner, spawner.Rand)
		sideways = op.VelExprs[1].Eval(spawner, spawner.Rand)
		up = op.VelExprs[2].Eval(spawner, spawner.Rand)
		return forward, sideways, up, forward != 0 || sideways != 0 || up != 0
	}
	if op.VelX == 0 && op.VelXTo == 0 && op.VelY == 0 && op.VelYTo == 0 && op.VelZ == 0 && op.VelZTo == 0 {
		return 0, 0, 0, false
	}
	return rollRange(op.VelX, op.VelXTo), rollRange(op.VelY, op.VelYTo), rollRange(op.VelZ, op.VelZTo), true
}

// rollRange picks a value the way a compiled frandom(min, max) is meant to:
// once per call, not once at migration time. A plain number compiles with
// its "to" equal to itself, so this still returns it exactly.
func rollRange(min, max float64) float64 {
	if max > min {
		return min + rand.Float64()*(max-min)
	}
	return min
}

// turnedBy swings a heading sideways by a DECORATE angle, about the world's
// own up, so a pitch survives the turn the way ZDoom's does.
//
// The sign is the whole of it. ZDoom counts an angle anticlockwise, so a
// positive one turns to the spawner's left - which is the opposite of a
// positive sideways offset, and that inversion is deliberate on ZDoom's part.
// Brutal Doom says which is which in its own words: `50CaseSpawn2 //right side
// eject` throws its brass with `random(-120,-140)`, and the left handed gun of
// the same pair uses `random(120,140)`.
func turnedBy(direction mgl32.Vec3, degrees float64) mgl32.Vec3 {
	if degrees == 0 {
		return direction
	}
	radians := -degrees * math.Pi / 180
	sin, cos := float32(math.Sin(radians)), float32(math.Cos(radians))
	return mgl32.Vec3{
		direction.X()*cos - direction.Z()*sin,
		direction.Y(),
		direction.X()*sin + direction.Z()*cos,
	}.Normalize()
}

// pitchedBy tilts a heading by a DECORATE pitch. ZDoom's sign is the whole of
// it: a positive pitch aims **down**, which is why the vertical comes out
// negative.
//
// The two callers mean different things by the number, and ZDoom implements
// them differently. A_FireCustomMissile does `self.Pitch += pitch` around the
// spawn, so its pitch is an adjustment to where the player is already looking.
// A_CustomMissile under CMF_ABSOLUTEPITCH - which includes aimmode 2, because
// the C++ sets that flag itself - throws the aim away and works the velocity
// out of the pitch alone: `missilespeed = |cos(pitch)| * Speed` across and
// `Vel.Z = -sin(pitch) * Speed` up. The absolute value is ZDoom's own and it
// matters here: Brutal Doom's smoke is fired at `random(40, 160)` degrees, and
// past ninety a plain cosine would send it out backwards.
func pitchedBy(direction mgl32.Vec3, degrees float64, absolute bool) mgl32.Vec3 {
	if degrees == 0 && !absolute {
		return direction
	}
	flat := mgl32.Vec3{direction.X(), 0, direction.Z()}
	if flat.Len() == 0 {
		return direction // straight up or down already; there is no yaw to keep
	}
	flat = flat.Normalize()

	radians := degrees * math.Pi / 180
	if !absolute {
		// An adjustment: add it to the pitch the heading already has.
		radians += math.Asin(float64(-direction.Normalize().Y()))
	}
	across, up := math.Cos(radians), -math.Sin(radians)
	if absolute {
		across = math.Abs(across)
	}
	return flat.Mul(float32(across)).Add(mgl32.Vec3{0, float32(up), 0}).Normalize()
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

// updateEffects steps everything in flight and clears out what has stopped.
func (g *Game) updateEffects(elapsed float64) {
	for i := len(g.worldActors) - 1; i >= 0; i-- {
		live := g.worldActors[i]
		// A Stop destroys the actor, which is what it does in ZDoom - the
		// state itself is called NULL there, and nothing runs after it. Kept
		// alive to keep falling, a casing spawner that had already dropped its
		// brass on the way through Spawn landed a moment later, was sent to
		// Death by hitSomething, and dropped a second one: every gun in Final
		// Doomer ejected twice.
		if live.cursor.Stopped() || !live.object.IsAlive() {
			live.remove(g)
			g.worldActors = append(g.worldActors[:i], g.worldActors[i+1:]...)
			continue
		}
		live.runtime.World = live
		live.publishState()
		live.move(g, elapsed)
		if !live.cursor.Stopped() {
			for _, op := range live.cursor.Update(live.runtime, elapsed) {
				// Republished between ops, not once a frame: a step's
				// arguments are worked out here, and a zero-tic chain reads
				// what the step before it just wrote. A smoke puff mirrors
				// itself with `scalex - scalex*2` one step after shrinking
				// itself to a tenth, and against a frame-old scalex that came
				// out as the unshrunk size, mirrored - one puff in two filled
				// the screen with grey.
				live.publishState()
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
// meets rather than passing through the world - unless its own class says it
// bounces, in which case that surface reflects it instead. See bounceOff.
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

	live.face(live.velocity, live.facing)

	transform := live.object.Transform()
	from := transform.GetPosition()
	step := live.velocity.Mul(float32(elapsed))
	to := from.Add(step)

	if !live.solidAt(to) {
		transform.SetPosition(to)
		return
	}

	// X, Z and Y are tested separately - moving into solid ground is a floor
	// even though the same step also carries the thing sideways - by trying
	// each on its own against the position the whole step was already found
	// to collide from. Testing X and Z separately (rather than the one
	// diagonal position this used to test) is what lets bounceOff reflect
	// only the component the wall actually opposes: a grenade clipping a
	// wall at a shallow angle slides along it and keeps most of its speed,
	// it does not bounce straight back the way it came.
	blockedByX := live.solidAt(mgl32.Vec3{to.X(), from.Y(), from.Z()})
	blockedByZ := live.solidAt(mgl32.Vec3{from.X(), from.Y(), to.Z()})
	if !blockedByX && !blockedByZ && live.solidAt(mgl32.Vec3{to.X(), from.Y(), to.Z()}) {
		// A step that clips exactly through a corner neither axis alone
		// occupies (both report clear, the diagonal does not). There is no
		// way to tell which face was actually hit - solidAt does not carry
		// that - so this treats it as if both were, the same as every actor
		// was stopped before per-axis reflection existed, rather than let it
		// sail through a corner untouched.
		blockedByX, blockedByZ = true, true
	}
	blockedByFloorOrCeiling := live.solidAt(mgl32.Vec3{from.X(), to.Y(), from.Z()})

	if next, bounced := live.bounceOff(from, to, step, blockedByX, blockedByZ, blockedByFloorOrCeiling); bounced {
		transform.SetPosition(next)
		return
	}

	live.settleOnSolid(g, from)
}

// bounceOff answers what a frag grenade's own +BOUNCEONFLOORS/WALLS/CEILINGS
// do that an ordinary missile's contact does not: reflect the velocity
// component the surface actually opposes instead of ending the flight
// outright. Takes which of the three axes move found blocked rather than
// asking the world itself, so the physics here is plain arithmetic over
// data.Bounce* and velocity - straightforward to test without a voxel world
// behind it.
//
// Reflecting only the blocked axis (or axes, on a true corner hit) rather
// than the whole horizontal velocity is what keeps the bounce angle right:
// a grenade that clips a wall almost end-on should deflect at a shallow
// angle and keep going, not reverse its entire course as if it had hit the
// wall dead center.
//
// ponytail: a wall and a floor blocked on the exact same step, where one
// bounces and the other does not, reflects the one that does and reports the
// whole step as bounced rather than settling for the one that does not -
// every actor this engine bounces sets all three flags together, so the
// floor/wall pair disagreeing on whether to bounce cannot come up for it.
// Give this its own outcome for that axis too if a partially-bouncing actor
// ever needs it to actually stop instead.
func (live *WorldActor) bounceOff(from, to, step mgl32.Vec3, blockedByX, blockedByZ, blockedByFloorOrCeiling bool) (next mgl32.Vec3, bounced bool) {
	next = from
	if blockedByFloorOrCeiling {
		ceiling := step.Y() > 0
		if (ceiling && live.data.BounceCeilings) || (!ceiling && live.data.BounceFloors) {
			live.velocity = mgl32.Vec3{live.velocity.X(), -live.velocity.Y() * float32(live.data.BounceFactor), live.velocity.Z()}
			bounced = true
		}
	} else {
		next = mgl32.Vec3{next.X(), to.Y(), next.Z()}
	}
	if blockedByX || blockedByZ {
		if live.data.BounceWalls {
			factor := float32(live.data.WallBounceFactor)
			vx, vz := live.velocity.X(), live.velocity.Z()
			if blockedByX {
				vx = -vx
			}
			if blockedByZ {
				vz = -vz
			}
			live.velocity = mgl32.Vec3{vx * factor, live.velocity.Y(), vz * factor}
			bounced = true
		}
	}
	if !blockedByX {
		next = mgl32.Vec3{to.X(), next.Y(), next.Z()}
	}
	if !blockedByZ {
		next = mgl32.Vec3{next.X(), next.Y(), to.Z()}
	}
	return next, bounced
}

// settleOnSolid is where anything that does not bounce off what it just met
// ends up: a shell lying on the floor, or a missile's own Death state.
func (live *WorldActor) settleOnSolid(g *Game, from mgl32.Vec3) {
	// It has arrived somewhere solid. A shell settles just above the
	// surface and lies there; anything with a speed of its own has hit
	// something, and its script decides what that means.
	live.velocity = mgl32.Vec3{}
	if !live.resting {
		// Something that has arrived and still has a script to run is
		// waiting rather than playing out, and gets a longer life.
		live.object.SetLifeTime(plantedActorLifetime)
	}
	live.resting = true
	live.homing = false
	// Something lying on the ground stops turning to face the camera.
	// Every sprite here is an XY billboard - it follows the camera's
	// pitch as well as its yaw, which is what ZDoom's +FORCEXYBILLBOARD
	// asks for and what keeps a puff of smoke from going edge-on when
	// you look down at it. A shell casing that has come to rest is the
	// one case where it reads as a defect rather than a nicety: the
	// quad lies almost flat on the floor, so turning on the spot sweeps
	// the shell round the floor with you, and a thing on the ground
	// visibly spinning is the first thing anybody notices. Resting, it
	// drops to a plain Y billboard, which is GZDoom's own default mode
	// (gl_billboard_mode 0): upright, facing you, never pitching.
	live.sprite.CameraFollowMode = quads.SpriteModeFollowHorizontal
	if live.data.Speed > 0 || live.data.Missile {
		live.hitSomething(g, from)
		return
	}
	// A casing's tumble is a Loop with no exit of its own, and its Death
	// state is the frame it lies still on. Landing is what ends it - which
	// is +MISSILE's own doing in ZDoom, and these mods do not all carry
	// the flag by the time it gets here. Nothing to go to means nothing
	// that was spinning, so it lies there as it was.
	live.cursor.Goto("Death")
}

// hitSomething sends a missile to its Death state, which is where a script puts
// the explosion.
//
// It is also where the arrival is heard. ZDoom plays a missile's DeathSound
// from P_ExplodeMissile, the same call that sends it to Death, so like the
// SeeSound at the other end of the flight it is on the actor and in none of its
// states - the classic plasma rifle's whole vocabulary is those two, and with
// only the first read every bolt left the barrel with a noise and landed
// without one.
func (live *WorldActor) hitSomething(g *Game, at mgl32.Vec3) {
	if live.data.DeathSound != "" {
		g.playSoundAt(wep.Op{Sound: live.data.DeathSound, Volume: 1, Pitch: 1, Channel: seeSoundChannel},
			at, live.origin, live.soundScope)
	}
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

// bakedLightHere is the flood-filled voxel light where this actor stands, which
// is what lights it. Doom lights a thing by the sector it is in rather than by
// anything pointed at it, and the voxel light level is this engine's sector
// light: without it a puff of smoke in a room lit entirely by baked torches
// draws black against walls that are lit.
//
// ponytail: one sample at the actor's own position, so a big sprite is lit
// evenly rather than across its own span. Per-corner sampling needs the light
// field in a texture the shader can read, which is a bigger change than a
// smoke puff justifies.
func absFloat(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func (live *WorldActor) bakedLightHere() float32 {
	if live.world == nil {
		return 0
	}
	return bakedLightIn(live.world, live.object.Transform().GetPosition())
}

// bakedLightAt is the same question asked of the map the player is standing
// in, which is what a decal stuck to a wall needs to be lit like that wall.
// The point it is asked about is a hit on a surface, and a hit sits exactly on
// the boundary between the solid block and the air in front of it: asked there
// it lands inside the wall as often as not, where the flood fill never reached
// and the answer is nought. So it is asked a little way out along the face's
// own normal, which is the lit side.
func (g *Game) bakedLightAt(position, normal mgl32.Vec3) float32 {
	return bakedLightIn(g.CubeMap, position.Add(normal.Mul(lightProbeOffset)))
}

// lightProbeOffset is how far off a surface to stand to ask what light falls on
// it: enough to be clear of the boundary, well inside the block in front.
const lightProbeOffset = 0.25

func bakedLightIn(world *voxel.Map, position mgl32.Vec3) float32 {
	if world == nil {
		return 0
	}
	block := world.GetBlockFromPosition(position)
	if block == nil {
		return 0
	}
	return float32(block.GetTorchLight())
}

func (live *WorldActor) solidAt(position mgl32.Vec3) bool {
	if live.world == nil {
		return false
	}
	grid := voxel.PositionToGridInt3(position)
	return live.world.IsSolidBlockAt(grid.X, grid.Y, grid.Z)
}

// TargetHas answers A_JumpIfInTargetInventory: what the thing that fired this
// is carrying. That is the whole of how a thrown charge learns it has been
// told to go off - the weapon gives itself the item, and every charge in the
// air is watching for it.
func (live *WorldActor) TargetHas(item string, amount int) bool {
	if live.owner == nil || item == "" {
		return false
	}
	if amount <= 0 {
		return live.owner.Inventory.AtMax(live.owner.Caps, item)
	}
	return live.owner.Inventory[item] >= amount
}

// The rest of what a script can ask about its surroundings needs monsters,
// which this game has none of, so it answers the way an empty world does.
func (live *WorldActor) CanSee() bool               { return false }
func (live *WorldActor) TargetWithin(float64) bool  { return false }
func (live *WorldActor) TargetHealthBelow(int) bool { return false }
func (live *WorldActor) HasFlag(name string) bool   { return false }

// groundProbe is how far under something to look for a floor, and gravity is
// how fast what has no body of its own falls.
const (
	groundProbe = 0.08
	gravity     = 9.8
)

// face points the thing along a direction, if there is one to point along. A
// velocity of nothing much - a puff of smoke hanging in the air, a casing that
// has landed - leaves it pointed where it already was, which is what ZDoom's
// angle does: it is set when a thing is spawned and by whatever moves it, and
// nothing resets it to zero when the thing stops.
func (live *WorldActor) face(velocity mgl32.Vec3, fallback mgl32.Vec2) {
	flat := mgl32.Vec2{velocity.X(), velocity.Z()}
	if flat.Len() < 0.0001 {
		flat = fallback
	}
	if flat.Len() < 0.0001 {
		return
	}
	live.facing = flat.Normalize()
}

// showCurrentFrame points the sprite at whatever the script is showing, and
// sizes the quad to the picture, which a packed atlas needs because its frames
// are all different.
func (live *WorldActor) showCurrentFrame(g *Game) {
	step := live.cursor.Step()
	// What a step shows is not always what it names: a "####" frame keeps the
	// picture the actor is already on, which only the cursor knows.
	sprite := live.cursor.Sprite()
	if step == nil || sprite == "" {
		live.sprite.Hide()
		return
	}
	// A frame the mod ships all eight views of is drawn from the one the
	// observer is standing in. What the step names is the view from behind, so
	// a frame with no others - which is most of them - is unaffected, and a
	// rocket flying away from the player still shows its exhaust.
	//
	// ponytail: the view is chosen here, once a frame, rather than per draw
	// call in the sprite pass. That means a second viewpoint in the same frame
	// - the shadow pass - draws the angle the player sees. These are rockets
	// and casings a few pixels across and they cast no shadows anyway; doing
	// it properly means per-direction quad sizes and pivots in the batch,
	// because Doom's rotations are not all the same width.
	if views := live.data.Views[sprite]; len(views) == core.ViewDirections {
		sprite = views[core.GetViewDirection(
			g.Camera.GetPosition(), live.object.Transform().GetPosition(), live.facing)]
	}
	index, known := g.sharedSpriteIndex[sprite]
	if !known {
		// Loading checks every frame a compiled actor names, so this is a
		// weapons folder somebody has edited. Said once per frame name rather
		// than once per tic.
		g.warnMissingSprite(sprite)
		live.sprite.Hide()
		return
	}
	if !live.drawn {
		live.drawn = true
		if live.hiddenAtPointBlank(g.Camera.GetPosition()) {
			live.sprite.Hide()
			return
		}
	}
	live.sprite.Show()
	live.sprite.BakedLight = live.bakedLightHere()
	// A frame ZDoom marks BRIGHT ignores the light where it stands: a muzzle
	// flash is the same flash in a lit room and a dark one. Which frames those
	// are is a property of the frame, not of the actor's render style - the
	// two used to be conflated here, so every additive sprite glowed and no
	// plain one ever did, and a mod's bright frames drawn on top of a
	// translucent puff came out as dark as the room.
	//
	// ponytail: added rather than replacing the lighting, which is what the
	// shader can do without another attribute, so a bright frame in a lit room
	// is brighter than one in a dark room instead of identical. Both are far
	// brighter than the room, which is what the frame is asking for, and the
	// overshoot is what the bloom threshold picks up.
	live.sprite.Emissive = 0
	if step.Bright {
		live.sprite.Emissive = brightEmissive
	}
	live.sprite.TextureIndices = []uint16{uint16(index)}
	left, top, right, bottom := g.sharedAtlas.GetUVInPx(index)
	// The size is the scale's magnitude, not the scale. ZDoom reads a negative
	// scale as a mirrored sprite rather than a smaller one, and several of
	// these effects flip themselves that way to make one puff of smoke look
	// like two - a size below zero here turned the quad inside out instead.
	//
	// ponytail: the mirror itself is dropped, so a flipped frame draws the
	// same way round as an unflipped one. Reproducing it means flipping the
	// UV rect, which is worth doing when something asymmetric flips.
	live.sprite.Size = mgl32.Vec2{
		float32(right-left) / spritePixelsPerUnit * absFloat(live.scale.X()),
		float32(bottom-top) / spritePixelsPerUnit * absFloat(live.scale.Y()),
	}
	// Anchored on the picture's own hotspot rather than its geometric center:
	// a muzzle flash or a burst of smoke is drawn where its own art says to
	// put it relative to where it spawned, not wherever its visible pixels
	// happen to be centered once padding is trimmed off.
	pivot := g.sharedAtlas.PivotFromCenter(index)
	live.sprite.PivotOverride = &pivot
	if debugActors {
		log.Printf("[actor] %-22s %-8s at=%v pivot=%+.2f,%+.2f size=%.2fx%.2f scale=%.2f alpha=%.2f style=%q emissive=%.2f baked=%.0f",
			live.data.Name, sprite, live.object.Transform().GetPosition(),
			pivot.X(), pivot.Y(),
			live.sprite.Size.X(), live.sprite.Size.Y(),
			live.scale.X(), live.sprite.Tint.W(), live.data.Render.Style,
			live.sprite.Emissive, live.sprite.BakedLight)
	}
}

// apply is what one of these asks the world for. It is a smaller set than a
// weapon's: nobody is holding it, so there is no trigger, no ammo and no view.
func (live *WorldActor) apply(g *Game, op wep.Op) {
	switch op.Op {
	case wep.OpPlaySound:
		g.playSoundAt(op, live.object.Transform().GetPosition(), live.origin, live.soundScope)
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
		live.changeVelocity(op)
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
	case wep.OpGiveToTarget:
		// A_GiveInventory(..., AAPTR_TARGET) and its opposite: the thing
		// hands an item to whoever fired it, which is the weapon that spawned
		// it, whether or not that weapon is still the one in hand.
		if live.owner != nil && op.Item != "" {
			if op.N < 0 {
				live.owner.Inventory.Take(op.Item, -op.N)
			} else {
				live.owner.Inventory.Give(live.owner.Caps, op.Item, max(op.N, 1))
				live.owner.StartItemTimer(op.Item)
			}
		}
	case wep.OpDie:
		// A_Die kills the actor, which in ZDoom means sending it to its Death
		// state - and doing nothing at all if it is already dead. That second
		// half is the whole of how a planted C4 charge goes off: its Death
		// state opens with A_Die and then runs on into the explosion, and the
		// second pass through the same frame has to fall through rather than
		// jump back. Killing it outright here meant the charge vanished
		// silently and nothing ever exploded.
		if live.dead {
			break
		}
		live.dead = true
		if !live.cursor.Goto("Death") {
			live.cursor.Goto("@enginestop")
			live.object.Kill()
		}
	case wep.OpRemove:
		live.cursor.Goto("@enginestop")
		live.object.Kill()
	}
}

// changeVelocity is A_ChangeVelocity(x, y, z, flags): a push, added to what the
// thing is already doing unless CVF_REPLACE says to replace it, and measured
// along the way it is heading when CVF_RELATIVE says so.
//
// Replacing rather than adding is not a near miss. Final Doomer's C4 charge is
// thrown forward as a missile and then given `A_ChangeVelocity(0, 0,
// frandom(1.5, 2), CVF_RELATIVE)` for a little lift; taken as a replacement,
// that is the whole of its velocity, and the charge went straight up out of
// the player's hands instead of across the room.
func (live *WorldActor) changeVelocity(op wep.Op) {
	values := live.argsOf(op)
	if len(values) < 3 {
		return
	}
	flags := strings.ToUpper(op.Flags)
	// Doom's x and y are the ground plane and its z is up.
	forward := float32(values[0] * worldUnitsPerDoomUnit * wep.TicRate)
	sideways := float32(values[1] * worldUnitsPerDoomUnit * wep.TicRate)
	up := float32(values[2] * worldUnitsPerDoomUnit * wep.TicRate)

	push := mgl32.Vec3{forward, up, sideways}
	if strings.Contains(flags, "CVF_RELATIVE") {
		// Along its own heading rather than along the world's axes.
		heading := live.velocity
		heading[1] = 0
		if heading.Len() > 0.0001 {
			heading = heading.Normalize()
			right := heading.Cross(mgl32.Vec3{0, 1, 0}).Normalize()
			push = heading.Mul(forward).Add(right.Mul(sideways)).Add(mgl32.Vec3{0, up, 0})
		}
	}
	if strings.Contains(flags, "CVF_REPLACE") {
		live.setVelocity(push)
		return
	}
	live.setVelocity(live.currentVelocity().Add(push))
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
	if alpha <= 0 {
		// A_FadeTo's own FTF_REMOVE, which every one of these scripts passes
		// and the migration does not carry: something that has faded to
		// nothing is gone. Final Doomer's bullet puff fades out inside a loop
		// that also grows it ten percent a tic, so left alive it went on
		// growing invisibly - sixty five voxels across and six hundred of them
		// at once, holding the actor budget shut against everything else.
		//
		// ponytail: without the flag, so it removes any fade to zero. An actor
		// at no alpha can only be seen again if something puts its alpha back,
		// and nothing in these mods does.
		live.object.Kill()
	}
}

func (live *WorldActor) currentVelocity() mgl32.Vec3 { return live.velocity }

func (live *WorldActor) setVelocity(velocity mgl32.Vec3) {
	live.velocity = velocity
	live.resting = velocity.Len() < 0.0001 && live.resting
}

func (live *WorldActor) remove(g *Game) {
	live.object.Kill()
	live.sprite.Hide()
	// Its sound channels go with it. Each of these has an origin of its own -
	// see the origin field - and a channel that outlives what it belonged to
	// is a mixer the speaker steps through on every callback for the rest of
	// the run.
	if g.audio != nil {
		g.audio.DropOrigin(live.origin)
	}
}

// How long a spawned thing may hang around whatever its script says, how far in
// front of the player a stray particle appears, how fast smoke drifts, and how
// much light a frame marked BRIGHT gives itself.
//
// plantedActorLifetime is the longer leash something gets once it has come to
// rest with its script still running. That is not an effect playing out - it
// is a thing waiting to be told something: Final Doomer's C4 charge sticks to
// a wall and watches the player's inventory for the word to go off, and eight
// seconds is shorter than the time it takes to plant a few and reach for the
// detonator.
const (
	worldActorLifetime   = 8.0
	plantedActorLifetime = 90.0
	spawnDistanceInFront = 0.5
	brightEmissive       = float32(1)
)

func alphaOf(actor *wep.Effect) float64 {
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

// doomPlayerRadius is Doom's own 16, which is what GZDoom measures the rule
// below against - not this engine's player, whose collision is its own affair.
const doomPlayerRadius = 16.0

// hiddenAtPointBlank is GZDoom's "don't draw first frame of a player missile",
// out of HWSprite::Process:
//
//	if (thing->flags & MF_MISSILE) {
//	    if (!(thing->flags7 & MF7_FLYCHEAT) && thing->target == vp.ViewActor && ...) {
//	        double speed = thing->Vel.Length();
//	        if (speed >= thing->target->radius / 2) {
//	            double clipdist = clamp(thing->Speed, thing->target->radius, thing->target->radius * 2);
//	            if ((thingpos - vp.Pos).LengthSquared() < clipdist * clipdist) return;
//	        }
//	    }
//	    thing->flags7 |= MF7_FLYCHEAT;   // only ever the very first frame
//	}
//
// It is what keeps a rocket out of your face on the frame you fire it, and
// without it Brutal Doom's assault shotgun draws eight DecorativeTracers - a
// bright additive quad each - a tenth of a voxel from the eye on every shot.
// They leave at 120 units a tic and are gone by the next frame, so all anybody
// sees is one enormous flash that sits still while the gun moves under it: it
// spawns at the player rather than at the muzzle, so it does not track the aim
// either. It reads exactly like a broken muzzle flash, and every weapon in the
// mod that lays a tracer has one.
//
// The three conditions all carry weight. Only a missile: smoke and casings are
// meant to be seen up close. Only one the player fired, which here is anything
// a weapon spawned. And only one actually moving - a puff spawned with a
// missile flag and no speed stays where it was put, which is where its script
// wants it.
func (live *WorldActor) hiddenAtPointBlank(eye mgl32.Vec3) bool {
	if !live.data.Missile || live.owner == nil {
		return false
	}
	speed := float64(live.velocity.Len()) / (worldUnitsPerDoomUnit * wep.TicRate)
	if speed < doomPlayerRadius/2 {
		return false
	}
	clipDistance := min(max(live.data.Speed, doomPlayerRadius), doomPlayerRadius*2)
	away := live.object.Transform().GetPosition().Sub(eye).Len()
	return float64(away) < clipDistance*worldUnitsPerDoomUnit
}
