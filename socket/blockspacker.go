package socket

import "encoding/binary"

type BlocksPacketPacker interface {
	GetHeaderLen() int
	Pack(buf *[]byte, blockContect *BlockContext) error
	PackRetransfer(buf *[]byte, sequenceNum int64, blockContect *BlockContext) error
	Unpack(buf *[]byte, blockContect *BlockContext) (*BlockPacket, error)
}

type BinaryBlocksPacketPacker struct {
}

func NewBinaryBlocksPacketPacker() BlocksPacketPacker {
	return &BinaryBlocksPacketPacker{}
}

func (bp *BinaryBlocksPacketPacker) GetHeaderLen() int {
	return 16
}

func (bp *BinaryBlocksPacketPacker) PackRetransfer(buf *[]byte, sequenceNumber int64, blockContext *BlockContext) error {
	binary.BigEndian.PutUint64((*buf)[8:16], uint64(sequenceNumber))
	binary.BigEndian.PutUint64((*buf)[0:8], uint64(blockContext.BlockId))
	return nil
}

func (bp *BinaryBlocksPacketPacker) Pack(buf *[]byte, blockContext *BlockContext) error {
	sequenceNumber := blockContext.GetNextSequenceNumber()
	binary.BigEndian.PutUint64((*buf)[8:16], uint64(sequenceNumber))
	binary.BigEndian.PutUint64((*buf)[0:8], uint64(blockContext.BlockId))
	return nil
}
func (bp *BinaryBlocksPacketPacker) Unpack(buf *[]byte, blockContext *BlockContext) (*BlockPacket, error) {
	p := BlockPacket{}
	p.SequenceNumber = (int64(binary.BigEndian.Uint64((*buf)[8:16])))
	p.BlockId = (int64(binary.BigEndian.Uint64((*buf)[0:8])))
	*buf = (*buf)[blockContext.BlocksPacketPacker.GetHeaderLen():]
	return &p, nil
}
