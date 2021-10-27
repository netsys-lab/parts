package dataplane

import (
	"encoding/binary"
	"errors"
)

type PartsPacketPacker interface {
	GetHeaderLen() int
	Pack(buf *[]byte, partContect *PartContext) error
	PackRetransfer(buf *[]byte, sequenceNum int64, partContect *PartContext) error
	Unpack(buf *[]byte, partContect *PartContext) (*PartPacket, error)
}

type BinaryPartsPacketPacker struct {
}

func NewBinaryPartsPacketPacker() PartsPacketPacker {
	return &BinaryPartsPacketPacker{}
}

func (bp *BinaryPartsPacketPacker) GetHeaderLen() int {
	return 16
}

func (bp *BinaryPartsPacketPacker) PackRetransfer(buf *[]byte, sequenceNumber int64, partContext *PartContext) error {
	binary.BigEndian.PutUint64((*buf)[8:16], uint64(sequenceNumber))
	binary.BigEndian.PutUint64((*buf)[0:8], uint64(partContext.PartId))
	return nil
}

func (bp *BinaryPartsPacketPacker) Pack(buf *[]byte, partContext *PartContext) error {
	sequenceNumber := partContext.GetNextSequenceNumber()
	binary.BigEndian.PutUint64((*buf)[8:16], uint64(sequenceNumber))
	binary.BigEndian.PutUint64((*buf)[0:8], uint64(partContext.PartId))
	return nil
}
func (bp *BinaryPartsPacketPacker) Unpack(buf *[]byte, partContext *PartContext) (*PartPacket, error) {
	p := PartPacket{}
	p.SequenceNumber = (int64(binary.BigEndian.Uint64((*buf)[8:16])))
	p.PartId = (int64(binary.BigEndian.Uint64((*buf)[0:8])))
	if p.PartId != partContext.PartId {
		return nil, errors.New("mismatching partId")
	}

	*buf = (*buf)[partContext.PartsPacketPacker.GetHeaderLen():]
	return &p, nil
}
