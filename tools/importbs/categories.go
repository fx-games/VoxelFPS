package main

// Where a sprite is filed. Every kind lands in assets/sprites under a role and a
// category: the role is which of the editor's tools places it, and the category
// is what it is. See tools/importss/categories.go, which does the same thing for
// the other game and shares this vocabulary on purpose - a level is furnished out
// of "furniture", whichever of the two games drew the chair.
//
// System Shock sorts itself: every one of its objects carries a class in the
// game's property table. Blake Stone has no such table, and what it has instead
// is bstone's own statinfo comments - the name of each picture and what the game
// does with it. That is where the item categories below come from: the game
// itself says which pictures are bonuses and which of its bonuses are keys. The
// scenery has only its name to go on, so those are sorted by hand, once, here.

// The three roles, which are the three folders at the top of assets/sprites and
// the three the editor's 2D PROPS, PICKUPS and ACTORS tools browse.
const (
	roleProps  = "props"
	roleItems  = "items"
	roleActors = "actors"
)

// bonusCategories is every bo_ in staticNames: what the game calls a thing the
// player walks over and takes. The names are Blake Stone's own.
var bonusCategories = map[string]string{
	"bo_pistol":           roleItems + "/weapons",
	"bo_burst_rifle":      roleItems + "/weapons",
	"bo_ion_cannon":       roleItems + "/weapons",
	"bo_grenade":          roleItems + "/weapons",
	"bo_plasma_detonator": roleItems + "/weapons",

	"bo_clip":  roleItems + "/ammo",
	"bo_clip2": roleItems + "/ammo",

	"bo_firstaid":     roleItems + "/supplies",
	"bo_water":        roleItems + "/supplies",
	"bo_water_puddle": roleItems + "/supplies",
	"bo_watervent":    roleItems + "/supplies",
	"bo_sandwich":     roleItems + "/supplies",
	"bo_ham":          roleItems + "/supplies",
	"bo_chicken":      roleItems + "/supplies",
	"bo_candybar":     roleItems + "/supplies",
	"bo_automapper1":  roleItems + "/supplies",

	"bo_red_key":    roleItems + "/cards",
	"bo_yellow_key": roleItems + "/cards",
	"bo_blue_key":   roleItems + "/cards",

	"bo_gold":      roleItems + "/treasure",
	"bo_gold1":     roleItems + "/treasure",
	"bo_gold2":     roleItems + "/treasure",
	"bo_gold3":     roleItems + "/treasure",
	"bo_coin":      roleItems + "/treasure",
	"bo_coin5":     roleItems + "/treasure",
	"bo_money_bag": roleItems + "/treasure",
	"bo_loot":      roleItems + "/treasure",
	"bo_bonus":     roleItems + "/treasure",
}

// staticCategories is the scenery, by the sprite's own name in staticNames.
// bstone's table says only whether each one is solid or walked through, which is
// a fact about collision and not about what the thing is, so what it is comes
// from the name it was given: a "Floor Lamp" is a light and a "Ficus Tree" is a
// plant, in the same categories the other game's decorations use.
var staticCategories = map[string]string{
	"STAT_2":       "furniture", // Lunch Table
	"STAT_4":       "furniture", // Lab Table
	"STAT_13":      "furniture", // General Table
	"STAT_15":      "furniture", // General Chair
	"STAT_37":      "furniture", // Office Desk
	"STAT_38":      "furniture", // Office Chair
	"STAT_39":      "furniture", // Security Desk
	"STAT_59":      "furniture", // Office Sink
	"STAT_61":      "furniture", // Table
	"STAT_62":      "furniture", // Chair
	"STAT_63":      "furniture", // Stool
	"HITECH_CHAIR": "furniture", // Hi-Tech table and chair

	"STAT_3":  "lights", // Floor Lamp
	"STAT_14": "lights", // Ceiling Light
	"STAT_20": "lights", // Indirect Half-Sphere
	"STAT_55": "lights", // Ceiling Lamp #2
	"STAT_56": "lights", // Ceiling Lamp #3
	"STAT_58": "lights", // OR Lamp

	"STAT_8":     "nature", // Ficus Tree
	"STAT_11":    "nature", // Plant 1
	"STAT_18":    "nature", // Plant 2
	"BONZI_TREE": "nature",
	"POT_PLANT":  "nature",
	"TUBE_PLANT": "nature",

	"STAT_5":     "structure", // Pillar
	"STAT_52":    "structure", // Greek Post
	"STAT_53":    "structure", // Red/Blue post
	"STAT_54":    "structure", // Red HiTech Post
	"VSPIKE8":    "structure",
	"VPOST8":     "structure",
	"GRATE":      "structure", // Floor Grate
	"STEAM_PIPE": "structure",

	"STAT_1":  "containers", // Containment Canister
	"STAT_12": "containers", // Vase
	"STAT_16": "containers", // Kitchen Trash
	"STAT_17": "containers", // Office Trash
	"STAT_23": "containers", // Body Can
	"CRATE_1": "containers",
	"CRATE_2": "containers",
	"CRATE_3": "containers",

	"STAT_10": "medical", // Water Fountain, and the theatre it stands in
	"STAT_19": "medical", // Gurney No-Blood
	// The four canisters and gurneys a character climbs out of, full and empty.
	"GURNEY_MUT_READY": "medical",
	"GURNEY_MUT_EMPTY": "medical",
	"LCAN_ALIEN_READY": "medical",
	"LCAN_ALIEN_EMPTY": "medical",
	"SCAN_ALIEN_READY": "medical",
	"SCAN_ALIEN_EMPTY": "medical",

	"STAT_9":      "corpses", // Half-Eaten Corpse
	"STAT_64":     "corpses", // Gore
	"STAT_74":     "corpses",
	"STAT_76":     "corpses",
	"OFC_DEAD":    "corpses",
	"PRO_DEAD":    "corpses",
	"GSCOUT_DEAD": "corpses",
	"DEAD_RENT":   "corpses",
	"DEAD_PRO":    "corpses",
	"DEAD_SWAT":   "corpses",

	"STAT_6":  "stains", // Blood Puddle
	"STAT_7":  "stains", // Piss Puddle
	"STAT_41": "stains", // Empty Water Bowl
	"STAT_43": "stains", // Chicken Bone
	"STAT_45": "stains", // Ham Bone
	"RUBBLE":  "stains", // Door Rubble

	"STAT_21": "wall_art", // Exit Sign
	"STAT_22": "wall_art", // Transporter
	"STAT_25": "wall_art", // Statue
	"STAT_47": "wall_art", // Video Game Machine
	"STAT_79": "panels",   // Auto-Charge Pistol, a charger on the wall
}

// blakePropCategories is the animated scenery, by its key in blakeProps. Those
// are a table of their own because the game numbers their frames rather than
// naming them, so they never reach staticNames.
var blakePropCategories = map[string]string{
	"steam":         roleProps + "/effects",
	"pipe_steam":    roleProps + "/effects",
	"blood_drip":    roleProps + "/effects",
	"water_drip":    roleProps + "/effects",
	"deco_arc":      roleProps + "/effects",
	"green_ooze":    roleProps + "/effects",
	"black_ooze":    roleProps + "/effects",
	"explosion":     roleProps + "/effects",
	"grenade_blast": roleProps + "/effects",
	"security_lamp": roleProps + "/lights",
	"air_vent":      roleProps + "/structure",
	"grate":         roleProps + "/structure",
	"steam_pipe":    roleProps + "/structure",
	"rubble":        roleProps + "/stains",
	"candy_wrapper": roleProps + "/stains",
	"sandwich_wrap": roleProps + "/stains",
	"bonzi_tree":    roleProps + "/nature",
	"pot_plant":     roleProps + "/nature",
	"tube_plant":    roleProps + "/nature",
	"hitech_chair":  roleProps + "/furniture",
	"dead_rent":     roleProps + "/corpses",
	"dead_pro":      roleProps + "/corpses",
	"dead_swat":     roleProps + "/corpses",
	"doorbomb":      roleItems + "/weapons",
	"automapper":    roleItems + "/supplies",
	"candy_bar":     roleItems + "/supplies",
	"sandwich":      roleItems + "/supplies",
}

// staticCategory is the folder for one numbered static: an item when the game
// treats it as a bonus, and otherwise scenery under whichever category its name
// puts it in. A picture nothing above names is clutter, which is what the
// half-dozen entries bstone left with a broken comment actually are.
func staticCategory(sprite, kind string) string {
	if category, isBonus := bonusCategories[kind]; isBonus {
		return category
	}
	if category, sorted := staticCategories[sprite]; sorted {
		return roleProps + "/" + category
	}
	return roleProps + "/clutter"
}

// blakePropCategory is the folder for one of the animated scenery pieces.
func blakePropCategory(id string) string {
	if category, sorted := blakePropCategories[id]; sorted {
		return category
	}
	return roleProps + "/clutter"
}
