package glhf

type IntVec2 [2]int32

func (v IntVec2) X() int32 {
    return v[0]
}

func (v IntVec2) Y() int32 {
    return v[1]
}

type IntVec4 [4]int32

func (v IntVec4) X() int32 {
    return v[0]
}

func (v IntVec4) Y() int32 {
    return v[1]
}

func (v IntVec4) Z() int32 {
    return v[2]
}

func (v IntVec4) W() int32 {
    return v[3]
}
