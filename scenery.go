package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelfps/core"
	"github.com/memmaker/voxelfps/geometry"
	"github.com/memmaker/voxelfps/quads"
	"github.com/memmaker/voxelfps/quads/voxel"
)

// Scenery is a piece of furniture the world is furnished with: a folder under
// assets/meshes/ holding the model and a scenery.json saying what it is called and
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
	// A box is what the player has to be *in* to climb, so it is the ladder's
	// own model plus the room the player's collider needs to be touching it:
	// the half-tile in front of the rungs, and no further. A box the size of
	// the whole voxel made the voxel the ladder - jumping anywhere in it left
	// you hanging in mid-air, and walking across it over a shaft meant you
	// could not fall - which is not what holding onto a ladder is.
	//
	// ponytail: it still runs taller than the model, so there is a window with
	// no gravity above the last rung to walk out of onto the landing in. That
	// window is the one place the player hovers without touching anything;
	// tightening it wants a real dismount, not a smaller box.
	//
	// It is authored as "this ladder climbs towards +Z"; registerClimbable is
	// what turns a ladder's own facing= into the box actually landing on the
	// wall it was placed against.
	Climbable [][2]mgl32.Vec3 `json:"climbable"`

	// Screen, if set, makes this piece's given material slot play a looping
	// flipbook instead of showing its baked-in texture - a computer monitor
	// turning on. Frames are read from a "screen" folder beside mesh.gltf:
	// "0.png", "1.png", ... See quads.MeshDrawer.EnsureScreenAnimation.
	Screen *ScreenSpec `json:"screen"`
}

// ScreenSpec names which material slot animates and how fast. Material is
// whatever index the model's own glTF export gave that face - check the
// model in Blockbench (or the exported .gltf's materials list) to know it.
type ScreenSpec struct {
	Material int     `json:"material"`
	FPS      float64 `json:"fps"`
}

// stairsMeshName is the staircase, which the engine has to know by name: it is
// what a map entity points its MeshName at to get one, and what
// core.MapEntity.StepTo exists for - see spawn_entities.go.
const stairsMeshName = "stairs"

// meshDir is the folder of folders, one to a model: a mesh.gltf or mesh.glb and
// whatever else that model needs beside it. A folder that also holds a
// scenery.json is a piece of furniture; one that does not - the cube, the torch -
// is just a mesh something spawns by name.
const meshDir = "assets/meshes"

// sceneryFile is what a furniture folder holds beside its mesh.glb or mesh.gltf.
const sceneryFile = "scenery.json"

// loadScenery reads every folder under dir that has a scenery.json in it. A
// folder without one is a plain model, not a mistake - they share a directory
// since the meshes and the scenery were consolidated.
//
// It walks the whole tree rather than the top level, because the imported models
// are filed under a category folder each - see modelCategory in tools/importss.
// A piece's ID is then its path below dir with forward slashes, which is what a
// map's MeshName holds and what meshFile joins back onto.
func loadScenery(dir string) (map[string]*Scenery, error) {
	library := make(map[string]*Scenery)
	err := filepath.WalkDir(dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != sceneryFile {
			return nil
		}
		id, err := filepath.Rel(dir, filepath.Dir(p))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		piece := &Scenery{ID: filepath.ToSlash(id)}
		if err := json.Unmarshal(data, piece); err != nil {
			return fmt.Errorf("scenery %s: %w", piece.ID, err)
		}
		if piece.Name == "" {
			piece.Name = piece.ID
		}
		library[piece.ID] = piece
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scenery: %w", err)
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
//
// They stay in the piece's own space, exactly as its scenery.json authored
// them: the solver turns them with the piece's matrix on its way past (see
// PhysicsComponent.WorldOBB), and so does the ray that picks them out. Turning
// them here as well is the rotation applied twice - which on the ark's ladders,
// a half turn each, put the solid slab back on the side of the tile the player
// is supposed to stand in.
func (s *Scenery) colliders(piece *quads.GameObject) []*quads.PhysicsComponent {
	boxes := make([]*quads.PhysicsComponent, 0, len(s.Colliders))
	for _, box := range s.Colliders {
		physics := quads.NewPhysicsComponent(false, voxel.NewAABB(box[0], box[1]).Scaled(piece.Transform().GetScale()))
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
	rotation := piece.Transform().GetRotation()
	// These are the one box that really is turned here: a climb volume is kept
	// as world extents and checked against the player's position directly, with
	// no piece and no matrix anywhere in the path - see Game.onLadder.
	for _, box := range def.Climbable {
		world := voxel.NewAABB(box[0], box[1]).Scaled(piece.Transform().GetScale()).Rotated(rotation).Add(pos)
		g.climbVolumes = append(g.climbVolumes, climbVolume{box: world, piece: piece})
	}
	if climbTo != nil && g.Pathfinder != nil {
		bottom := geometry.Point{X: int(pos.X()), Y: int(pos.Z())}
		top := geometry.Point{X: int(climbTo.X()), Y: int(climbTo.Z())}
		g.Pathfinder.AddClimbLink(bottom, top, pos.Y(), climbTo.Y())
	}
}

// meshFile is where a named model is. Scenery keeps its own beside its
// scenery.json, as mesh.gltf (Blockbench's own export, JSON with embedded
// buffers/images) if the model has one, else the older generated mesh.glb -
// gltf.Open sniffs the content either way, so the extension only has to be
// honest, not load-bearing. Scenery and the plain meshes answer to the same
// shape now, so there is nothing to branch on.
func (g *Game) meshFile(name string) string {
	return meshFileIn(filepath.Join(meshDir, name))
}

// meshFileIn is the model inside one folder, given the folder. Split out of
// meshFile because the editor's browser walks the folder itself and so has the
// directory in hand rather than a piece's name.
func meshFileIn(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "mesh.gltf")); err == nil {
		return filepath.Join(dir, "mesh.gltf")
	}
	return filepath.Join(dir, "mesh.glb")
}

// hasScenery is whether a folder is a piece of furniture rather than a category
// to walk further into - which is exactly the question the browser asks of every
// directory it lists, and the same question loadScenery answers by walking.
func hasScenery(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, sceneryFile))
	return err == nil
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

// climbVolume is a ladder's climbable box in the world and the piece it belongs
// to, so a ladder the editor has moved or deleted stops being climbable where it
// was.
type climbVolume struct {
	box   voxel.AABB
	piece *quads.GameObject
}
