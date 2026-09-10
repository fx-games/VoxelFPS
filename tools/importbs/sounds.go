package main

// soundNames maps a slot in VSWAP's digitized-sound table to the name the game
// calls that sound by. The file itself only has a table of offsets; the pairing
// is bstone's digitized_map, which exists because the digitized set is a subset
// of the hundred AdLib effects and is not in the same order.
var soundNames = map[int]string{
	0:  "ATKIONCANNON",
	1:  "ATKCHARGED",
	2:  "ATKBURSTRIFLE",
	3:  "OPENDOOR",
	4:  "CLOSEDOOR",
	5:  "HTECHDOOROPEN",
	6:  "HTECHDOORCLOSE",
	7:  "INFORMANTDEATH",
	8:  "GOLDSTERNHALT",
	9:  "HALT",
	10: "RENTDEATH1",
	11: "EXPLODE1",
	12: "GGUARDHALT",
	13: "PROGUARDDEATH",
	14: "DOGBOYHALT",
	15: "SCANHALT",
	16: "PROHALT",
	17: "GGUARDDEATH",
	18: "BLUEBOYDEATH",
	19: "SCIENTISTHALT",
	20: "SCIENTISTDEATH",
	21: "DOGBOYDEATH",
	22: "SWATHALT",
	23: "SCANDEATH",
	24: "GOLDSTERNLAUGH",
	25: "ELECARCDAMAGE",
	26: "PODHATCH",
	27: "ELECTSHOT",
	28: "ELECAPPEAR",
	29: "ELECDIE",
	30: "LIQUIDDIE",
	31: "GURNEY",
	32: "WARPIN",
	33: "WARPOUT",
	34: "RENTDEATH2",
	35: "EXPLODE2",
	36: "LCANHALT",
	37: "LCANDEATH",
	39: "INFORMDEATH2",
	40: "INFORMDEATH3",
	41: "GURNEYDEATH",
	42: "PRODEATH2",
	43: "PRODEATH3",
	44: "LCANBREAK",
	45: "SCANBREAK",
	46: "ATKGRENADE",
	47: "SWATDIE",
	48: "SWATDEATH2",
	49: "PODDEATH",
	50: "PODHALT",
	51: "BLUEBOYHALT",
	52: "SWATDEATH3",
	53: "SCIDEATH2",
	54: "SCIDEATH3",
	55: "SPITATTACK",
	56: "CLAWATTACK",
	57: "PUNCHATTACK",
}

// soundIsDigitised reports whether a name is one of the effects VSWAP actually
// carries. Blake Stone's AdLib set is a hundred effects and only these are also
// digitised, so a name from the game's source is not by itself a name that can
// be extracted.
func soundIsDigitised(name string) bool {
	for _, digitised := range soundNames {
		if digitised == name {
			return true
		}
	}
	return false
}
