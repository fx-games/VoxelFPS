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
			screenDir := filepath.Join(sceneryDir, entity.MeshName, "screen")
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
	}

	switch entity.MeshName {
	case doorSlideMeshName:
		g.spawnDoor(goFromEntity, doorSlide, spawnPos)
	case doorSwingLeafMeshName:
		g.spawnDoor(goFromEntity, doorSwing, spawnPos)
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
	if _, isScenery := g.scenery[entity.MeshName]; isScenery {
		g.furniture[goFromEntity] = true
	}

	return goFromEntity
}

// weaponRangeEntity is what the map calls the marker the test range is laid
// out from. The map decides where the range is; the engine decides what goes
// in it, which is every weapon the folder holds.
const weaponRangeEntity = "WeaponRange"
