package socket

type BlockRequestPacket struct {
	BlockId                int64
	LastSequenceNumber     int64
	MissingSequenceNumbers []int64
}

type BlockPacket struct {
	SequenceNumber int64
	BlockId        int64
	BlockSize      int64
	Payload        []byte
}
