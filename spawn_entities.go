package main

import (
	"fmt"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/geometry"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/glhf"
	"github.com/memmaker/goray/quads/voxel"
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
	goFromEntity := quads.NewGameObject(entity.Name)
	goFromEntity.Transform().SetPosition(spawnPos)
	if entity.Facing.Len() > 0 {
		goFromEntity.Transform().SetForward2D(entity.Facing)
	}

	var aabb voxel.AABB

	if entity.MeshName != "" {
		meshFile := g.meshFile(entity.MeshName)
		if !quads.FileExists(meshFile) {
			panic("Mesh file not found: " + meshFile)
		}
		mesh, meshAABB := g.Meshes.NewMesh(meshFile)
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

		if entity.SpriteSheetName != "" {
			atlasFile := filepath.Join("assets", "sheets", entity.SpriteSheetName+".png") //"assets/scientist_sheet.png"
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

		aabb = voxel.NewAABB(atlas.AABBFromOpaqueAtIndex(int(entity.TextureIndices[0])))

		goFromEntity.SetComponent(sprite)
		if entity.IsTransparent {
			g.Sprites.AddTransparent(sprite)
		} else {
			g.Sprites.Add(sprite)
		}

	}
	physics := quads.NewPhysicsComponent(false, aabb)
	goFromEntity.SetComponent(physics)

	if actorNames[entity.Name] {
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
	if _, isScenery := g.scenery[entity.MeshName]; isScenery && !interactableMesh(entity.MeshName) {
		g.furniture[goFromEntity] = true
	}

	return goFromEntity
}

// weaponRangeEntity is what the map calls the marker the test range is laid
// out from. The map decides where the range is; the engine decides what goes
// in it, which is every weapon the folder holds.
const weaponRangeEntity = "WeaponRange"

// interactableMesh is the scenery the E key means something to: it is placed
// out of the same palette as a table, and unlike a table it is used.
func interactableMesh(meshName string) bool {
	switch meshName {
	case doorSlideMeshName, doorSwingLeafMeshName, keypadMeshName, noteMeshName, audioLogMeshName:
		return true
	}
	return false
}
