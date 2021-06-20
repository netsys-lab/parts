package packet

type PacketPacker interface {
	GetHeaderLen() int
	Pack([]byte) error
	Unpack([]byte) error
}
