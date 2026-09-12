package main

import (
	"fmt"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/geometry"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
	"log"
	"path/filepath"
)

func (g *Game) spawnProjectileImpact(worldPos mgl32.Vec3, atlas *glhf.TextureAtlasFixed, spriteAnimationIndex int) {
	firstFrameIndex := g.Sprites.GetFirstFrameIndex(spriteAnimationIndex, 0)
	sprite := quads.NewSprite(atlas, []uint16{firstFrameIndex})
	newProjectile := quads.NewGameObject("Impact")
	newProjectile.Transform().SetPosition(worldPos)
	newProjectile.SetComponent(sprite)

	g.Sprites.Add(sprite)

	sprite.PlayAnimationDieAfterLastFrame(spriteAnimationIndex)
}

func (g *Game) spawnPlayer(cam *core.FPSCamera, spawn mgl32.Vec3, dir mgl32.Vec2) *quads.GameObject {
	startPos := spawn
	cam.SetForward2D(mgl32.Vec3{float32(dir.X()), 0, float32(dir.Y())}.Normalize())
	player := quads.NewGameObject("Player")
	player.Transform().SetPosition(startPos)

	// Doom's player is 56 units tall and looks out of it at 41. Everything a
	// weapon spawns is placed relative to those two numbers, so they are the
	// ones to be, rather than approximately them.
	physics := quads.NewPhysicsComponent(true, voxel.NewAABB(
		mgl32.Vec3{-0.2, 0.0, -0.2},
		mgl32.Vec3{0.2, playerHeight, 0.2},
	))
	player.SetComponent(physics)
	player.SetComponent(quads.NewCameraComponent(cam, float64(playerEyeHeight)))

	g.physicsSystem.AddObject(physics)
	return player
}

func (g *Game) spawnEntity(spawnPos mgl32.Vec3, entity core.MapEntity) *quads.GameObject {
	if entity.Name == weaponRangeEntity {
		// Not a thing in the world: a corner the weapons are laid out from.
		g.spawnWeaponRange(spawnPos)
		return nil
	}
	if entity.WeaponName != "" {
		// A weapon lying on the floor is not built like the other entities: its
		// picture is in the shared store rather than in an atlas of its own, and
		// what it is worth to the player is on the weapon rather than on the
		// entity. spawnPickup is the one path that knows both.
		index, known := g.weaponByID[entity.WeaponName]
		if !known {
			log.Printf("[map] no weapon named %q to put on the floor", entity.WeaponName)
			return nil
		}
		return g.spawnPickup(g.weapons[index], entity.WeaponAmmo, spawnPos)
	}

	goFromEntity := quads.NewGameObject(entity.Name)
	goFromEntity.Transform().SetPosition(spawnPos)
	if entity.Facing.Len() > 0 {
		// Flat on the floor for nearly everything, which is what SetForward2D
		// is for. A facing that points up or down is a piece turned out of that
		// plane - a floor hatch is a slide door lying down - and has to keep its
		// Y, so it takes the 3D one.
		if entity.Facing.Y() != 0 {
			goFromEntity.Transform().SetForward(entity.Facing)
		} else {
			goFromEntity.Transform().SetForward2D(entity.Facing)
		}
	}

	var aabb voxel.AABB

	if entity.MeshName != "" {
		meshFile := g.meshFile(entity.MeshName)
		if !quads.FileExists(meshFile) {
			panic("Mesh file not found: " + meshFile)
		}
		mesh, meshAABB := g.Meshes.NewMesh(meshFile)
		// A sign's picture is its writing, so a real one gets a model of its
		// own to paint; the editor's ghost shares the blank one.
		if entity.MeshName == signMeshName && entity.CreateCollider {
			mesh, meshAABB = g.Meshes.NewOwnMesh(meshFile)
		}
		aabb = meshAABB

		goFromEntity.SetComponent(mesh)
		g.Meshes.Add(mesh)

		if piece, isScenery := g.scenery[entity.MeshName]; isScenery && piece.Screen != nil {
			fps := piece.Screen.FPS
			if fps <= 0 {
				fps = 4
			}
			screenDir := filepath.Join(meshDir, entity.MeshName, "screen")
			if err := g.Meshes.EnsureScreenAnimation(mesh.MeshIndex, piece.Screen.Material, screenDir, 1/fps); err != nil {
				panic(fmt.Sprintf("scenery %s: %s", entity.MeshName, err))
			}
		}
	}

	if len(entity.TextureIndices) > 0 {
		atlas := g.AtlasSprites
		// A sprite sheet names a kind under assets/sprites/ - a guard, a
		// mutant, a barrel - which brings its own picture, its own frame size
		// and its own animations with it. A name with no folder behind it is a
		// sheet in the older assets/sheets/ sense, kept working so a map saved
		// before the folders existed still loads.
		kind := g.spriteKinds[entity.SpriteSheetName]
		if kind != nil {
			atlas = kind.Atlas()
		} else if entity.SpriteSheetName != "" {
			atlasFile := filepath.Join("assets", "sheets", entity.SpriteSheetName+".png")
			tileWidth := entity.SpriteSheetTileSize.X
			tileHeight := entity.SpriteSheetTileSize.Y
			atlas = quads.Disk.TextureAtlasFixed(atlasFile, tileWidth, tileHeight)
		}
		for i, index := range entity.TextureIndices {
			if int(index) > atlas.LastValidIndex() {
				println(fmt.Sprintf("Invalid texture index %d for entity %s", index, entity.Name))
				entity.TextureIndices[i] = 0
			}
		}
		sprite := quads.NewSpriteFloor(atlas, entity.TextureIndices)

		if kind != nil {
			// The kind's own six animations, and the size the art was drawn
			// for. Everything downstream keeps saying AnimWalk; the base is
			// what makes that this kind's walk - see SpriteComponent.AnimBase.
			sprite.AnimBase = kind.animBase
			sprite.Size = kind.Size()
			// A decoration with more than one idle frame is an animation - a
			// flickering barrel, a jet of steam - and has to be started, or it
			// stands on frame zero forever. An actor's own idle is started by
			// spawnActor along with everything else it does.
			if !kind.Actor {
				sprite.PlayAnimationLooping(int(quads.AnimIdle))
			}
		}

		// The box comes off the picture rather than off a number: the opaque
		// part of what it draws, at the size the quad is drawn. That is what a
		// barrel has always been picked and bumped into by, and a guard is a
		// barrel that walks.
		//
		// An imported kind brings one box per animation, measured off its sheet
		// at import time - see SpriteKind.box. Anything else is measured here,
		// off the card, from the one frame there is to measure.
		if kind != nil {
			if measured, ok := kind.box("idle"); ok {
				aabb = measured
			} else {
				aabb = quads.SpriteAABB(atlas, int(entity.TextureIndices[0]), sprite.Size)
			}
		} else {
			aabb = quads.SpriteAABB(atlas, int(entity.TextureIndices[0]), sprite.Size)
		}

		goFromEntity.SetComponent(sprite)
		if entity.IsTransparent {
			g.Sprites.AddTransparent(sprite)
		} else {
			g.Sprites.Add(sprite)
		}

	}
	physics := quads.NewPhysicsComponent(false, aabb)
	// A thing the world walks through, but still on the grid: the crosshair
	// finds it, the OBB outlines it, and nothing is stopped by it. See
	// core.MapEntity.PassThrough - it is how a hall of exhibits is a hall
	// rather than a maze.
	physics.PassThrough = entity.PassThrough
	goFromEntity.SetComponent(physics)

	// And the other five, so that the box follows what is being drawn: a guard
	// mid-lunge is wider than a guard standing, and the body he falls into is
	// a different shape again. PhysicsComponent keys them by the sprite's own
	// animation number - see LocalAnimation - which is what makes AnimDying
	// mean this kind's death rather than some other kind's slot.
	if kind := g.spriteKinds[entity.SpriteSheetName]; kind != nil {
		for anim := quads.AnimIndex(0); anim < quads.AnimCount; anim++ {
			if measured, ok := kind.box(animOrder[anim]); ok {
				physics.SetAABBForAnimation(anim, measured)
			}
		}
		// And the box of every picture it is drawn as, for the ray rather than
		// the solver: a bird with its wings out is twice the bird it is with
		// them folded, and one box for both is one the crosshair picks out
		// beside itself. See SpriteKind.pickBoxes.
		physics.SetPickBoxes(kind.pickBoxes())
	}

	// What walks is what its own folder says walks: a map entity naming a
	// sprite kind marked actor gets an Actor, anything else is a prop standing
	// in the level. The name in the map file is a label, not the decision.
	if kind := g.spriteKinds[entity.SpriteSheetName]; kind != nil && kind.Actor {
		g.spawnActor(goFromEntity, physics, entity, spawnPos)
	} else if len(entity.TextureIndices) > 0 && entity.MeshName == "" {
		// A sprite decoration - a barrel, a crate, whatever the atlas holds -
		// is a thing standing in the level rather than part of it, and it falls
		// like the rest of them. It stays solid, so the player is stopped at
		// its surface instead of ending up inside it.
		physics.ApplyGravity = true
	}

	// The ghost that follows the editor's crosshair comes through here too, and
	// it is the one entity with no collider - so that is what tells a thing
	// standing in the level from a preview of one. A ghost door registered as a
	// Door would animate, and a ghost keypad would hold a door shut.
	switch entity.MeshName {
	case doorSlideMeshName:
		if entity.CreateCollider {
			spawnPos = slideDoorPlacement(goFromEntity, spawnPos)
			goFromEntity.Transform().SetPosition(spawnPos)
			g.spawnDoor(goFromEntity, doorSlide, spawnPos)
		}
	case doorSwingLeafMeshName:
		if entity.CreateCollider {
			g.spawnDoor(goFromEntity, doorSwing, spawnPos)
		}
	case keypadMeshName:
		// The panel's display is the only bright, saturated thing on it, and
		// the emissive pass picks texels out by exactly that - so this lights
		// the readout and leaves the casing round it dark.
		goFromEntity.Mesh().EmissiveStrength = keypadEmissiveStrength
		if entity.CreateCollider {
			g.spawnKeypad(goFromEntity, entity.Code)
		}
	case noteMeshName:
		// A note is walked over rather than bumped into, the way a pickup is:
		// on the physics grid so the selection beam finds it, skipped by the
		// collision solver so nothing trips over a piece of paper.
		physics.PassThrough = true
		// A note is an item, and an item falls - see spawnPickup.
		physics.ApplyGravity = true
		if entity.CreateCollider {
			g.spawnNote(goFromEntity, entity.Text)
		}
	case audioLogMeshName:
		// A cartridge is picked up the way a note is: on the grid for the
		// selection beam, walked through by the solver, and it falls.
		physics.PassThrough = true
		physics.ApplyGravity = true
		// Its running light, and only its running light - see the keypad above
		// for the same trick and why the shell around it stays dark.
		goFromEntity.Mesh().EmissiveStrength = audioLogEmissiveStrength
		if entity.CreateCollider {
			g.spawnAudioLog(goFromEntity, entity.Sound, entity.Text)
		}
	case signMeshName:
		goFromEntity.Mesh().EmissiveStrength = signEmissiveStrength
		if entity.CreateCollider {
			g.spawnSign(goFromEntity, entity.Text)
		}
	case torchMeshName:
		// A torch head is bright on screen wherever the torch came from, the
		// map or the ghost on the editor's crosshair. Its light *in the map* is
		// not: only a torch that has actually been dropped floods the voxels
		// around it, see dropTorch.
		goFromEntity.Mesh().EmissiveStrength = torchEmissiveStrength
	}

	if piece, isScenery := g.scenery[entity.MeshName]; isScenery && len(piece.Climbable) > 0 {
		g.registerClimbable(goFromEntity, piece, entity.ClimbTo)
	}

	// A staircase (or anything else bridging an elevation change) tells
	// pathfinding about the two tiles it connects; see core.MapEntity.StepTo.
	if entity.StepTo != nil && g.Pathfinder != nil {
		from := geometry.Point{X: int(spawnPos.X()), Y: int(spawnPos.Z())}
		to := geometry.Point{X: int(entity.StepTo.X()), Y: int(entity.StepTo.Z())}
		g.Pathfinder.AddWalkLink(from, to)
	}

	// A piece of scenery that names its own boxes collides as those instead of
	// as the one the mesh drawer hands out. The mesh is one box to the drawer
	// and would be one box to physics too: stairs need a box per step or they
	// are a wall, and a shelf needs an open front or nothing on it can be
	// reached. They all belong to this one object - see Scenery.colliders - so
	// a shelf is still one thing in the world with one model and one name.
	if entity.CreateCollider {
		if piece, isScenery := g.scenery[entity.MeshName]; isScenery && len(piece.Colliders) > 0 {
			for _, shape := range piece.colliders(goFromEntity) {
				g.physicsSystem.AddObject(shape)
			}
		} else {
			g.physicsSystem.AddObject(physics)
		}
	}
	// Furniture is what the selection looks through. A door, a keypad and a note
	// are all pieces of scenery by the folder they live in - which is what puts
	// them on the editor's placement palette - but every one of them is a thing
	// to press E on, so none of them may be looked through.
	//
	// A piece that can be walked through is not holding anything up, so there is
	// nothing behind it to look at: it is an exhibit, and it is selected like
	// anything else.
	if _, isScenery := g.scenery[entity.MeshName]; isScenery && !interactableMesh(entity.MeshName) && !entity.PassThrough {
		g.furniture[goFromEntity] = true
	}

	return goFromEntity
}

// slideDoorPlacement puts a leaf where the voxel it was placed in wants it.
//
// The model stands on its own origin: a metre wide across its face and a metre
// tall up from that point, which is exactly the doorway an upright door was
// placed in the floor of. Laid flat as a floor hatch that same metre runs along
// the deck instead, so the leaf reaches a whole voxel out of the hole on one
// side and covers none of it on the other - the half-open-looking hatch - and
// lies a voxel below the deck it is a hatch in, which is a pit to stand in.
//
// A flat leaf is therefore centred on its voxel and lifted to that voxel's top
// face, which is the surface it is a hatch in: shut it is flush with the deck,
// and open it has slid its own width aside and is lying on the plate next to
// the hole. An upright door is already where its model wants to be and is left
// alone.
func slideDoorPlacement(object *quads.GameObject, spawnPos mgl32.Vec3) mgl32.Vec3 {
	up := object.Transform().GetRotation().Rotate(mgl32.Vec3{0, 1, 0})
	if mgl32.Abs(up.Y()) > 0.001 {
		return spawnPos // upright, or as near as makes no difference
	}
	return spawnPos.Add(mgl32.Vec3{0, 1, 0}).Sub(up.Mul(0.5))
}

// weaponRangeEntity is what the map calls the marker the test range is laid
// out from. The map decides where the range is; the engine decides what goes
// in it, which is every weapon the folder holds.
const weaponRangeEntity = "WeaponRange"

// interactableMesh is the scenery the E key means something to: it is placed
// out of the same palette as a table, and unlike a table it is used.
func interactableMesh(meshName string) bool {
	switch meshName {
	case doorSlideMeshName, doorSwingLeafMeshName, keypadMeshName, noteMeshName, audioLogMeshName, signMeshName:
		return true
	}
	return false
}
