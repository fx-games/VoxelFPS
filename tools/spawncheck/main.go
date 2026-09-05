package main

import (
	"fmt"
	"math/rand"
	"os"

	"github.com/memmaker/goray/wep"
)

func main() {
	lib, err := wep.Load("weapons")
	if err != nil {
		fmt.Println(err)
		return
	}
	id := os.Args[1]
	w := lib.ByID[id]
	if w == nil {
		fmt.Println("no", id)
		return
	}
	rt := wep.NewRuntime(w, rand.New(rand.NewSource(1)))
	rt.Input.Trigger = true
	c := wep.NewCursor(&w.States, "Select")
	c.StartRaise()
	spawns := map[string]int{}
	for i := 0; i < 200; i++ {
		for _, op := range c.Update(rt, 1.0/wep.TicRate) {
			if op.Op == wep.OpSpawnActor || op.Op == wep.OpFireProjectile || op.Op == wep.OpSpawnItem {
				spawns[op.Actor]++
			}
		}
	}
	fmt.Printf("%s spawned %d kinds in 200 tics\n", id, len(spawns))
	actors := lib.Actors[id]
	for name, n := range spawns {
		a := actors[lowerOf(name)]
		if a == nil {
			fmt.Printf("  %-40s x%-3d NOT IN actors map\n", name, n)
			continue
		}
		_, hasSpawn := a.States.Label("Spawn")
		visible := 0
		for _, s := range a.States.Steps {
			if s.Sprite != "" {
				visible++
			}
		}
		fmt.Printf("  %-40s x%-3d stock=%v spawnLabel=%v steps=%d visibleFrames=%d\n",
			name, n, a.Stock, hasSpawn, len(a.States.Steps), visible)
	}
}

func lowerOf(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
