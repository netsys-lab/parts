package control

type BlockTransfer struct {
	BlockId   uint64
	BlockSize uint64
	Port      uint32
}

type BlocksHandshake struct {
	Version                   uint8
	Flags                     uint16
	Reserved                  uint32
	MTU                       uint32
	InterfaceSpeed            uint64
	NumPorts                  uint32
	BufferSize                uint64
	EstimatedBandwidthPerPort uint64
	NumTransfers              uint32
	BlockTransfers            []BlockTransfer
	Raw                       []byte
}

func NewBlocksHandshake() *BlocksHandshake {
	hs := BlocksHandshake{}
	return &hs
}

func (bs *BlocksHandshake) Unpack(buf []byte) {
	bs.Raw = buf
}

func (bs *BlocksHandshake) Pack(buf []byte) {
	bs.Raw = buf
}
