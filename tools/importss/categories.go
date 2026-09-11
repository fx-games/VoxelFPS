package main

// Where a sprite is filed. Every kind lands in assets/sprites under a role and a
// category: the role is which of the editor's tools places it, and the category
// is what it is. Six hundred folders in one flat directory is a directory nobody
// can find a chair in, and the editor now browses this tree with a picture in
// every cell - see quads.GlApplication.OpenMeshBrowser and the editor's pickers.
//
// The sorting is the game's own. Every object in System Shock carries a
// class/subclass/type triple in the property table, and that triple already says
// whether a thing is a gun, a chair or a mutant; reading it is both less work
// and more accurate than eighty judgement calls about what a MED CART is.

import "github.com/inkyblackness/hacked/ss1/content/object"

// The three roles, which are the three folders at the top of assets/sprites and
// the three the editor's 2D PROPS, PICKUPS and ACTORS tools browse.
const (
	roleProps  = "props"
	roleItems  = "items"
	roleActors = "actors"
)

// spriteCategory is the folder for one object, as "<role>/<category>".
//
// The classes, out of the game's own headers: 0 guns, 1 ammunition, 2 the things
// they fire, 3 grenades, 4 the drug patches, 5 hardware, 6 software, 7 the
// station's decorations, 8 items, 9 the wall panels, 10 doors and gratings, 11
// the animating effects, 12 the invisible triggers, 13 containers and the bodies
// left in them, 14 everything that walks.
func spriteCategory(triple object.Triple) string {
	class, subclass := int(triple.Class), int(triple.Subclass)
	switch class {
	case 0:
		return roleItems + "/weapons"
	case 1:
		return roleItems + "/ammo"
	case 3:
		return roleItems + "/explosives"
	case 4:
		return roleItems + "/patches"
	case 5:
		return roleItems + "/hardware"
	case 6:
		return roleItems + "/software"
	case 2, 11:
		// What a gun leaves behind and what an explosion looks like: pictures
		// that are played rather than placed, but placeable all the same.
		return roleProps + "/effects"
	case 7:
		return roleProps + "/" + pick(decorationCategories[:], subclass, "clutter")
	case 8:
		return pick(itemClassCategory[:], subclass, roleProps+"/clutter")
	case 9:
		return roleProps + "/panels"
	case 10:
		return roleProps + "/doors"
	case 12:
		return roleProps + "/markers"
	case 13:
		// The first three subclasses are things with a lid; the rest are what is
		// lying on the floor after a fight, which belongs with the other corpses.
		if subclass <= 2 {
			return roleProps + "/containers"
		}
		return roleProps + "/corpses"
	case 14:
		return roleActors + "/" + pick(critterCategories[:], subclass, "mutants")
	}
	return roleProps + "/clutter"
}

// decorationCategories is class seven by subclass: the station's furnishings.
var decorationCategories = [8]string{
	0: "electronics",
	1: "furniture",
	2: "wall_art",
	3: "lights",
	4: "medical",
	5: "lab",
	6: "nature",
	7: "structure",
}

// itemClassCategory is class eight by subclass. It is the one class that spans
// two roles: three of its subclasses are things the player picks up and the rest
// are things that stand in the room, so the role comes out of the subclass here
// rather than out of the class.
var itemClassCategory = [8]string{
	0: roleProps + "/clutter",  // a briefcase, a helmet, a fire extinguisher
	1: roleProps + "/debris",   // the broken halves of the above
	2: roleProps + "/corpses",  // bones and bodies
	3: roleItems + "/supplies", // first aid, batteries, a logic probe
	4: roleItems + "/cards",    // the access cards
	5: roleProps + "/cyberspace",
	6: roleProps + "/stains", // blood, burns, bullet holes
	7: roleItems + "/plot",   // the isotopes and the plastique
}

// critterCategories is class fourteen by subclass: everything that walks, in the
// order the game groups it - the mutants, the robots, the cyborgs, the software
// that lives in cyberspace, and the two that are neither and are the ending.
var critterCategories = [5]string{
	0: "mutants",
	1: "robots",
	2: "cyborgs",
	3: "cyberspace",
	4: "bosses",
}

// pick is one of a table's entries, or the fallback for a subclass the table does
// not reach. The tables cover every subclass the game actually ships; this is so
// a modified property table files something odd somewhere rather than panicking
// halfway through an import.
func pick(table []string, index int, fallback string) string {
	if index < 0 || index >= len(table) || table[index] == "" {
		return fallback
	}
	return table[index]
}
