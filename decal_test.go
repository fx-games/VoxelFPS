package main

import (
	"testing"

	"github.com/memmaker/voxelfps/wep"
)

// What a hitscan marks the wall with, by ZDoom's rules: the weapon's own decal
// first, then the puff's, and GZDoom's stock BulletPuff - which is what a call
// naming no puff gets - carries "BulletChip". The two ends of that are the
// Dual 92s, which name nothing and must still chip, and the Blacktail's laser
// pointer, whose own puff says `Decal "None"` and must not.
func TestDecalForFollowsTheWeaponThenThePuff(t *testing.T) {
	weapon := func(decal string, actors map[string]*wep.Effect) *Weapon {
		return &Weapon{Data: &wep.Weapon{Decal: decal}, Actors: actors}
	}
	redDot := map[string]*wep.Effect{"reddot": {Name: "RedDot"}}
	stock := map[string]*wep.Effect{"bulletpuff": {Name: "BulletPuff", Stock: true}}
	chip := map[string]*wep.Effect{"btpuff": {Name: "BTPuff", Decal: "BulletChip"}}
	noDecal := map[string]*wep.Effect{"btpuff": {Name: "BTPuff", Decal: "BulletChip", NoDecal: true}}

	for _, c := range []struct {
		name   string
		weapon *Weapon
		puff   string
		want   string
	}{
		{"names nothing at all", weapon("", nil), "", "BulletChip"},
		{"names GZDoom's own puff", weapon("", stock), "BulletPuff", "BulletChip"},
		{"the weapon's own decal wins", weapon("BulletChip", redDot), "RedDot", "BulletChip"},
		{"a mod puff that marks nothing", weapon("", redDot), "RedDot", ""},
		{"a mod puff that marks a chip", weapon("", chip), "BTPuff", "BulletChip"},
		{"+NODECAL stops everything", weapon("BulletChip", noDecal), "BTPuff", ""},
	} {
		if got := (&Game{}).decalFor(c.weapon, c.puff); got != c.want {
			t.Errorf("%s: decalFor(%q) = %q, want %q", c.name, c.puff, got, c.want)
		}
	}

	if !leavesABulletHole("BulletChip") {
		t.Error(`leavesABulletHole("BulletChip") is false, so nothing above draws anything`)
	}
}
