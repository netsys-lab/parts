package control

type PartTransfer struct {
	PartId   uint64
	PartSize uint64
	Port     uint32
}

type PartsHandshake struct {
	Version                   uint8
	Flags                     uint16
	Reserved                  uint32
	MTU                       uint32
	InterfaceSpeed            uint64
	NumPorts                  uint32
	BufferSize                uint64
	EstimatedBandwidthPerPort uint64
	NumTransfers              uint32
	PartTransfers             []PartTransfer
	Raw                       []byte
}

func NewPartsHandshake() *PartsHandshake {
	hs := PartsHandshake{}
	return &hs
}

func (bs *PartsHandshake) Unpack(buf []byte) {
	bs.Raw = buf
}

func (bs *PartsHandshake) Pack(buf []byte) {
	bs.Raw = buf
}
