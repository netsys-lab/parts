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
	return 44
}

func (bp *BinaryPartsPacketPacker) PackRetransfer(buf *[]byte, sequenceNumber int64, partContext *PartContext) error {
	/*Flags          int32
	AppId          int64
	PartId         int64
	PartPackets       int64
	SequenceNumber int64
	Payload        []byte*/
	binary.BigEndian.PutUint32((*buf)[0:4], uint32(partContext.Flags))
	binary.BigEndian.PutUint64((*buf)[4:12], uint64(partContext.AppId))
	binary.BigEndian.PutUint64((*buf)[12:20], uint64(partContext.PartId))
	binary.BigEndian.PutUint64((*buf)[20:28], uint64(partContext.NumPackets))
	binary.BigEndian.PutUint64((*buf)[28:36], uint64(partContext.PartSize))
	binary.BigEndian.PutUint64((*buf)[36:44], uint64(sequenceNumber))
	return nil
}

func (bp *BinaryPartsPacketPacker) Pack(buf *[]byte, partContext *PartContext) error {
	sequenceNumber := partContext.GetNextSequenceNumber()
	binary.BigEndian.PutUint32((*buf)[0:4], uint32(partContext.Flags))
	binary.BigEndian.PutUint64((*buf)[4:12], uint64(partContext.AppId))
	binary.BigEndian.PutUint64((*buf)[12:20], uint64(partContext.PartId))
	binary.BigEndian.PutUint64((*buf)[20:28], uint64(partContext.NumPackets))
	binary.BigEndian.PutUint64((*buf)[28:36], uint64(partContext.PartSize))
	binary.BigEndian.PutUint64((*buf)[36:44], uint64(sequenceNumber))
	return nil
}
func (bp *BinaryPartsPacketPacker) Unpack(buf *[]byte, partContext *PartContext) (*PartPacket, error) {
	p := PartPacket{}
	p.Flags = int32(binary.BigEndian.Uint32((*buf)[0:4]))
	p.AppId = int64(binary.BigEndian.Uint64((*buf)[4:12]))
	p.PartId = int64(binary.BigEndian.Uint64((*buf)[12:20]))
	p.PartPackets = int64(binary.BigEndian.Uint64((*buf)[20:28]))
	p.PartSize = (int64(binary.BigEndian.Uint64((*buf)[28:36])))
	p.SequenceNumber = (int64(binary.BigEndian.Uint64((*buf)[36:44])))

	// TODO: Ensure new part announces without handshake
	if partContext.PartId > 0 && p.PartId != partContext.PartId {
		return nil, errors.New("mismatching partId")
	}

	*buf = (*buf)[partContext.PartsPacketPacker.GetHeaderLen():]
	return &p, nil
}
