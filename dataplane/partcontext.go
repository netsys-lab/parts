package dataplane

import (
	"net"
	"os"
	"sync"

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
	NumPackets                int
	RecommendedBufferSize     int
	HeaderLength              int
	Data                      []byte
	HighestSequenceNumber     int64
	MissingSequenceNums       []int64
	MissingSequenceNumOffsets []int64
	currentSequenceNumber     int64
	PartId                    int64
	TestingMode               bool
	RecvPackets               int
}

func (b *PartContext) GetNextSequenceNumber() int64 {
	b.currentSequenceNumber++
	return b.currentSequenceNumber
}

func (b *PartContext) GetPayloadByPacketIndex(i int) []byte {
	partStart := i * b.PayloadLength
	end := utils.Min(partStart+b.PayloadLength, len(b.Data))
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

func (b *PartContext) Prepare() {
	b.MissingSequenceNums = make([]int64, 0)
	b.MissingSequenceNumOffsets = make([]int64, 0)
	b.HeaderLength = b.TransportPacketPacker.GetHeaderLen() + b.PartsPacketPacker.GetHeaderLen()
	b.PayloadLength = b.MaxPacketLength - b.HeaderLength
	b.NumPackets = utils.CeilForceInt(len(b.Data), b.PayloadLength)
	b.RecommendedBufferSize = b.NumPackets * b.MaxPacketLength
	log.Debugf("Having HeaderLength %d, PayloadLength %d", b.HeaderLength, b.PayloadLength)
	log.Debugf("Having NumPackets %d = len(b.Data) %d / b.PayloadLen %d", b.NumPackets, len(b.Data), b.PayloadLength)
	log.Debugf("Recommended buffer size %d, numPackets %d * MaxPacketLen %d = %d", b.RecommendedBufferSize, b.NumPackets, b.MaxPacketLength, b.NumPackets*b.MaxPacketLength)

	if b.TestingMode && !testingState.BufferCreated {
		testingState.BufferCreated = true
		testingState.TestingReceiveBuffer = make([]byte, b.RecommendedBufferSize)
	}

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
			log.Errorf("Got retransfer packet for sequenceNumber %d that was not handled properly", p.SequenceNumber)
			log.Error(b.MissingSequenceNums)
			log.Error(b.MissingSequenceNumOffsets)
			os.Exit(1)
		}

	}

	partStart := int(p.SequenceNumber-1) * b.PayloadLength
	end := utils.Min(partStart+b.PayloadLength, len(b.Data))
	copy(b.Data[partStart:end], *packetBuffer)
	// log.Infof("Copied %d bytes from %d to %d with payloadlen %d, md5 %x", len(b.Data[partStart:end]), partStart, end, b.PayloadLength, md5.Sum(b.Data[partStart:end]))
	// os.Exit(1)
	if diff > 0 {
		b.HighestSequenceNumber = p.SequenceNumber // +=diff
	}
	return nil
}
