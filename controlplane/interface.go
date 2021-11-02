package controlplane

var ControlPacketTypes = newControlPacketTypes()

func newControlPacketTypes() *controlPacketTypes {
	return &controlPacketTypes{
		AckPacket:       1,
		HandshakePacket: 2,
		UndefinedPacket: 99,
	}
}

type controlPacketTypes struct {
	AckPacket       int
	HandshakePacket int
	UndefinedPacket int
}

type PartAckPacket struct {
	PartId                       int64
	LastSequenceNumber           int64
	NumPacketsPerTx              int
	PacketTxIndex                int
	MissingSequenceNumbers       []int64
	MissingSequenceNumberOffsets []int64
	TransactionId                int64
	RequestSequenceId            int
	NumPackets                   int64
	PartFinished                 bool
}

type PartTransfer struct {
	PartId   uint64
	PartSize uint64
	Port     uint32
}

type PartsHandshakePacket struct {
	Version                   uint8
	Flags                     uint16
	Reserved                  uint32
	MTU                       uint32
	InterfaceSpeed            uint64
	NumPorts                  uint32
	BufferSize                uint64
	EstimatedBandwidthPerPort uint64
	NumTransfers              uint32
	SenderAddr                string
	PartTransfers             []PartTransfer
}
