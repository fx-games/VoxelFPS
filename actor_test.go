package main

import (
	"strings"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
)

// testActor builds a bare Actor good enough for TakeDamage/die to run without a
// whole game behind them: a real GameObject with a sprite (both death paths
// play an animation on it) and an empty PhysicsSystem, which tracks nothing
// until something is added to it - RemoveObject is a safe no-op for an object
// it never heard of, and the collision grid is real enough for a move to be
// listed on it, see TestAGuideOnTourIsStillSelectableWhereItWalkedTo.
func testActor(t *testing.T, health int, at mgl32.Vec3) *Actor {
	t.Helper()
	object := quads.NewGameObject("t")
	object.Transform().SetPosition(at)
	object.SetComponent(quads.NewSprite(nil, []uint16{0}))
	object.SetComponent(quads.NewPhysicsComponent(false, voxel.AABB{}))
	return &Actor{
		Health:        health,
		MaxHealth:     health,
		object:        object,
		pathing:       quads.NewPathingComponent(),
		physicsSystem: quads.NewCollisionSolver(func(x, y, z int32) bool { return false }),
	}
}

// This is the bug the user caught: radiusDamage used to deal an instant kill
// for every A_Explode in earshot, real detonation or not.
// GrenadeAlerter overrides FragGrenadeExplosion's Spawn state with its own,
// much smaller A_Explode(2, 240) (see assets/weapons/bd_frag_grenade/source.txt) and
// fires once every fuse tick while the grenade is still in flight, well
// before it actually lands and goes off - so a bystander three and a half
// units away died the instant a grenade was thrown, never mind whether it
// ever hit anything. A hit that carries its own Amount costs exactly that.
func TestDamageWithAnAmountTakesItRatherThanKillingOutright(t *testing.T) {
	alerterPoke := testActor(t, defaultActorHealth, mgl32.Vec3{})
	alerterPoke.TakeDamage(Damage{Amount: 2})
	if alerterPoke.dying {
		t.Fatalf("GrenadeAlerter's own 2-damage poke killed a %d-health actor outright", defaultActorHealth)
	}
	if alerterPoke.Health != defaultActorHealth-2 {
		t.Errorf("Health = %d, want %d", alerterPoke.Health, defaultActorHealth-2)
	}

	realBlast := testActor(t, defaultActorHealth, mgl32.Vec3{})
	realBlast.TakeDamage(Damage{Amount: 67})
	realBlast.TakeDamage(Damage{Amount: 53}) // FragGrenadeExplosion's own falloff calls stack
	if !realBlast.dying {
		t.Errorf("120 cumulative blast damage on a %d-health actor left it alive", defaultActorHealth)
	}

	dead := testActor(t, defaultActorHealth, mgl32.Vec3{})
	dead.dying = true
	dead.TakeDamage(Damage{Amount: 999})
	if dead.Health != defaultActorHealth {
		t.Errorf("a second blast changed a corpse's health to %d, want it left at %d", dead.Health, defaultActorHealth)
	}
}

// radiusDamage only reaches what the blast actually covers, and passes the
// real amount through rather than substituting a kill.
func TestRadiusDamageOnlyHitsActorsInsideTheBlast(t *testing.T) {
	g := &Game{}
	near := testActor(t, defaultActorHealth, mgl32.Vec3{1, 0, 0})
	far := testActor(t, defaultActorHealth, mgl32.Vec3{100, 0, 0})
	g.actors = []*Actor{near, far}

	g.radiusDamage(mgl32.Vec3{}, 5, 2)

	if near.Health != defaultActorHealth-2 {
		t.Errorf("actor inside the blast radius has %d health, want %d", near.Health, defaultActorHealth-2)
	}
	if far.Health != defaultActorHealth {
		t.Errorf("actor outside the blast radius took damage: %d health, want %d", far.Health, defaultActorHealth)
	}
}

// testGame is enough Game for the companion's follow logic: it only ever
// reads the player's position. Its combat half needs a whole GL world behind
// it (fireHitscanWith raycasts the voxel map) and is left to be looked at in
// the running game.
func testGame(t *testing.T, playerAt mgl32.Vec3) *Game {
	t.Helper()
	object := quads.NewGameObject("Player")
	object.Transform().SetPosition(playerAt)
	game := &Game{}
	game.player = newPlayer(game, object)
	return game
}

// A companion has to actually stop when it has caught up. Nothing in the
// physics system slows a body down on its own - the pathfinder writes a
// velocity every frame while it has somewhere to be, and the last one it
// wrote stands until something overwrites it - so "arrived" that only
// switches the pathing off leaves the companion coasting away across the
// room at walking pace, forever.
func TestCompanionStopsDeadOnceItHasCaughtUp(t *testing.T) {
	game := testGame(t, mgl32.Vec3{10, 1, 10})
	companion := testActor(t, defaultActorHealth, mgl32.Vec3{11, 1, 10})
	companion.follow()
	companion.object.Physics().SetVelocity(mgl32.Vec3{2, -5, 3})

	game.followPlayer(companion, 1.0/60.0)

	if companion.pathing.Enabled {
		t.Error("a companion standing next to the player is still being driven by the pathfinder")
	}
	if velocity := companion.object.Physics().Velocity(); velocity.X() != 0 || velocity.Z() != 0 {
		t.Errorf("horizontal velocity = %v, want it zeroed - the companion coasts off otherwise", velocity)
	}
	if velocity := companion.object.Physics().Velocity(); velocity.Y() != -5 {
		t.Errorf("Y velocity = %f, want the fall left alone at -5", velocity.Y())
	}
}

// The other half: out of reach, it walks after the player - but re-aiming is
// a whole A* search (SetDestination throws the current route away and the
// pathfinder immediately runs a fresh one), so it happens on a clock rather
// than every frame.
func TestCompanionWalksAfterTheDistantPlayerButRepathsOnAClock(t *testing.T) {
	game := testGame(t, mgl32.Vec3{20, 1, 20})
	companion := testActor(t, defaultActorHealth, mgl32.Vec3{2, 1, 2})
	companion.follow()

	game.followPlayer(companion, 1.0/60.0)
	if !companion.pathing.Enabled {
		t.Fatal("a companion left behind is not walking after the player")
	}
	if got := companion.pathing.Destination(); got != (mgl32.Vec3{20, 1, 20}) {
		t.Fatalf("destination = %v, want the player's position", got)
	}

	// The player moves; the very next frame must not buy another search.
	game.player.Transform().SetPosition(mgl32.Vec3{24, 1, 24})
	game.followPlayer(companion, 1.0/60.0)
	if got := companion.pathing.Destination(); got != (mgl32.Vec3{20, 1, 20}) {
		t.Errorf("re-pathed to %v the very next frame, want one search per %.2fs", got, companionRepathInterval)
	}

	// ...but once the clock is up, it follows the player to where they went.
	game.followPlayer(companion, companionRepathInterval)
	if got := companion.pathing.Destination(); got != (mgl32.Vec3{24, 1, 24}) {
		t.Errorf("destination = %v after the re-path interval, want the player's new position", got)
	}
}

// "Wait here" means here. A dismissed companion handed its old patrol back
// would turn round and walk off to the far end of a beat it was hired away
// from half a map ago.
func TestDismissedCompanionStaysWhereItWasLeft(t *testing.T) {
	game := testGame(t, mgl32.Vec3{0, 1, 0})
	companion := testActor(t, defaultActorHealth, mgl32.Vec3{30, 1, 30})
	companion.patrol = []mgl32.Vec3{{1, 1, 1}, {40, 1, 40}}
	companion.follow()
	companion.stopFollowing()

	game.actors = []*Actor{companion}
	game.updateActors(1.0 / 60.0)

	if companion.following {
		t.Error("still following after being dismissed")
	}
	if companion.pathing.Enabled {
		t.Errorf("dismissed companion is walking to %v instead of waiting", companion.pathing.Destination())
	}
}

// startTour is what a guide's own "tour" dialogue action calls - see
// dialogueActions. It has to set off for the second point on the route: the
// first is always the spot it is already standing on, added by patrolRoute.
func TestStartTourSetsOffForTheSecondWaypoint(t *testing.T) {
	guide := testActor(t, defaultActorHealth, mgl32.Vec3{})
	guide.patrol = []mgl32.Vec3{{0, 1, 0}, {5, 1, 0}, {10, 1, 0}}

	guide.startTour()

	if !guide.touring {
		t.Fatal("startTour did not mark the guide as touring")
	}
	if guide.patrolIndex != 1 {
		t.Errorf("patrolIndex = %d, want 1 - the route's second point", guide.patrolIndex)
	}
	if !guide.object.Sprite().IsPlaying(int(quads.AnimWalk)) {
		t.Error("startTour did not set the guide walking")
	}
}

// A guide with nowhere to go - no route on its map entity - has to stay put
// rather than faulting on patrol[1] of a one-point slice.
func TestStartTourWithNoRouteDoesNothing(t *testing.T) {
	guide := testActor(t, defaultActorHealth, mgl32.Vec3{})
	guide.patrol = []mgl32.Vec3{{0, 1, 0}}

	guide.startTour()

	if guide.touring {
		t.Error("a guide with no route started touring anyway")
	}
}

// A hologram is light, not a body: nothing lands on it. Even a headshot or a
// blast big enough to kill outright must leave it exactly as it was.
func TestHologramIgnoresEveryHit(t *testing.T) {
	hologram := testActor(t, defaultActorHealth, mgl32.Vec3{})
	hologram.kind = &SpriteKind{Hologram: true}

	hologram.TakeDamage(Damage{Headshot: true})
	hologram.TakeDamage(Damage{Amount: 99999})
	hologram.TakeDamage(Damage{})

	if hologram.dying || hologram.Health != defaultActorHealth {
		t.Errorf("hologram took damage: dying=%v health=%d, want untouched at %d", hologram.dying, hologram.Health, defaultActorHealth)
	}
}

// "Actors Active" is a debug switch that freezes ordinary NPC minds - see
// actor_debug.go - and it is off by default in every session, editor or not.
// A touring guide is not an NPC mind wandering on its own initiative; it is
// doing what the player just asked for through its own dialogue, so
// updateActors must not let this switch freeze it. This is the bug the user
// caught: the guide played its walk animation and stood there forever,
// because updateActors froze it back in place the very same frame.
func TestTourRunsEvenWithActorsActiveOff(t *testing.T) {
	game := testGame(t, mgl32.Vec3{5, 1, 0})
	guide := testActor(t, defaultActorHealth, mgl32.Vec3{0, 1, 0})
	guide.patrol = []mgl32.Vec3{{0, 1, 0}, {5, 1, 0}}
	guide.startTour()

	if game.actorsActive {
		t.Fatal("test setup: actorsActive should default to false")
	}
	game.actors = []*Actor{guide}
	game.updateActors(1.0 / 60.0)

	if got := guide.object.Transform().GetPosition(); got == (mgl32.Vec3{0, 1, 0}) {
		t.Error("the guide did not move at all - updateActors froze it despite touring")
	}
}

// A guide leading a tour walks a straight line to its next point rather than
// through quads.PathfindingSystem - see updateTour's own comment for why the
// ark breaks that system's floor scan for every actor on it, guides included.
func TestTourWalksInAStraightLineToEachWaypoint(t *testing.T) {
	game := testGame(t, mgl32.Vec3{0, 1, 0})
	guide := testActor(t, defaultActorHealth, mgl32.Vec3{0, 1, 0})
	guide.patrol = []mgl32.Vec3{{0, 1, 0}, {5, 1, 0}}
	guide.startTour()

	// tourWalkSpeed a second should land it exactly on the waypoint.
	game.player.Transform().SetPosition(guide.object.Transform().GetPosition())
	for i := 0; i < 60; i++ {
		game.updateTour(guide, 1.0/60.0)
	}

	got := guide.object.Transform().GetPosition()
	if !got.ApproxEqualThreshold(mgl32.Vec3{tourWalkSpeed, 1, 0}, 0.01) {
		t.Errorf("position after 1s = %v, want ~%v units along the route", got, mgl32.Vec3{tourWalkSpeed, 1, 0})
	}
}

// A guide leading a tour only takes its next leg once it has arrived at this
// one *and* the player is close enough to have followed - it must not talk to
// an empty corridor.
func TestTourWaitsForThePlayerBeforeTheNextLeg(t *testing.T) {
	game := testGame(t, mgl32.Vec3{0, 1, 0})
	guide := testActor(t, defaultActorHealth, mgl32.Vec3{5, 1, 0}) // already at the first leg
	guide.patrol = []mgl32.Vec3{{0, 1, 0}, {5, 1, 0}, {10, 1, 0}}
	guide.touring = true
	guide.patrolIndex = 1

	// Far off: the guide waits rather than walking on alone.
	game.player.Transform().SetPosition(mgl32.Vec3{5, 1, 100})
	game.updateTour(guide, 1.0/60.0)
	if guide.patrolIndex != 1 {
		t.Fatalf("patrolIndex = %d, want 1 - the player has not caught up yet", guide.patrolIndex)
	}
	if got := guide.object.Transform().GetPosition(); got != (mgl32.Vec3{5, 1, 0}) {
		t.Errorf("position = %v, want it to have stayed put while waiting", got)
	}
	// The bug the user caught: waiting used to call halt(), which turns
	// gravity back on. updateTour never checks a floor is under the guide -
	// see its own comment on why it cannot trust the ark's own floor scan for
	// that - so a guide left standing with gravity on would drop through the
	// first gap under it, which on the ark is often a ladder shaft, and carry
	// on the tour from wherever it landed instead of where its route says.
	if guide.object.Physics().AffectedByGravity() {
		t.Error("a waiting guide has gravity switched back on - it will fall through the first gap under it")
	}

	// Close enough: on to the next leg.
	game.player.Transform().SetPosition(mgl32.Vec3{5, 1, 3})
	game.updateTour(guide, 1.0/60.0)
	if guide.patrolIndex != 2 {
		t.Fatalf("patrolIndex = %d, want 2 - the player caught up", guide.patrolIndex)
	}
	if !guide.touring {
		t.Error("the guide stopped touring before reaching the end of its route")
	}

	// Walk the rest of the way to the far end.
	for i := 0; i < 600 && guide.touring; i++ {
		game.updateTour(guide, 1.0/60.0)
	}

	// Arriving at the far end ends the tour rather than looping it, unlike an
	// ordinary patrol.
	if guide.touring {
		t.Error("the tour did not stop at the end of the route")
	}
	if got := guide.object.Transform().GetPosition(); !got.ApproxEqualThreshold(mgl32.Vec3{10, 1, 0}, 0.01) {
		t.Errorf("final position = %v, want the route's last point", got)
	}
}

// Both of the ark's hologram guides are data, the same way the Scientist's
// conversation is - see TestScientistDialogueLoadsFromDisk. This only checks
// that their files load and that agreeing to the tour actually starts one;
// the words themselves are content, not behaviour.
func TestHologramGuideDialogueStartsATour(t *testing.T) {
	for _, name := range []string{"Purser", "Reyes"} {
		t.Run(name, func(t *testing.T) {
			guide := testActor(t, defaultActorHealth, mgl32.Vec3{})
			guide.Name = name
			guide.patrol = []mgl32.Vec3{{0, 1, 0}, {5, 1, 0}}

			greeting := dialogueFor(guide)
			if greeting == nil {
				t.Fatalf("no dialogue loaded for %s", name)
			}

			// More than one reply on the greeting does something now (asking
			// where the route goes is the other), and which is which is not
			// visible from out here - a Do is an opaque closure. So: every one
			// of them is picked, and one of them has to have started the tour.
			acted := false
			for _, choice := range greeting.Choices {
				if choice.Do == nil {
					continue
				}
				acted = true
				choice.Do()
			}
			if !acted {
				t.Fatal("no reply on the greeting does anything")
			}
			if !guide.touring {
				t.Error("no reply on the greeting starts the tour")
			}
		})
	}
}

// A tour is agreed to in a conversation, and the conversation is still on the
// screen when the reply that started it runs. The world keeps updating behind
// a modal widget, so the guide used to walk its whole route - the ark's is
// thirty metres of it, six seconds at tourWalkSpeed - while the player was
// still reading the line that set it off, and be standing at the far end with
// the tour already over by the time they closed it. What that looked like in
// the game was a guide that stopped at its first stop and had to be asked
// again to go on, since asking again is startTour and startTour starts over.
func TestTourWaitsWhileTheConversationIsStillOnTheScreen(t *testing.T) {
	game := testGame(t, mgl32.Vec3{0, 1, 0})
	guide := testActor(t, defaultActorHealth, mgl32.Vec3{0, 1, 0})
	guide.kind = &SpriteKind{Guide: true}
	guide.patrol = []mgl32.Vec3{{0, 1, 0}, {5, 1, 0}}
	guide.startTour()
	game.actors = []*Actor{guide}

	game.widgetsAreBlockingMovement = true
	game.updateActors(1.0)
	if got := guide.object.Transform().GetPosition(); got != (mgl32.Vec3{0, 1, 0}) {
		t.Errorf("the guide walked to %v with the dialogue still up, want it waiting at its spawn", got)
	}

	game.widgetsAreBlockingMovement = false
	game.updateActors(1.0)
	if got := guide.object.Transform().GetPosition(); got == (mgl32.Vec3{0, 1, 0}) {
		t.Error("the guide did not set off once the conversation was closed")
	}
}

// The other half of tourIdle's bug, at the end of the route rather than in the
// middle of it: the guide drops out of touring and into the ordinary actor
// path, which freezes it - and freeze switches gravity back on. Both of the
// ark's tours end in mid-air in the spine shaft, at the foot of the ladder up,
// so that last frame of the tour was followed by a thirteen-voxel fall down
// the shaft. A guide is never an ordinary actor: its route is not a patrol
// beat the pathfinder can walk either.
func TestAGuideThatHasFinishedItsTourIsLeftWhereTheRouteEnded(t *testing.T) {
	game := testGame(t, mgl32.Vec3{5, 1, 0})
	guide := testActor(t, defaultActorHealth, mgl32.Vec3{5, 1, 0})
	guide.kind = &SpriteKind{Guide: true}
	guide.patrol = []mgl32.Vec3{{0, 1, 0}, {5, 1, 0}}
	guide.touring = true
	guide.patrolIndex = 1
	game.actors = []*Actor{guide}

	game.updateActors(1.0 / 60.0) // arrives at the last point: the tour is over
	if guide.touring {
		t.Fatal("test setup: the guide should have finished its route on the first frame")
	}

	game.updateActors(1.0 / 60.0)
	if guide.object.Physics().AffectedByGravity() {
		t.Error("gravity is back on for a guide stood in mid-air at the end of its route - it will fall down the shaft")
	}
	if guide.pathing.Enabled {
		t.Errorf("the guide's route was handed to the pathfinder as a patrol beat, walking to %v", guide.pathing.Destination())
	}
	if got := guide.object.Transform().GetPosition(); got != (mgl32.Vec3{5, 1, 0}) {
		t.Errorf("position = %v, want the route's last point", got)
	}
}

// A guide walks its route by hand - straight from one point to the next, see
// updateTour - rather than through the physics system, and the collision grid
// only hears about a move the system itself made. So the guide was left listed
// at its spawn for the rest of the level: the picking ray reads that grid and
// nothing else, so the crosshair found nothing wherever the guide was actually
// standing, and a guide that cannot be picked out is one that cannot be talked
// to again.
func TestAGuideOnTourIsStillSelectableWhereItWalkedTo(t *testing.T) {
	game := testGame(t, mgl32.Vec3{0, 1, 0})
	guide := testActor(t, defaultActorHealth, mgl32.Vec3{0, 1, 0})
	guide.kind = &SpriteKind{Guide: true}
	guide.patrol = []mgl32.Vec3{{0, 1, 0}, {5, 1, 0}}
	guide.touring = true
	guide.patrolIndex = 1
	game.actors = []*Actor{guide}

	physics := guide.object.Physics()
	guide.physicsSystem.AddObject(physics)

	game.updateActors(1.0)

	at := guide.object.Transform().GetPosition()
	if at.X() == 0 {
		t.Fatal("test setup: the guide did not walk at all")
	}
	if !guide.physicsSystem.GetCollidersAt(int32(at.X()), int32(at.Y()), int32(at.Z()))[physics] {
		t.Errorf("the guide walked to %v and is not on the collision grid there - the crosshair cannot pick it out, so it cannot be talked to", at)
	}
	if guide.physicsSystem.GetCollidersAt(0, 1, 0)[physics] {
		t.Error("the guide is still listed on the grid at the spawn it walked away from")
	}
}

// "Where are we going?" is answered out of where the guide has actually got
// to, so the reply writes the line it is about to show - see the waypoint
// entry in dialogueActions.
func TestAGuideSaysWhichStopOfTheTourItIsOn(t *testing.T) {
	guide := testActor(t, defaultActorHealth, mgl32.Vec3{0, 1, 0})
	guide.patrol = []mgl32.Vec3{{0, 1, 0}, {5, 1, 0}, {10, 1, 0}}

	before := guide.waypointText()
	guide.startTour()
	walking := guide.waypointText()
	if walking == before {
		t.Errorf("the guide says %q both before the tour and on its way to the first stop", walking)
	}
	if !strings.Contains(walking, "1 of 2") {
		t.Errorf("on the first leg of two the guide says %q", walking)
	}

	guide.patrolIndex = 2
	guide.touring = false
	if done := guide.waypointText(); done == walking || !strings.Contains(done, "2 of 2") {
		t.Errorf("at the end of the route the guide says %q", done)
	}
}
