package glhf

type IntVec2 [2]int32

func (v IntVec2) X() int32 {
    return v[0]
}

func (v IntVec2) Y() int32 {
    return v[1]
}
