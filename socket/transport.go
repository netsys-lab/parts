package socket

import (
	"encoding/binary"
	"net"
	"sync"
	"time"

	"github.com/martenwallewein/blocks/utils"
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

type BlockContext struct {
	sync.Mutex
	SocketOptions         SocketOptions
	OnBlockStatusChange   OnBlockStatusChange
	BlocksPacketPacker    BlocksPacketPacker
	TransportPacketPacker TransportPacketPacker
	TransferFinished      bool
	MaxPacketLength       int
	// The number of bytes of the payload for each packet that has to be send
	// It must be used within send/receive block to retrieve how large the actual payload is
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
	BlockId                   int64
	TestingMode               bool
}

func (b *BlockContext) GetNextSequenceNumber() int64 {
	b.currentSequenceNumber++
	return b.currentSequenceNumber
}

func (b *BlockContext) GetPayloadByPacketIndex(i int) []byte {
	blockStart := i * b.PayloadLength
	end := utils.Min(blockStart+b.PayloadLength, len(b.Data))
	return b.Data[blockStart:end]
}

func (b *BlockContext) SetPayloadByPacketIndex(i int, payload []byte) {
	blockStart := i * b.PayloadLength
	end := utils.Min(blockStart+b.PayloadLength, len(b.Data))
	copy(b.Data[blockStart:end], payload)
}

func (b *BlockContext) WriteToConn(conn net.Conn, data []byte) (int, error) {
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

func (b *BlockContext) ReadFromConn(conn net.Conn, data []byte) (int, error) {
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

func (b *BlockContext) WriteToPacketConn(conn net.PacketConn, data []byte, adr net.Addr) (int, error) {
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

func (b *BlockContext) ReadFromPacketConn(conn net.PacketConn, data []byte) (int, net.Addr, error) {
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

func (b *BlockContext) Prepare() {
	b.MissingSequenceNums = make([]int64, 0)
	b.MissingSequenceNumOffsets = make([]int64, 0)
	b.HeaderLength = b.TransportPacketPacker.GetHeaderLen() + b.BlocksPacketPacker.GetHeaderLen()
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

func (b *BlockContext) SerializePacket(packetBuffer *[]byte) {

	b.BlocksPacketPacker.Pack(packetBuffer, b)
	b.TransportPacketPacker.Pack(packetBuffer, b.PayloadLength+b.BlocksPacketPacker.GetHeaderLen())
	// log.Infof("Send md5 for %x sequeceNumber %d", md5.Sum((*packetBuffer)[b.BlocksPacketPacker.GetHeaderLen():]), b.currentSequenceNumber)
}

func (b *BlockContext) SerializeRetransferPacket(packetBuffer *[]byte, sequenceNum int64) {

	b.BlocksPacketPacker.PackRetransfer(packetBuffer, sequenceNum, b)
	b.TransportPacketPacker.Pack(packetBuffer, b.PayloadLength+b.BlocksPacketPacker.GetHeaderLen())
	// log.Infof("Send md5 for %x sequeceNumber %d", md5.Sum((*packetBuffer)[b.BlocksPacketPacker.GetHeaderLen():]), b.currentSequenceNumber)
}

func (b *BlockContext) DeSerializePacket(packetBuffer *[]byte) {
	// log.Infof("Received ")

	b.TransportPacketPacker.Unpack(packetBuffer)
	p, _ := b.BlocksPacketPacker.Unpack(packetBuffer, b) // TODO: Error handling
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

	blockStart := int(p.SequenceNumber-1) * b.PayloadLength
	end := utils.Min(blockStart+b.PayloadLength, len(b.Data))
	copy(b.Data[blockStart:end], *packetBuffer)
	if diff > 0 {
		b.HighestSequenceNumber += diff
	}
}

type SocketOptions struct {
	// TBD
	UseMmsg bool
	// TBD
	UseGsoGro bool
	// Use the first path queried from SCION in SetDestination call
	// To ensure at every time there is a valid path
	StartWithDefaultPath bool
}

// Callback each time a send/receive call to the kernel is done and
// a number of bytes is returned. This is probably a single write/read call
// in default case, but can contain the sending/receiving of multiple messages
// if useMmsg or UseGsoGro is enabled and implemented correctly
type OnBlockStatusChange func(numMsgs int, completedBytes int)

type TransportSocket interface {
	// Returns the local Socket Address
	// GetLocalAddr() net.Addr // TODO: Support *snet.UDPAddr here
	Listen(addr string) error
	WriteBlock(blockContext *BlockContext) (uint64, error)
	ReadBlock(blockContext *BlockContext) (uint64, error)
	Write(buf []byte) (int, error)
	Read(buf []byte) (int, error)
	// SetPath(path *snet.Path) error TODO: SCION Specific: How to solve this
	Dial(addr string) error
}

type TransportPacketPacker interface {
	GetHeaderLen() int
	Pack(buf *[]byte, payloadLen int) error
	Unpack(buf *[]byte) error
}

type BlocksPacketPacker interface {
	GetHeaderLen() int
	Pack(buf *[]byte, blockContect *BlockContext) error
	PackRetransfer(buf *[]byte, sequenceNum int64, blockContect *BlockContext) error
	Unpack(buf *[]byte, blockContect *BlockContext) (*BlockPacket, error)
}

type UDPTransportSocket struct {
	Conn         *net.UDPConn
	LocalAddr    *net.UDPAddr
	RemoteAddr   *net.UDPAddr
	PacketBuffer []byte
}

func NewUDPTransportSocket() *UDPTransportSocket {
	return &UDPTransportSocket{}
}

func (uts *UDPTransportSocket) Listen(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	uts.RemoteAddr = udpAddr
	uts.Conn = udpConn
	return nil

}
func (uts *UDPTransportSocket) WriteBlock(bc *BlockContext) (uint64, error) {
	uts.PacketBuffer = make([]byte, bc.RecommendedBufferSize)
	var n uint64 = 0
	log.Infof("Write with %d packets", bc.NumPackets)
	for i := 0; i < bc.NumPackets; i++ {
		start := i * bc.MaxPacketLength
		packetBuffer := uts.PacketBuffer[start : start+bc.MaxPacketLength]
		payload := bc.GetPayloadByPacketIndex(i)
		copy(packetBuffer[bc.HeaderLength:], payload)

		bc.SerializePacket(&packetBuffer)
		// bts, err := uts.Conn.WriteTo(packetBuffer, uts.RemoteAddr)
		bts, err := bc.WriteToPacketConn(uts.Conn, packetBuffer, uts.RemoteAddr)
		bc.OnBlockStatusChange(1, bts)
		if err != nil {
			return 0, err
		}
		n += uint64(bts)
	}
	return n, nil
}
func (uts *UDPTransportSocket) ReadBlock(bc *BlockContext) (uint64, error) {

	uts.PacketBuffer = make([]byte, bc.RecommendedBufferSize)
	var n uint64 = 0
	j := 0
	for i := 0; i < bc.NumPackets; i++ {
		start := i * bc.MaxPacketLength
		packetBuffer := uts.PacketBuffer[start : start+bc.MaxPacketLength]
		// bts, err := uts.Conn.Read(packetBuffer)
		bts, err := bc.ReadFromConn(uts.Conn, packetBuffer)
		if err != nil {
			return 0, err
		}
		// To test retransfers, drop every 100 packets
		/*if j > 0 && j%1000 == 0 {
			j++
			i--
			continue
		}*/
		bc.DeSerializePacket(&packetBuffer)

		// log.Infof("Extracting payload from %d to %d with md5 for %x", blockStart, blockStart+bc.PayloadLength, md5.Sum(packetBuffer))
		n += uint64(bts)
		bc.OnBlockStatusChange(1, bts)
		j++
	}
	return n, nil
}

func (uts *UDPTransportSocket) Dial(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	uts.RemoteAddr = udpAddr
	return nil
}

func (uts *UDPTransportSocket) Write(buf []byte) (int, error) {
	return uts.Conn.WriteTo(buf, uts.RemoteAddr)
}
func (uts *UDPTransportSocket) Read(buf []byte) (int, error) {
	return uts.Conn.Read(buf)
}

type UDPPacketPacker struct {
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

func NewUDPTransportPacketPacker() TransportPacketPacker {
	return &UDPPacketPacker{}
}

func (up *UDPPacketPacker) GetHeaderLen() int {
	return 0
}

func (up *UDPPacketPacker) Pack(buf *[]byte, payloadLen int) error {
	return nil
}
func (up *UDPPacketPacker) Unpack(buf *[]byte) error {
	return nil
}
