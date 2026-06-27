package datura

type MortonCoder struct {
}

func NewMortonCoder() *MortonCoder {
	return &MortonCoder{}
}

func (c *MortonCoder) Encode(x, y uint64) uint64 {
	return splitMortonBits(x) | splitMortonBits(y)<<1
}

func (c *MortonCoder) Decode(morton uint64) (uint64, uint64) {
	return compactMortonBits(morton), compactMortonBits(morton >> 1)
}

func splitMortonBits(value uint64) uint64 {
	value &= 0x00000000ffffffff
	value = (value | value<<16) & 0x0000ffff0000ffff
	value = (value | value<<8) & 0x00ff00ff00ff00ff
	value = (value | value<<4) & 0x0f0f0f0f0f0f0f0f
	value = (value | value<<2) & 0x3333333333333333
	value = (value | value<<1) & 0x5555555555555555

	return value
}

func compactMortonBits(value uint64) uint64 {
	value &= 0x5555555555555555
	value = (value | value>>1) & 0x3333333333333333
	value = (value | value>>2) & 0x0f0f0f0f0f0f0f0f
	value = (value | value>>4) & 0x00ff00ff00ff00ff
	value = (value | value>>8) & 0x0000ffff0000ffff
	value = (value | value>>16) & 0x00000000ffffffff

	return value
}
