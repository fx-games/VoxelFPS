package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/goray/core"
	"github.com/memmaker/goray/geometry"
	"github.com/memmaker/goray/quads"
	"github.com/memmaker/goray/quads/voxel"
)

// Scenery is a piece of furniture the world is furnished with: a folder under
// scenery/ holding the model and a scenery.json saying what it is called and
// what you bump into. The weapons folder works the same way and for the same
// reason - which pieces exist is decided by which folders are present, so a new
// table is a folder and not a line of Go.
//
// Only the JSON is read at startup. The model itself is loaded the first time
// something spawns one, which is what MeshDrawer's name cache already does, so
// a folder of scenery nobody has placed costs nothing.
type Scenery struct {
	// ID is the folder's name, which is what a map entity's MeshName says.
	ID string `json:"-"`
	// Name is what the editor's placement menu calls it.
	Name string `json:"name"`
	// Colliders are the boxes physics uses, relative to where the piece stands.
	// A piece that names none is one box around its whole model, which is what
	// a table wants; a piece that names them replaces that, which is the only
	// way stairs can be walked up rather than into - each step has to be its
	// own box - and the only way a shelf can be reached into.
	Colliders [][2]mgl32.Vec3 `json:"colliders"`

	// Climbable are boxes nobody collides with but that mark this piece as a
	// ladder: standing in one lets the player or a pathing actor go straight up
	// or down instead of falling or being walled off. See registerClimbable and
	// core.MapEntity.ClimbTo.
	//
	// ponytail: a climbable box that stopped exactly at the ladder's own model
	// left nowhere to land - gravity always came back before the player was
	// horizontally clear of the wall the ladder is mounted on. scenery/ladder's
	// box runs taller than the model and a tile past it, into the landing, so
	// there is still a window with no gravity to walk out over the edge in.
	// That bakes in "this ladder climbs towards +Z"; a ladder facing another
	// way needs its own box, there is no rotation to derive it from yet.
	Climbable [][2]mgl32.Vec3 `json:"climbable"`
}

// sceneryDir is the folder of folders, one to a piece.
const sceneryDir = "scenery"

// sceneryFile is what every one of them holds beside its mesh.glb or mesh.gltf.
const sceneryFile = "scenery.json"

// loadScenery reads every folder under dir. A folder without a scenery.json is
// a mistake worth hearing about rather than a piece to leave out silently.
func loadScenery(dir string) (map[string]*Scenery, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scenery: %w", err)
	}
	library := make(map[string]*Scenery, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name(), sceneryFile))
		if err != nil {
			return nil, fmt.Errorf("scenery: %w", err)
		}
		piece := &Scenery{ID: entry.Name()}
		if err := json.Unmarshal(data, piece); err != nil {
			return nil, fmt.Errorf("scenery %s: %w", entry.Name(), err)
		}
		if piece.Name == "" {
			piece.Name = piece.ID
		}
		library[piece.ID] = piece
	}
	return library, nil
}

// colliders is this piece's collision shape: the boxes it names, all of them
// belonging to the one object the model is on. They are bound to it rather than
// set on it, because a GameObject holds one component of a type and a shelf is
// five boxes - so the object keeps its own, the one around the whole model, and
// these are extra shapes that answer with its name, its transform and its life.
//
// That is what makes a shelf one thing: the ray that hits a board reports the
// shelf, not a nameless box, and killing the shelf takes its boxes with it.
func (s *Scenery) colliders(piece *quads.GameObject) []*quads.PhysicsComponent {
	boxes := make([]*quads.PhysicsComponent, 0, len(s.Colliders))
	for _, box := range s.Colliders {
		physics := quads.NewPhysicsComponent(false, voxel.NewAABB(box[0], box[1]))
		physics.Init(piece)
		boxes = append(boxes, physics)
	}
	return boxes
}

// registerClimbable turns a ladder piece's local Climbable boxes into world
// volumes the player's movement checks every frame, and - given where the
// entity says its landing is - a ClimbLink pathfinding can route actors
// through. A piece with no ClimbTo is climbable by the player but invisible
// to pathfinding: nothing routes an actor onto a ladder that does not say
// where it comes out.
func (g *Game) registerClimbable(piece *quads.GameObject, def *Scenery, climbTo *mgl32.Vec3) {
	pos := piece.Transform().GetPosition()
	for _, box := range def.Climbable {
		world := voxel.NewAABB(box[0], box[1]).Add(pos)
		g.climbVolumes = append(g.climbVolumes, world)
	}
	if climbTo != nil && g.Pathfinder != nil {
		bottom := geometry.Point{X: int(pos.X()), Y: int(pos.Z())}
		top := geometry.Point{X: int(climbTo.X()), Y: int(climbTo.Z())}
		g.Pathfinder.AddClimbLink(bottom, top, pos.Y(), climbTo.Y())
	}
}

// meshFile is where a named model is. Scenery keeps its own beside its
// scenery.json, as mesh.gltf (Blockbench's own export, JSON with embedded
// buffers/images) if the piece has one, else the older generated mesh.glb -
// gltf.Open sniffs the content either way, so the extension only has to be
// honest, not load-bearing. Everything else - the torch, the cube, the debug
// meshes - is still a bare file under assets/meshes.
func (g *Game) meshFile(name string) string {
	if _, isScenery := g.scenery[name]; isScenery {
		dir := filepath.Join(sceneryDir, name)
		if _, err := os.Stat(filepath.Join(dir, "mesh.gltf")); err == nil {
			return filepath.Join(dir, "mesh.gltf")
		}
		return filepath.Join(dir, "mesh.glb")
	}
	return filepath.Join("assets", "meshes", name+".glb")
}

// sceneryIDs is the folder's contents in a fixed order, for a menu.
func (g *Game) sceneryIDs() []string {
	ids := make([]string, 0, len(g.scenery))
	for id := range g.scenery {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// spawnScenery puts one piece in the world at pos. It is spawnEntity with the
// entity filled in, because that is all a piece of scenery is: a mesh with a
// name. solid is false for the ghost that follows the crosshair in the editor -
// something you can walk into is not a ghost.
func (g *Game) spawnScenery(id string, pos mgl32.Vec3, solid bool) *quads.GameObject {
	piece, known := g.scenery[id]
	if !known {
		return nil
	}
	return g.spawnEntity(pos, core.MapEntity{Name: piece.Name, MeshName: id, CreateCollider: solid})
}
