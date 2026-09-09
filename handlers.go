package main

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/events"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
	"github.com/memmaker/goray/wep"
)

// EditorTool is what a left click does while a map authoring tool is picked.
// Digging, painting and filling are things you do to a map, not things a gun
// does, so they no longer travel through the weapon in hand: they work whatever
// is being held, and nothing is held while the tool is on.
type EditorTool uint8

const (
	EditorToolNone EditorTool = iota
	EditorToolDig
	EditorToolPaint
	EditorToolFill
)

func (t EditorTool) String() string {
	switch t {
	case EditorToolDig:
		return "digger"
	case EditorToolPaint:
		return "paint gun"
	case EditorToolFill:
		return "fill gun"
	}
	return "off"
}

// setEditorTool picks the tool a left click uses, and says so.
func (g *Game) setEditorTool(tool EditorTool) {
	g.editor.tool = tool
	g.Toast(fmt.Sprintf("editor tool: %s", tool))
}

// applyEditorTool does what the picked tool does where the player is looking.
func (g *Game) applyEditorTool() {
	origin, direction := g.Camera.GetPickingRayFromScreenPosition(0.5, 0.5)
	result := g.rayCast(origin, direction)
	if !result.HitInfo.Hit || result.HitObject != nil {
		return
	}
	blockPos := result.HitInfo.CollisionGridPosition
	switch g.editor.tool {
	case EditorToolDig:
		g.CubeMap.SetAir(blockPos)
		g.EmitParticlesByName(quads.ParticlesBulletImpact, result.WorldPosition(), 30)
	case EditorToolPaint:
		g.CubeMap.SetBlockSide(blockPos, result.HitInfo.Side, uint16(g.editor.currentPaintGunTextureIndex))
	case EditorToolFill:
		g.CubeMap.SetBlockSideFill(blockPos, result.HitInfo.Side, uint16(g.editor.currentPaintGunTextureIndex))
	}
}

// selectWeapon puts weapon number index (0 based) in the player's hands.
func (g *Game) selectWeapon(index int) {
	if index < 0 || index >= len(g.weapons) || index == g.currentWeaponIndex {
		return
	}
	if held := g.currentWeapon(); held != nil {
		held.Unequip()
	}
	g.currentWeaponIndex = index
	weapon := g.currentWeapon()
	weapon.Equip()
	g.keepLoaded(weapon)
	// The view belongs to the weapon in hand: a scope put away stops zooming.
	g.applyWeaponZoom()
	if ammo := weapon.AmmoLine(); ammo != "" {
		g.Toast(fmt.Sprintf("%s (%s)", weapon.DisplayName(), ammo))
		return
	}
	g.Toast(weapon.DisplayName())
}

// keepLoaded remembers which weapons have their pictures in memory and gives
// back the ones nobody has held for a while. Loading an atlas costs something
// like fifty milliseconds, which is a stutter to pay on every switch, and the
// whole folder at once is half a gigabyte of texture - so a handful of the
// most recent are kept, which is what swapping between two guns needs.
func (g *Game) keepLoaded(weapon *Weapon) {
	for i, kept := range g.recentWeapons {
		if kept == weapon {
			g.recentWeapons = append(g.recentWeapons[:i], g.recentWeapons[i+1:]...)
			break
		}
	}
	g.recentWeapons = append(g.recentWeapons, weapon)
	for len(g.recentWeapons) > weaponsKeptLoaded {
		g.recentWeapons[0].unload()
		g.recentWeapons = g.recentWeapons[1:]
	}
}

// weaponsKeptLoaded is how many weapons' pictures stay in memory at once.
const weaponsKeptLoaded = 4

// currentWeapon is what the player is holding, or nothing when they are empty
// handed - which is how a run starts, before anything has been picked up.
func (g *Game) currentWeapon() *Weapon {
	if g.currentWeaponIndex < 0 || g.currentWeaponIndex >= len(g.weapons) {
		return nil
	}
	return g.weapons[g.currentWeaponIndex]
}

// weaponByScope is the weapon a spawned actor's sounds and library of further
// actors belong to - see WorldActor.soundScope - which is whichever weapon
// spawned it, not whatever is in hand by the time it does something.
func (g *Game) weaponByScope(id string) *Weapon {
	if index, known := g.weaponByID[id]; known {
		return g.weapons[index]
	}
	return nil
}

// HitscanEffect is one pellet's worth of a shot, with the compiled op it came
// from so the impact knows what kind of shot it was and how hard, and the
// weapon that fired it, so the puff it names can be drawn as itself.
type HitscanEffect struct {
	Hit       RaycastResult
	Direction mgl32.Vec3
	Shot      wep.Op
	Weapon    *Weapon
}

func (g *Game) fireHitscanWith(weapon *Weapon, origin, direction mgl32.Vec3, shot wep.Op) {
	hit := g.rayCast(origin, direction)
	events.Publish(events.EventHitScanCollision, HitscanEffect{
		Hit:       hit,
		Direction: direction,
		Shot:      shot,
		Weapon:    weapon,
	})
}

func (g *Game) hitScanCollision(id events.EventID, event events.Event) {
	hitScanEffect := event.(HitscanEffect)
	hit := hitScanEffect.Hit
	if !hit.HitInfo.Hit {
		return // the ray ran out without touching anything, so there is nothing to mark
	}
	// WorldPosition picks the right one of the two positions a hit carries: the
	// object intersection for an object, the voxel face for the world. The two
	// used to be swapped here, which put every impact effect at the world origin.
	hitPos := hit.WorldPosition()

	// The surface normal of a voxel hit is the face the ray came in through; an
	// object hit has no face, so the decals go on the wall behind it instead.
	surfaceNormal := hit.HitInfo.Side.AsDirection().ToVec3()
	g.createImpactEffects(hitScanEffect.Weapon, hitScanEffect.Shot, hit.HitObject, hitPos, surfaceNormal)
	if hit.HitObject != nil && hitScanEffect.Shot.Damage > 0 {
		if isNPC(hit.HitObject) {
			g.splatterBehind(hitPos, hitScanEffect.Direction)
		}
		g.damage(hit, hitScanEffect.Shot.Damage)
	}
}

// DamageEvent is a shot landing on something, published so whatever grows a
// health bar later has it without any of this changing.
type DamageEvent struct {
	Target *quads.GameObject
	Amount int
	Where  mgl32.Vec3
}

// damage is what a shot does to what it hits: publish the hit, then - for an
// actor - let TakeDamage decide whether it flinches or dies. hit.Relative.Y()
// in [0.6, 0.8) is a hit somewhere around the head rather than the torso; see
// Actor.TakeDamage for what headshot vs. body then does with it.
func (g *Game) damage(hit RaycastResult, amount int) {
	hitObject := hit.HitObject
	events.Publish(events.EventDamage, DamageEvent{
		Target: hitObject,
		Amount: amount,
		Where:  hit.WorldPosition(),
	})
	actor := g.actorFor(hitObject)
	if actor == nil {
		return
	}
	// Whatever the player last shot is what the companions pile onto. That is
	// the whole of the enemy model: there are no factions in this game, every
	// actor is a Scientist, and "the one you are shooting at" is the only
	// answer that does not need a map to declare who hates whom.
	//
	// ponytail: only the hitscan path sets this, which is every conventional
	// weapon. A projectile kill (physicsCollision) does not - it kills what it
	// hits outright, so there is nothing left to pile onto anyway.
	if !actor.following {
		g.companionTarget = actor
	}
	headshot := hit.Relative.Y() > 0.6 && hit.Relative.Y() < 0.8
	actor.TakeDamage(headshot)
}
func (g *Game) fireProjectile(origin, direction mgl32.Vec3, damage int) {
	offsetOrigin := origin.Add(direction.Mul(0.8))

	sprite := quads.NewSprite(g.AtlasSprites, []uint16{179})
	// A plasma bolt is its own light source: the sprite is drawn past white so the
	// bloom pass halos it, and an omni light rides along so the room it flies through
	// lights up with it. Both are the same colour, which is what sells it as one thing.
	sprite.Emissive = projectileEmissive
	newProjectile := quads.NewGameObject("Projectile")
	newProjectile.SetLifeTime(5)
	newProjectile.Transform().SetPosition(offsetOrigin)
	newProjectile.SetComponent(sprite)

	physics := quads.NewPhysicsComponent(false, voxel.NewAABB(mgl32.Vec3{-0.1, -0.1, -0.1}, mgl32.Vec3{0.1, 0.1, 0.1}))
	newProjectile.SetComponent(physics)

	newProjectile.SetComponent(quads.NewProjectileComponent(damage))

	g.Sprites.Add(sprite)
	sprite.PlayAnimationLooping(g.projectileFlyingAnim)

	g.physicsSystem.AddObject(physics)
	physics.SetVelocity(direction.Mul(10))

	// The bolt takes the one tracked light off whatever had it, so the muzzle
	// flash no longer owns it and A_Light0 must not reach in and switch it off.
	g.muzzleFlashLit = false
	g.AttachLightTo(newProjectile, projectileLightColor, projectileLightIntensity, projectileLightReach)
}

// How a projectile glows. Emissive is how far past white the sprite is drawn, which
// has to clear DefaultBloomThreshold by a margin to bloom at all; the light's
// intensity is its brightness at LIGHT_REF_DIST, on the same scale as a map light.
const (
	projectileEmissive       = float32(6)
	projectileLightIntensity = float32(10)
	projectileLightReach     = float32(8)
)

var projectileLightColor = mgl32.Vec3{0.4, 0.8, 1.0}

func (g *Game) playerInteractWith(id events.EventID, event events.Event) {
	hoveredObj := event.(*quads.GameObject)
	if hoveredObj == nil {
		return
	}
	if door := g.doorFor(hoveredObj); door != nil {
		door.Interact()
		return
	}
	if actor := g.actorFor(hoveredObj); actor != nil && actor.Dialogue != nil {
		g.openDialogue(actor.Dialogue)
	}
}
func (g *Game) physicsCollision(id events.EventID, event events.Event) {
	collisionEvent := event.(quads.CollisionEvent)
	mover := collisionEvent.ObjectA.GameObject()
	if mover.HasComponent(quads.ComponentTypeProjectile) && mover.IsAlive() {
		mover.Kill()

		projectileComponent := mover.GetComponent(quads.ComponentTypeProjectile).(*quads.ProjectileComponent)

		// impact effects
		var hitObject *quads.GameObject
		var relativePosition mgl32.Vec3
		// npc animation & world state changes
		if collisionEvent.ObjectB != nil {
			hitObject = collisionEvent.ObjectB.GameObject()
			relativePosition = hitObject.Transform().GetRelativePosition(collisionEvent.WorldPos)
		}

		_ = relativePosition
		g.createImpactEffects(nil, wep.Op{Puff: wep.PuffExplosion}, hitObject, collisionEvent.WorldPos, collisionEvent.SurfaceNormal)
		if projectileComponent.HitEffect > 0 {
			// A direct projectile hit always kills outright, same as before -
			// routed through TakeDamage(headshot: true) now so the corpse
			// actually stops moving, stops colliding and stops taking further
			// hits instead of just freezing its sprite. See actor.go's die().
			if actor := g.actorFor(hitObject); actor != nil {
				actor.TakeDamage(true)
			}
		}
	}
}

// splatterBehind carries a hit on a body through to the wall behind it and leaves
// blood there. The ray restarts past the object's own collider so it does not hit
// the same body again; if nothing solid is close enough behind, nothing is drawn.
//
// ponytail: fixed restart offset and range rather than the object's real extent.
// Bodies in this game are all roughly a metre wide.
func (g *Game) splatterBehind(hitPos, direction mgl32.Vec3) {
	behind := g.rayCast(hitPos.Add(direction.Mul(1.0)), direction)
	if !behind.HitInfo.Hit || behind.HitObject != nil || behind.HitInfo.Distance > bloodSplatterRange {
		return
	}
	// behind.HitObject is always nil here - the guard above already bailed
	// out on hitting one - so this always lands on raw terrain, never on a
	// door or anything else that could carry it away.
	g.addDecal(wep.BloodSplatSprite, behind.HitInfo.CollisionWorldPosition,
		behind.HitInfo.Side.AsDirection().ToVec3(), behind.HitObject)
}

// addDecal sticks one of the engine's own decal pictures on a wall - or, if
// hitObject is not nil, on that GameObject, so it moves with it from then on
// (a bullet hole on a door, say - see quads.decal's own doc comment). They
// live in the shared picture store with everything else a weapon throws at a
// wall, so they are packed into the same atlas as the rest and are found by
// name rather than by a tile number somebody has to keep in step with a sheet.
func (g *Game) addDecal(sprite string, pos, normal mgl32.Vec3, hitObject *quads.GameObject) {
	index, known := g.sharedSpriteIndex[sprite]
	if !known {
		return
	}
	width, height := decalSize(g.sharedAtlas, index)
	g.Sprites.AddDecal(g.sharedAtlas, uint16(index), pos, normal,
		width, height, g.bakedLightAt(pos, normal), hitObject)
}

// decalSize is how much wall a decal covers, which is ZDoom's own rule: a
// decal's size in map units is its picture's own size in pixels, times the
// scale decaldef.txt gives it. So a seven by nine bullet chip is three and a
// half by four and a half units of wall - a chip, and worth saying out loud
// because it is a sixth of the blob that used to be here.
//
// It comes off the atlas rather than out of a constant because the picture is
// the only thing that knows: five bullet chips are five different shapes, and
// a number written down next to them is a number that goes stale the first
// time one of them is redrawn.
func decalSize(atlas *glhf.TextureAtlasPacked, index int) (float32, float32) {
	left, top, right, bottom := atlas.GetUVInPx(index)
	return float32(right-left) * decalScale * worldUnitsPerDoomUnit,
		float32(bottom-top) * decalScale * worldUnitsPerDoomUnit
}

// How big the decals are, in world units where a voxel is 1, and how far behind a
// body a wall can be and still catch the blood.
const (
	// decalScale is what decaldef.txt scales BulletChip1..5 by, on both axes.
	decalScale         = float32(0.5)
	bloodSplatterRange = 4.0
)

// createImpactEffects paints what a shot leaves where it landed. A weapon that
// names its own puff actor gets that actor's own sprite, drawn exactly as its
// mod drew it; the particle system is only what is left for a shot with
// nothing of its own to draw, or for a body, which has no mod sprite for blood.
// isNPC is who bleeds: a body with somewhere to walk to, which today is only
// the Scientist. A barrel, a shelf's pickup or a decoration is hit the same
// way a wall is - it has no blood of its own to spill.
func isNPC(hitObject *quads.GameObject) bool {
	return hitObject != nil && hitObject.HasComponent(quads.ComponentTypePathing)
}

func (g *Game) createImpactEffects(weapon *Weapon, shot wep.Op, hitObject *quads.GameObject, pos, surfaceNormal mgl32.Vec3) {
	if shot.Puff == wep.PuffExplosion {
		g.spawnProjectileImpact(pos, g.AtlasSprites, g.projectileImpactAnim)
		g.EmitParticlesByName(quads.ParticlesExplosion, pos, 80)
		g.EmitParticlesByName(quads.ParticlesSmoke, pos, 40)
		if isNPC(hitObject) {
			g.EmitParticlesByName(quads.ParticlesBlood, pos, 30)
		}
		return
	}
	// A shot that does no damage is a sight, not a shot: the Blacktail's laser
	// pointer is a hitscan fired every tic whose whole job is to put its puff
	// on whatever the barrel is aimed at. Held on a body it bled thirty
	// particles a tic, forever. It still spawns its puff below, so the red dot
	// lands on a barrel the same as it lands on a wall.
	if isNPC(hitObject) && shot.Damage > 0 {
		g.EmitParticlesByName(quads.ParticlesBlood, pos, 30)
		return
	}
	// The mod's own puff and the mark on the wall are two separate things in
	// ZDoom, spawned by the same hit and neither in place of the other:
	// P_LineAttack spawns the puff, and then spawns a decal - if anything
	// names one.
	puff := g.spawnPuffAt(weapon, shot.PuffActor, pos, surfaceNormal)
	if puff == nil {
		g.EmitParticlesByName(quads.ParticlesBulletImpact, pos, 30)
	}
	if name := g.decalFor(weapon, shot.PuffActor); leavesABulletHole(name) {
		g.addDecal(wep.BulletChipSprites[rand.Intn(len(wep.BulletChipSprites))], pos, surfaceNormal, hitObject)
	}
}

// decalFor is what a shot from this weapon marks the wall with, by ZDoom's own
// name for the decal. SpawnShootDecal reads the shooter's ready weapon first
// and only falls back on the puff, and +NODECAL on the puff stops both.
//
// A shot that names no puff at all gets GZDoom's own BulletPuff, and that
// carries `Decal "BulletChip"` - so a plain hitscan chips the wall even when
// neither the weapon nor the mod says a word about decals. The Dual 92s are
// the whole of `A_FireBullets(2.5, 2.5, -1, 5)`, name nothing, and left the
// wall unmarked until this read the default the way ZDoom does.
//
// A puff the mod defines decides for itself, which is how the Blacktail's
// laser pointer - a hitscan fired every single tic to put a red dot on the
// wall - says `Decal "None"` and does not chip the wall thirty five times a
// second.
func (g *Game) decalFor(weapon *Weapon, puffActor string) string {
	if weapon == nil {
		return ""
	}
	puff := weapon.Actors[lowerName(puffActor)]
	if puff != nil && puff.NoDecal {
		return ""
	}
	if weapon.Data.Decal != "" {
		return weapon.Data.Decal
	}
	if puff == nil || puff.Stock {
		return "BulletChip"
	}
	return puff.Decal
}

// leavesABulletHole says whether a decal by that name is one this engine has a
// picture for.
//
// ponytail: one picture, the bullet hole, for the names that mean a chip in a
// wall. A mod names a dozen others - scorches, cracks, sword slashes, blood
// smears - and drawing a bullet hole for a katana is worse than drawing
// nothing, which is what the rest get until there are pictures for them.
func leavesABulletHole(decal string) bool {
	switch strings.ToLower(decal) {
	case "bulletchip", "bulletchipsmall", "chip", "impact":
		return true
	}
	return false
}
