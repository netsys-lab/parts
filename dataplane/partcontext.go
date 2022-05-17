package dataplane

import (
	"errors"
	"net"
	"sync"

	"github.com/netsys-lab/parts/shared"
	"github.com/netsys-lab/parts/utils"
	log "github.com/sirupsen/logrus"
)

type TestingState struct {
	sync.Mutex
	TestingReceiveBuffer []byte
	TestingWriteIndex    int
	TestingReadIndex     int
	BufferCreated        bool
}

var testingState TestingState

type PartContext struct {
	sync.Mutex
	SocketOptions         SocketOptions
	OnPartStatusChange    OnPartStatusChange
	PartsPacketPacker     PartsPacketPacker
	TransportPacketPacker TransportPacketPacker
	TransferFinished      bool
	MaxPacketLength       int
	// The number of bytes of the payload for each packet that has to be send
	// It must be used within send/receive part to retrieve how large the actual payload is
	// This must be defined by the TransportSocket, since its the only instance who knows
	// how big headers are actually.
	PayloadLength             int
	NumPackets                int64
	AppId                     int64
	Flags                     int32
	RecommendedBufferSize     int64
	HeaderLength              int
	Data                      []byte
	Buffer                    []byte
	HighestSequenceNumber     int64
	MissingSequenceNums       []int64
	MissingSequenceNumOffsets []int64
	currentSequenceNumber     int64
	PartId                    int64
	TestingMode               bool
	RecvPackets               int
	StartIndex                int64
}

func NewWaitingContext() *PartContext {
	return &PartContext{
		PartsPacketPacker:         NewBinaryPartsPacketPacker(),
		TransportPacketPacker:     NewSCIONPacketPacker(),
		MaxPacketLength:           PACKET_SIZE,
		MissingSequenceNums:       make([]int64, 0),
		MissingSequenceNumOffsets: make([]int64, 0),
	}
}

func (b *PartContext) GetNextSequenceNumber() int64 {
	b.currentSequenceNumber++
	return b.currentSequenceNumber
}

func (b *PartContext) GetPayloadByPacketIndex(i int64) []byte {
	partStart := i * int64(b.PayloadLength)
	end := utils.Min64(partStart+int64(b.PayloadLength), int64(len(b.Data)))
	return b.Data[partStart:end]
}

func (b *PartContext) SetPayloadByPacketIndex(i int, payload []byte) {
	partStart := i * b.PayloadLength
	end := utils.Min(partStart+b.PayloadLength, len(b.Data))
	copy(b.Data[partStart:end], payload)
}

func (b *PartContext) WriteToConn(conn net.Conn, data []byte) (int, error) {
	return conn.Write(data)

}

func (b *PartContext) ReadFromConn(conn net.Conn, data []byte) (int, error) {
	return conn.Read(data)
}

func (b *PartContext) WriteToPacketConn(conn net.PacketConn, data []byte, adr net.Addr) (int, error) {
	return conn.WriteTo(data, adr)
}

func (b *PartContext) ReadFromPacketConn(conn net.PacketConn, data []byte) (int, net.Addr, error) {
	return conn.ReadFrom(data)
}

func (b *PartContext) PrepareNew() {
	b.MissingSequenceNums = make([]int64, 0)
	b.MissingSequenceNumOffsets = make([]int64, 0)
	b.HeaderLength = b.TransportPacketPacker.GetHeaderLen() + b.PartsPacketPacker.GetHeaderLen()
	b.PayloadLength = b.MaxPacketLength - b.HeaderLength
	b.RecommendedBufferSize = b.NumPackets * int64(b.MaxPacketLength)
	log.Debugf("Having HeaderLength %d, PayloadLength %d", b.HeaderLength, b.PayloadLength)
	log.Debugf("Recommended buffer size %d, numPackets %d * MaxPacketLen %d = %d", b.RecommendedBufferSize, b.NumPackets, b.MaxPacketLength, b.NumPackets*int64(b.MaxPacketLength))
	b.AppId = shared.AppId
}

func (b *PartContext) Prepare() {
	b.MissingSequenceNums = make([]int64, 0)
	b.MissingSequenceNumOffsets = make([]int64, 0)
	b.HeaderLength = b.TransportPacketPacker.GetHeaderLen() + b.PartsPacketPacker.GetHeaderLen()
	b.PayloadLength = b.MaxPacketLength - b.HeaderLength
	b.NumPackets = utils.CeilForceInt64(int64(len(b.Data)), int64(b.PayloadLength))
	b.RecommendedBufferSize = b.NumPackets * int64(b.MaxPacketLength)
	log.Debugf("Having HeaderLength %d, PayloadLength %d", b.HeaderLength, b.PayloadLength)
	log.Debugf("Having NumPackets %d = len(b.Data) %d / b.PayloadLen %d", b.NumPackets, len(b.Data), b.PayloadLength)
	log.Debugf("Recommended buffer size %d, numPackets %d * MaxPacketLen %d = %d", b.RecommendedBufferSize, b.NumPackets, b.MaxPacketLength, b.NumPackets*int64(b.MaxPacketLength))

	if b.TestingMode && !testingState.BufferCreated {
		testingState.BufferCreated = true
		testingState.TestingReceiveBuffer = make([]byte, b.RecommendedBufferSize)
	}
	b.AppId = shared.AppId

}

func (b *PartContext) SerializePacket(packetBuffer *[]byte) {

	b.PartsPacketPacker.Pack(packetBuffer, b)
	b.TransportPacketPacker.Pack(packetBuffer, b.PayloadLength+b.PartsPacketPacker.GetHeaderLen())
	// log.Infof("Send md5 for %x sequeceNumber %d and len %d", md5.Sum((*packetBuffer)), b.currentSequenceNumber, len((*packetBuffer)))
}

func (b *PartContext) SerializeRetransferPacket(packetBuffer *[]byte, sequenceNum int64) {
	b.PartsPacketPacker.PackRetransfer(packetBuffer, sequenceNum, b)
	b.TransportPacketPacker.Pack(packetBuffer, b.PayloadLength+b.PartsPacketPacker.GetHeaderLen())
}

func (b *PartContext) DeSerializePacket(packetBuffer *[]byte) error {
	// log.Infof("Received ")

	b.TransportPacketPacker.Unpack(packetBuffer)
	p, err := b.PartsPacketPacker.Unpack(packetBuffer, b)
	if err != nil {
		return err
	}
	diff := p.SequenceNumber - b.HighestSequenceNumber
	// log.Infof("Got md5 for Payload %x ", md5.Sum((*packetBuffer)[:1320]))
	// log.Infof("Received SequenceNumber %d", p.SequenceNumber)
	if diff > 1 {
		// var off int64 = 1
		// for off < diff {
		b.Lock()
		index := utils.IndexOf(p.SequenceNumber-(diff-1), b.MissingSequenceNums)
		if index < 0 {
			b.MissingSequenceNums = append(b.MissingSequenceNums, p.SequenceNumber-(diff-1))
			b.MissingSequenceNumOffsets = append(b.MissingSequenceNumOffsets, diff-1)
			if diff > 100 {
				log.Debugf("PartId %d/%d: Add %d to missing sequence number offsets for seqNumber %d having highest SequenceNumber %d", p.PartId, b.PartId, diff-1, p.SequenceNumber, b.HighestSequenceNumber)
			}

			// if p.SequenceNumber == 6024 {
			//	log.Debugf("Setting highestSeqNumber from %d to 6024", b.HighestSequenceNumber)
			//}
			b.HighestSequenceNumber = p.SequenceNumber // TODO: Why do we need this duplicated here?
		}

		b.Unlock()
		//	off++
		// }
		// log.Infof("Appending missing sequence number %d to %d for highest number %d", p.SequenceNumber-off, p.SequenceNumber, highestSequenceNumber)
	} else if diff < 0 {
		// retransfer = true
		// log.Infof("Received retransferred sequence number %d", p.SequenceNumber)
		// log.Infof("Received retransfer md5 %x for sequenceNumber %d", md5.Sum(*packetBuffer), p.SequenceNumber)

		index := utils.IndexOfMin(p.SequenceNumber, b.MissingSequenceNums)
		if index >= 0 {
			b.Lock()
			// b.MissingSequenceNums[index]++
			b.MissingSequenceNumOffsets[index]--
			if b.MissingSequenceNumOffsets[index] == 0 {
				b.MissingSequenceNums = utils.RemoveFromSliceByIndex(b.MissingSequenceNums, int64(index))
				b.MissingSequenceNumOffsets = utils.RemoveFromSliceByIndex(b.MissingSequenceNumOffsets, int64(index))
				// log.Warnf("Removed offset from missingNumbers at index %d, %d remaining", index, len(b.MissingSequenceNumOffsets))
				// log.Error(b.MissingSequenceNums)
				// log.Error(b.MissingSequenceNumOffsets)
				// log.Errorf("Got %d from %d packets", b.RecvPackets, b.NumPackets)
			}
			b.Unlock()
		} else {
			log.Debugf("Warn: Got retransfer packet for sequenceNumber %d that was not handled properly", p.SequenceNumber)
			// log.Error(b.MissingSequenceNums)
			// log.Error(b.MissingSequenceNumOffsets)
			// os.Exit(1)
			return errors.New("Duplicate Retransfer")
		}

	}

	partStart := int(p.SequenceNumber-1) * b.PayloadLength
	end := utils.Min(partStart+b.PayloadLength, len(b.Data))
	copy(b.Data[partStart:end], *packetBuffer)

	if diff > 0 {

		b.HighestSequenceNumber = p.SequenceNumber // +=diff

	}
	return nil
}

func (b *PartContext) DeSerializeNewPacket(packetBuffer *[]byte) error {
	// log.Infof("Received ")

	b.TransportPacketPacker.Unpack(packetBuffer)
	p, err := b.PartsPacketPacker.Unpack(packetBuffer, b)
	if err != nil {
		return err
	}

	// create buffer etc
	b.PartId = p.PartId
	b.NumPackets = p.PartPackets
	b.PrepareNew()
	b.Data = make([]byte, b.RecommendedBufferSize)

	// TODO: What happens when first part packet is lost
	partStart := int(p.SequenceNumber-1) * b.PayloadLength
	end := utils.Min(partStart+b.PayloadLength, len(b.Data))
	copy(b.Data[partStart:end], *packetBuffer)
	b.HighestSequenceNumber = p.SequenceNumber // +=diff
	b.StartIndex = 1

	return nil
}
