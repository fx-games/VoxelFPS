package main

import (
    "github.com/go-gl/mathgl/mgl32"
    "github.com/memmaker/goray/core"
    "github.com/memmaker/goray/quads"
    "github.com/memmaker/goray/quads/voxel"
    "path/filepath"
    "strings"
)

func (g *Game) fireProjectile() {
    origin, direction := g.Camera.GetPickingRayFromScreenPosition(0.5, 0.5)
    offsetOrigin := origin.Add(direction.Mul(0.8))

    sprite := quads.NewSprite(g.defaultAtlas, []uint16{179})
    newProjectile := quads.NewGameObject("Projectile")
    newProjectile.SetLifeTime(5)
    newProjectile.Transform().SetPosition(offsetOrigin)
    newProjectile.SetComponent(sprite)

    physics := quads.NewPhysicsComponent(false, voxel.NewAABB(mgl32.Vec3{-0.1, -0.1, -0.1}, mgl32.Vec3{0.1, 0.1, 0.1}))
    newProjectile.SetComponent(physics)

    g.Sprites.Add(sprite)
    sprite.PlayAnimationLooping(g.projectileFlyingAnim)

    g.physicsSystem.AddObject(physics)
    physics.SetVelocity(direction.Mul(10))
}

func (g *Game) spawnProjectileImpact(worldPos mgl32.Vec3, anim int) {
    firstFrameIndex := g.Sprites.GetFirstFrameIndex(anim, 0)
    sprite := quads.NewSprite(g.defaultAtlas, []uint16{firstFrameIndex})
    newProjectile := quads.NewGameObject("Impact")
    newProjectile.Transform().SetPosition(worldPos)
    newProjectile.SetComponent(sprite)

    g.Sprites.Add(sprite)

    sprite.PlayAnimationDieAfterLastFrame(anim)
}

func (g *Game) spawnPlayer(cam *core.FPSCamera, spawn mgl32.Vec3, dir mgl32.Vec2) *quads.GameObject {
    startPos := spawn
    cam.SetForward2D(mgl32.Vec3{float32(dir.X()), 0, float32(dir.Y())}.Normalize())
    player := quads.NewGameObject("Player")
    player.Transform().SetPosition(startPos)

    physics := quads.NewPhysicsComponent(true, voxel.NewAABB(
        mgl32.Vec3{-0.2, 0.0, -0.2},
        mgl32.Vec3{0.2, 0.9, 0.2},
    ))
    player.SetComponent(physics)
    player.SetComponent(quads.NewCameraComponent(cam, 0.7))

    g.physicsSystem.AddObject(physics)
    return player
}

func (g *Game) spawnDecorationFromFile(spawnPos mgl32.Vec3, filename string) *quads.GameObject {
    lowerExt := strings.ToLower(filepath.Ext(filename))
    name := filepath.Base(filename)[:len(filepath.Base(filename))-len(lowerExt)]

    goDeco := quads.NewGameObject(name)
    goDeco.Transform().SetPosition(spawnPos)

    if lowerExt == ".png" {
        spriteTexture := quads.LoadTexture(filename)
        sprite := quads.NewSpriteFloor(spriteTexture, []uint16{0})
        goDeco.SetComponent(sprite)
        g.Sprites.Add(sprite)
        physics := quads.NewPhysicsComponent(false, voxel.NewAABB(spriteTexture.AABBFromOpaque()))
        goDeco.SetComponent(physics)
        g.physicsSystem.AddObject(physics)
    } else if lowerExt == ".glb" {
        mesh, meshAABB := g.Meshes.NewMesh(filename)
        goDeco.SetComponent(mesh)
        g.Meshes.Add(mesh)
        physics := quads.NewPhysicsComponent(false, meshAABB)
        goDeco.SetComponent(physics)
        g.physicsSystem.AddObject(physics)
    }
    return goDeco
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
        atlas := g.defaultAtlas

        if entity.SpriteSheetName != "" {
            atlasFile := filepath.Join("assets", "sheets", entity.SpriteSheetName+".png") //"assets/scientist_sheet.png"
            tileWidth := entity.SpriteSheetTileSize.X
            tileHeight := entity.SpriteSheetTileSize.Y
            atlas = quads.LoadTextureAtlas(atlasFile, tileWidth, tileHeight)
        }

        sprite := quads.NewSpriteFloor(atlas, entity.TextureIndices)

        aabb = voxel.NewAABB(atlas.AABBFromOpaqueAtIndex(entity.TextureIndices[0]))

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
            atlas.AABBFromOpaqueAtIndex(g.Sprites.GetLastFrameIndex(int(quads.AnimDying), 0)),
        )

        physics.SetAABBForAnimation(quads.AnimDying, dyingAABB)

        pather := quads.NewPathingComponent()
        pather.SetDestination(mgl32.Vec3{5.5, 1, 2.5})
        goFromEntity.SetComponent(pather)

        g.Pathfinder.AddMovingObject(pather)
    }

    g.physicsSystem.AddObject(physics)

    return goFromEntity
}
