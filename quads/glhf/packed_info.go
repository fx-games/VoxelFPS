package glhf

type PackedInfo interface {
	Bytes() []byte
	UVSize(index int) (int, int)
	UVPosition(index int) (int, int)
	UV(index int) (minU int, minV int, maxU int, maxV int)
	SizeForLayout(index int) (int, int)
	PositionOffsetForLayout(index int) (int, int)
	HasIndex(index int) bool
	LastValidIndex() int
	Name(index int) string
}
