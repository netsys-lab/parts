package socket

type PartRequestPacket struct {
	PartId                       int64
	LastSequenceNumber           int64
	MissingSequenceNumbers       []int64
	MissingSequenceNumberOffsets []int64
	TransactionId                int64
}

type PartPacket struct {
	SequenceNumber int64
	PartId         int64
	PartSize       int64
	Payload        []byte
}
