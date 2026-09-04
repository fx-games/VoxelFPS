package tpack

import "image"

type Byter interface {
	Bytes() []byte
}

type NamedImage struct {
	image.Image
	Name string
}

func (i NamedImage) Pixels() []uint8 {
	return AsRGBA(i).Pix
}

type LPString8 string

func (s LPString8) String() string {
	return string(s)
}
func (s LPString8) Bytes() []byte {
	length := uint8(len(s))
	return append([]byte{length}, []byte(s)...)
}
