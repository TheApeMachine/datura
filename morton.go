package datura

type MortonCoder struct {
}

func NewMortonCoder() *MortonCoder {
	return &MortonCoder{}
}

func (c *MortonCoder) Encode(x, y uint64) uint64 {
	return x<<1 | y
}

func (c *MortonCoder) Decode(morton uint64) (uint64, uint64) {
	return morton >> 1, morton & 1
}
