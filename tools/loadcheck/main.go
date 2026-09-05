package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/memmaker/goray/wep"
)

func main() {
	lib, err := wep.Load(os.Args[1])
	if err != nil {
		msg := err.Error()
		lines := strings.Split(msg, "\n")
		fmt.Printf("FAILED with %d problem line(s):\n", len(lines))
		for i, l := range lines {
			if i > 14 {
				fmt.Println("  ...")
				break
			}
			fmt.Println(l)
		}
		os.Exit(1)
	}
	steps, actors := 0, 0
	for _, w := range lib.Weapons {
		steps += len(w.States.Steps)
		actors += len(lib.Actors[w.ID])
	}
	fmt.Printf("loaded %d weapons, %d steps, %d actors, %d keys bound\n",
		len(lib.Weapons), steps, actors, len(lib.Keys))
}
