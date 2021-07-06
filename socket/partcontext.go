package socket

import (
	"net"
	"sync"
	"time"

	"github.com/martenwallewein/parts/utils"
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
	if b.TestingMode {
		b.Lock()
		start := testingState.TestingWriteIndex * b.MaxPacketLength
		end := utils.Min(start+b.MaxPacketLength, len(testingState.TestingReceiveBuffer))
		copy(testingState.TestingReceiveBuffer[start:end], data)
		testingState.TestingWriteIndex++
		log.Infof("Increase b.TestingWriteIndex with %p and val %d", &testingState.TestingWriteIndex, testingState.TestingWriteIndex)
		b.Unlock()
		return len(data), nil
	} else {
		return conn.Write(data)
	}
}

func (b *PartContext) ReadFromConn(conn net.Conn, data []byte) (int, error) {
	if b.TestingMode {
		// log.Infof("Read called")
		for testingState.TestingReadIndex >= testingState.TestingWriteIndex {
			log.Infof("Check b.TestingWriteIndex with %p and val %d", &testingState.TestingWriteIndex, testingState.TestingWriteIndex)
			time.Sleep(1000 * time.Millisecond)
		}
		// log.Infof("Read received")
		b.Lock()
		start := testingState.TestingReadIndex * b.MaxPacketLength
		end := utils.Min(start+b.MaxPacketLength, len(testingState.TestingReceiveBuffer))
		// log.Infof("Copy from %d to %d, %p", start, end, &testingState.TestingReceiveBuffer)
		// log.Info(testingState.TestingReceiveBuffer[start:end])
		copy(data, testingState.TestingReceiveBuffer[start:end])
		testingState.TestingReadIndex++
		b.Unlock()
		return len(data), nil
	} else {
		return conn.Read(data)
	}
}

func (b *PartContext) WriteToPacketConn(conn net.PacketConn, data []byte, adr net.Addr) (int, error) {
	if b.TestingMode {
		b.Lock()
		start := testingState.TestingWriteIndex * b.MaxPacketLength
		end := utils.Min(start+b.MaxPacketLength, len(testingState.TestingReceiveBuffer))
		copy(testingState.TestingReceiveBuffer[start:end], data)
		// log.Infof("Copied from %d to %d, %p", start, end, &testingState.TestingReceiveBuffer)
		// log.Info(testingState.TestingReceiveBuffer[start:end])
		testingState.TestingWriteIndex++
		// log.Infof("Increase b.TestingWriteIndex with %p and val %d", &testingState.TestingWriteIndex, testingState.TestingWriteIndex)
		b.Unlock()
		return len(data), nil
	} else {
		return conn.WriteTo(data, adr)
	}
}

func (b *PartContext) ReadFromPacketConn(conn net.PacketConn, data []byte) (int, net.Addr, error) {
	if b.TestingMode {
		for testingState.TestingReadIndex >= testingState.TestingWriteIndex {
			time.Sleep(1 * time.Microsecond)
		}
		b.Lock()
		start := testingState.TestingReadIndex * b.MaxPacketLength
		end := utils.Min(start+b.MaxPacketLength, len(testingState.TestingReceiveBuffer))
		copy(data, testingState.TestingReceiveBuffer[start:end])
		testingState.TestingReadIndex++
		b.Unlock()
		return len(data), nil, nil
	} else {
		return conn.ReadFrom(data)
	}
}

func (b *PartContext) Prepare() {
	b.MissingSequenceNums = make([]int64, 0)
	b.MissingSequenceNumOffsets = make([]int64, 0)
	b.HeaderLength = b.TransportPacketPacker.GetHeaderLen() + b.PartsPacketPacker.GetHeaderLen()
	b.PayloadLength = b.MaxPacketLength - b.HeaderLength
	b.NumPackets = utils.CeilForceInt(len(b.Data), b.PayloadLength)
	b.RecommendedBufferSize = b.NumPackets * b.MaxPacketLength
	log.Infof("Having HeaderLength %d, PayloadLength %d", b.HeaderLength, b.PayloadLength)
	log.Infof("Having NumPackets %d = len(b.Data) %d / b.PayloadLen %d", b.NumPackets, len(b.Data), b.PayloadLength)
	log.Infof("Recommended buffer size %d, numPackets %d * MaxPacketLen %d = %d", b.RecommendedBufferSize, b.NumPackets, b.MaxPacketLength, b.NumPackets*b.MaxPacketLength)

	if b.TestingMode && !testingState.BufferCreated {
		testingState.BufferCreated = true
		testingState.TestingReceiveBuffer = make([]byte, b.RecommendedBufferSize)
	}

}

func (b *PartContext) SerializePacket(packetBuffer *[]byte) {

	b.PartsPacketPacker.Pack(packetBuffer, b)
	b.TransportPacketPacker.Pack(packetBuffer, b.PayloadLength+b.PartsPacketPacker.GetHeaderLen())
	// log.Infof("Send md5 for %x sequeceNumber %d", md5.Sum((*packetBuffer)[b.PartsPacketPacker.GetHeaderLen():]), b.currentSequenceNumber)
}

func (b *PartContext) SerializeRetransferPacket(packetBuffer *[]byte, sequenceNum int64) {

	b.PartsPacketPacker.PackRetransfer(packetBuffer, sequenceNum, b)
	b.TransportPacketPacker.Pack(packetBuffer, b.PayloadLength+b.PartsPacketPacker.GetHeaderLen())
	// log.Infof("Send md5 for %x sequeceNumber %d", md5.Sum((*packetBuffer)[b.PartsPacketPacker.GetHeaderLen():]), b.currentSequenceNumber)
}

func (b *PartContext) DeSerializePacket(packetBuffer *[]byte) {
	// log.Infof("Received ")

	b.TransportPacketPacker.Unpack(packetBuffer)
	p, _ := b.PartsPacketPacker.Unpack(packetBuffer, b) // TODO: Error handling
	diff := p.SequenceNumber - b.HighestSequenceNumber
	// log.Infof("Got md5 for %x sequeceNumber %d", md5.Sum(*packetBuffer), b.highestSequenceNumber)
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
		// log.Infof("Received md5 %x for sequenceNumber %d", md5.Sum(*packetBuffer), p.SequenceNumber)
		b.Lock()
		index := utils.IndexOf(p.SequenceNumber, b.MissingSequenceNums)
		if index >= 0 {
			b.MissingSequenceNums = utils.RemoveFromSliceByIndex(b.MissingSequenceNums, int64(index))
			b.MissingSequenceNumOffsets = utils.RemoveFromSliceByIndex(b.MissingSequenceNumOffsets, int64(index))
		}

		b.Unlock()

	}

	partStart := int(p.SequenceNumber-1) * b.PayloadLength
	end := utils.Min(partStart+b.PayloadLength, len(b.Data))
	copy(b.Data[partStart:end], *packetBuffer)
	if diff > 0 {
		b.HighestSequenceNumber += diff
	}
}