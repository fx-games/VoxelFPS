package main

import (
	"fmt"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
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
	goFromEntity := quads.NewGameObject(entity.Name)
	goFromEntity.Transform().SetPosition(spawnPos)

	var aabb voxel.AABB

	if entity.MeshName != "" {
		meshFile := filepath.Join("assets", "meshes", entity.MeshName+".glb")
		if !quads.FileExists(meshFile) {
			panic("Mesh file not found: " + meshFile)
		}
		mesh, meshAABB := g.Meshes.NewMesh(meshFile)
		aabb = meshAABB

		goFromEntity.SetComponent(mesh)
		g.Meshes.Add(mesh)
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

	if entity.Name == "Scientist" { // TODO: REPLACE
		sprite := goFromEntity.Sprite()
		atlas := sprite.AtlasTexture
		dyingAABB := voxel.NewAABB(
			atlas.AABBFromOpaqueAtIndex(int(g.Sprites.GetLastFrameIndex(int(quads.AnimDying), 0))),
		)

		physics.SetAABBForAnimation(quads.AnimDying, dyingAABB)

		pather := quads.NewPathingComponent()
		// The far end of the beat used to be this one literal, which only made sense
		// for the LDtk test map. A map that wants a different one says so.
		destination := mgl32.Vec3{5.5, 1, 2.5}
		if entity.PatrolTo != nil {
			destination = *entity.PatrolTo
		}
		pather.SetDestination(destination)
		goFromEntity.SetComponent(pather)

		g.Pathfinder.AddMovingObject(pather)
	}

	if entity.MeshName == stairsMeshName {
		// The mesh is one box to the drawer and would be one box to physics too.
		// Stairs need a collider per step, see stairColliders.
		for _, step := range stairColliders(spawnPos) {
			g.physicsSystem.AddObject(step)
		}
	}

	if entity.CreateCollider {
		g.physicsSystem.AddObject(physics)
	}

	return goFromEntity
}
