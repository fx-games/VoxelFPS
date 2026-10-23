package voxel

import (
	"math"

	lv "github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/physics"

	"github.com/go-gl/mathgl/mgl32"
)

type HitInfo2D struct {
	Distance               float64
	Side                   CubeSide
	CollisionX, CollisionY float64
	GridX                  int64
	GridY                  int64
}

func Raycast2D(startX, startY, directionX, directionY float64, shouldStopRay func(x, y int64) bool) HitInfo2D {
	// 2. Die sich wiederholenden Strahlenteile für Schritte in CollisionX und CollisionY Richtung berechnen
	//    - Steigung des Richtungsvektors und ein Schritt in CollisionX bzw. CollisionY Richtung
	//    - Satz des Pythagoras mit a = 1 und b = ray.y / ray.x für einen Schritt in die CollisionX-Richtung
	//    - Satz des Pythagoras mit a = 1 und b = ray.x / ray.y für einen Schritt in die CollisionY-Richtung
	deltaDistX := math.Sqrt(1 + (directionY*directionY)/(directionX*directionX))
	deltaDistY := math.Sqrt(1 + (directionX*directionX)/(directionY*directionY))

	startGridX := int64(math.Floor(startX))
	startGridY := int64(math.Floor(startY))

	mapX := startGridX
	mapY := startGridY

	var intraCellPositionX float64
	var intraCellPositionY float64

	var sideDistX float64
	var sideDistY float64

	var mapStepX int64
	var mapStepY int64

	// 3. Anhand der Richtung des Strahls folgende Werte berechnen
	//     - Die Schrittrichtung auf der Karte (je Quadrant)
	//     - Die translation innerhalb der aktuellen Zelle
	//     - Die anteiligen Startteile der beiden Strahlen für CollisionX und CollisionY Schritte
	if directionX < 0 {
		mapStepX = -1
		intraCellPositionX = startX - float64(mapX)
		sideDistX = intraCellPositionX * deltaDistX
	} else {
		mapStepX = 1
		intraCellPositionX = float64(mapX) + 1.0 - startX
		sideDistX = intraCellPositionX * deltaDistX
	}

	if directionY < 0 {
		mapStepY = -1
		intraCellPositionY = startY - float64(mapY)
		sideDistY = intraCellPositionY * deltaDistY
	} else {
		mapStepY = 1
		intraCellPositionY = float64(mapY) + 1.0 - startY
		sideDistY = intraCellPositionY * deltaDistY
	}

	eastWestSide := false
	stopRaycasting := false
	nextCollisionX := 0.0
	nextCollisionY := 0.0

	// 4. Schrittweise die Strahlen verlängern, immer der kürzeste zuerst
	//    - Die Kartenposition wird aktualisiert
	//    - Die Wandrichtung (Nord/Süd vs. Ost/west) wird gesetzt
	//    - Der Strahl wird verlängert
	for !stopRaycasting {
		if sideDistX < sideDistY {
			// move one unit in CollisionX Direction
			nextCollisionX = startX + (directionX * sideDistX)
			nextCollisionY = startY + (directionY * sideDistX)

			mapX += mapStepX
			eastWestSide = true
			sideDistX += deltaDistX
		} else {
			// move one unit in CollisionY Direction
			nextCollisionX = startX + (directionX * sideDistY)
			nextCollisionY = startY + (directionY * sideDistY)

			mapY += mapStepY
			eastWestSide = false
			sideDistY += deltaDistY
		}

		// Optional: Sprites - Prüfen ob Sprites auf diesem Feld sind
		//Sprite visibleSprite = GetSpriteMapAt(mapX, mapY);
		/*
		   if (visibleSprite != null && !mVisibleSprites.Contains(visibleSprite))
		       mVisibleSprites.AddFlat(visibleSprite);
		*/

		// 5. Prüfen wir ob eine Wand getroffen wurde
		stopRaycasting = shouldStopRay(mapX, mapY)
	}

	// 6. Den Abstand zur Wand berechnen (Im Prinzip den letzten Schritt rückgängig machen)
	// Vektoren subtrahieren
	var perpWallDist float64
	if eastWestSide {
		perpWallDist = sideDistX - deltaDistX
	} else {
		perpWallDist = sideDistY - deltaDistY
	}

	// 6.1 Die original Distanz zur Wand für das Texture Mapping verwenden
	var wallX float64 // CollisionX translation an der wir die Wand getroffen haben
	// Komponentenweise Vektoraddition und Skalierung
	if eastWestSide {
		wallX = startY + (perpWallDist * directionY)
	} else {
		wallX = startX + (perpWallDist * directionX)
	}

	wallX -= math.Floor(wallX) // Uns interessieren nur die Nachkommastellen
	//textureX := (int)(wallX * mWallTextures[textureIndex].Width);

	var hitSide CubeSide
	if eastWestSide {
		if directionX < 0 {
			hitSide = West
		} else {
			hitSide = East
		}
	} else {
		if directionY < 0 {
			hitSide = North
		} else {
			hitSide = South
		}
	}
	return HitInfo2D{
		Distance:   perpWallDist,
		Side:       hitSide,
		CollisionX: nextCollisionX,
		CollisionY: nextCollisionY,
		GridX:      mapX,
		GridY:      mapY,
	}
}

type CubeSide uint8

const (
	// DON'T CHANGE THE ORDER
	East CubeSide = iota
	West
	Top
	Bottom
	South
	North
)

func (s CubeSide) String() string {
	switch s {
	case North:
		return "North"
	case East:
		return "East"
	case South:
		return "South"
	case West:
		return "West"
	case Top:
		return "Top"
	case Bottom:
		return "Bottom"
	}
	return "Unknown"
}

func (s CubeSide) AsDirection() Int3 {
	switch s {
	case North:
		return Int3{0, 0, -1}
	case East:
		return Int3{1, 0, 0}
	case South:
		return Int3{0, 0, 1}
	case West:
		return Int3{-1, 0, 0}
	case Top:
		return Int3{0, 1, 0}
	case Bottom:
		return Int3{0, -1, 0}
	}
	return Int3{}
}

type HitInfo3D struct {
	Distance               float64
	Side                   CubeSide
	CollisionWorldPosition mgl32.Vec3
	PreviousGridPosition   Int3
	CollisionGridPosition  Int3
	Hit                    bool
}

func (d HitInfo3D) WithCollisionWorldPosition(point mgl32.Vec3) HitInfo3D {
	d.CollisionWorldPosition = point
	return d
}

func (d HitInfo3D) WithDistance(newDist float64) HitInfo3D {
	d.Distance = newDist
	return d
}

// DDARaycast walks the voxel grid along the ray (VoxelLib physics.Walk) until
// stopRay reports a solid voxel or maxDistance is exceeded. maxDistance is
// measured in rayDir lengths, so pass a normalized direction to get world
// units. A ray that reaches maxDistance without a hit returns HitInfo3D{Hit: false}.
func DDARaycast(rayStart, rayDir mgl32.Vec3, maxDistance float64, stopRay func(x, y, z int32) bool) HitInfo3D {
	length := float64(rayDir.Len())
	if length == 0 {
		return HitInfo3D{}
	}
	u := rayDir.Mul(float32(1 / length))
	var hit HitInfo3D
	physics.Walk(nil, float64(rayStart.X()), float64(rayStart.Y()), float64(rayStart.Z()), float64(u.X()), float64(u.Y()), float64(u.Z()), maxDistance*length,
		func(x, y, z int, _ lv.StateID, f lv.Face, t float64) bool {
			if !stopRay(int32(x), int32(y), int32(z)) {
				return false
			}
			cell := Int3{X: int32(x), Y: int32(y), Z: int32(z)}
			var side CubeSide
			var previous Int3
			if t > 0 { // zero: the ray starts inside the cell
				side = faceSide[f]
				previous = cell.Add(side.AsDirection())
			}
			hit = HitInfo3D{
				Hit:                    true,
				Distance:               t / length,
				Side:                   side,
				CollisionWorldPosition: rayStart.Add(u.Mul(float32(t))),
				PreviousGridPosition:   previous,
				CollisionGridPosition:  cell,
			}
			return true
		})
	return hit
}

// FaceSide is the side of a cell a ray enters through face f.
func FaceSide(f lv.Face) CubeSide { return faceSide[f] }

// faceSide is the side of a cell a ray enters it through.
var faceSide = [lv.FaceCount]CubeSide{
	lv.FaceDown:  Bottom,
	lv.FaceUp:    Top,
	lv.FaceNorth: North,
	lv.FaceSouth: South,
	lv.FaceWest:  West,
	lv.FaceEast:  East,
}
