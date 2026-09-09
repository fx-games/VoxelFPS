// firecheck loads a weapons folder and, for every weapon, holds the trigger
// down against its compiled state machine and watches what comes out: a
// hitscan, a projectile, a melee swing, or nothing at all in a window long
// enough that a working weapon has fired several times over.
//
// It runs the same wep.Cursor/wep.Runtime the game does, headless - no
// window, no GPU, no sprites - so it is cheap enough to run after every
// migration and catch a weapon that compiles but never actually shoots.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"

	"github.com/memmaker/goray/wep"
)

// raiseTics is generous headroom for a weapon's own raise animation; the
// slowest of them takes a few dozen. fireTics is long enough to see several
// shots out of the slowest-firing weapon, or a refire loop settle.
const (
	raiseTics = 200
	fireTics  = 700
	// pressTics and releaseTics shape the tap: long enough down that a
	// held-to-fire weapon gets several shots off, long enough up that a
	// release-to-fire weapon's Ready sequence has settled before the next
	// press, short enough that dozens of taps fit inside fireTics.
	pressTics   = 20
	releaseTics = 10
	tic         = 1.0 / wep.TicRate
)

func main() {
	dir := flag.String("weapons", "assets/weapons", "the weapons folder to check")
	verbose := flag.Bool("v", false, "list every weapon, not just the ones with a problem")
	flag.Parse()

	library, err := wep.Load(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "loading:", err)
		os.Exit(1)
	}

	ids := make([]string, 0, len(library.Weapons))
	for _, w := range library.Weapons {
		ids = append(ids, w.ID)
	}
	sort.Strings(ids)

	problems := 0
	for _, id := range ids {
		weapon := library.ByID[id]
		kind, err := fireCheck(weapon)
		if err != nil {
			problems++
			fmt.Printf("%-30s FAILS: %v\n", id, err)
			continue
		}
		if *verbose {
			fmt.Printf("%-30s ok (%s)\n", id, kind)
		}
	}
	if problems > 0 {
		fmt.Printf("\n%d of %d weapon(s) never fired\n", problems, len(ids))
		os.Exit(1)
	}
	fmt.Printf("all %d weapon(s) fire\n", len(ids))
}

// fireCheck raises a weapon, holds the trigger, and reports the first kind of
// attack it saw, or why it never saw one.
func fireCheck(weapon *wep.Weapon) (kind string, err error) {
	random := rand.New(rand.NewSource(1))
	runtime := wep.NewRuntime(weapon, random)
	cursor := wep.NewCursor(&weapon.States, "Select")
	cursor.StartRaise()

	for i := 0; i < raiseTics; i++ {
		cursor.Update(runtime, tic)
		if _, onReady := cursor.Ready(); onReady {
			break
		}
		if cursor.Stopped() {
			return "", fmt.Errorf("its Select state stops before the weapon is ready (%s)", cursor.Describe())
		}
	}
	if _, onReady := cursor.Ready(); !onReady {
		return "", fmt.Errorf("never reaches a ready frame within %d tics of raising (stuck at %s)", raiseTics, cursor.Describe())
	}

	// A held trigger is the common case, but some weapons (Brutal Doom's axe
	// among them) read A_Refire as "still down means keep winding up" and
	// only swing once the button comes back up - a real player taps and
	// releases, so a press that never lets go would never see that swing
	// land, not because the weapon is broken but because nothing here ever
	// asked it to finish. Tapping - down for a stretch, then up for one -
	// finds a real attack either way.
	pressed := true
	sinceChange := 0
	for i := 0; i < fireTics; i++ {
		runtime.Input.Trigger = pressed
		for _, op := range cursor.Update(runtime, tic) {
			if kind := attackKind(op.Op); kind != "" {
				return kind, nil
			}
		}
		if cursor.Stopped() {
			return "", fmt.Errorf("its own states stop while the trigger is held (%s)", cursor.Describe())
		}
		sinceChange++
		if (pressed && sinceChange >= pressTics) || (!pressed && sinceChange >= releaseTics) {
			pressed = !pressed
			sinceChange = 0
		}
	}
	return "", fmt.Errorf("tapped the trigger for %d tics and never fired, spawned a projectile, or swung (last at %s)",
		fireTics, cursor.Describe())
}

// attackKind names the three shapes an attack takes, matching what a
// hand-written DECORATE weapon actually reaches for: a hitscan, a spawned
// projectile, or a melee reach/area check.
func attackKind(op string) string {
	switch op {
	case wep.OpFireBullets, wep.OpRailAttack:
		return "hitscan"
	case wep.OpFireProjectile:
		return "projectile"
	case wep.OpCustomPunch, wep.OpSaw:
		return "melee"
	case wep.OpExplode:
		return "explode"
	}
	return ""
}
