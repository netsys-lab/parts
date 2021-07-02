package socket

import (
	"encoding/binary"
	"net"
	"sync"

	"github.com/martenwallewein/blocks/utils"
	log "github.com/sirupsen/logrus"
)

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
	PayloadLength         int
	NumPackets            int
	RecommendedBufferSize int
	HeaderLength          int
	Data                  []byte
	HighestSequenceNumber int64
	MissingSequenceNums   []int64
	currentSequenceNumber int64
	BlockId               int64
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

func (b *BlockContext) Prepare() {
	b.MissingSequenceNums = make([]int64, 0)
	b.HeaderLength = b.TransportPacketPacker.GetHeaderLen() + b.BlocksPacketPacker.GetHeaderLen()
	b.PayloadLength = b.MaxPacketLength - b.HeaderLength
	b.NumPackets = utils.CeilForceInt(len(b.Data), b.PayloadLength)
	b.RecommendedBufferSize = b.NumPackets * b.MaxPacketLength
	log.Infof("Having HeaderLength %d, PayloadLength %d", b.HeaderLength, b.PayloadLength)
	log.Infof("Having NumPackets %d = len(b.Data) %d / b.PayloadLen %d", b.NumPackets, len(b.Data), b.PayloadLength)
	log.Infof("Recommended buffer size %d, numPackets %d * MaxPacketLen %d = %d", b.RecommendedBufferSize, b.NumPackets, b.MaxPacketLength, b.NumPackets*b.MaxPacketLength)
}

func (b *BlockContext) SerializePacket(packetBuffer *[]byte) {

	b.BlocksPacketPacker.Pack(packetBuffer, b)
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
		var off int64 = 1
		for off < diff {
			// log.Infof("Append %d", p.SequenceNumber-off)
			b.Lock()
			b.MissingSequenceNums = utils.AppendIfMissing(b.MissingSequenceNums, p.SequenceNumber-off)
			b.Unlock()
			off++
		}
		// log.Infof("Appending missing sequence number %d to %d for highest number %d", p.SequenceNumber-off, p.SequenceNumber, highestSequenceNumber)
	} else if diff < 0 {
		// retransfer = true
		// log.Infof("Received retransferred sequence number %d", p.SequenceNumber)
		b.Lock()
		b.MissingSequenceNums = utils.RemoveFromSlice(b.MissingSequenceNums, p.SequenceNumber)
		b.Unlock()

	}
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
	for i := 0; i < bc.NumPackets; i++ {
		start := i * bc.MaxPacketLength
		packetBuffer := uts.PacketBuffer[start : start+bc.MaxPacketLength]
		blockStart := i * bc.PayloadLength
		end := utils.Min(blockStart+bc.PayloadLength, len(bc.Data))
		copy(packetBuffer[bc.HeaderLength:], bc.Data[blockStart:end])
		// log.Infof("Extracting payload from %d to %d with md5 for %x", blockStart, blockStart+bc.PayloadLength, md5.Sum(packetBuffer[bc.HeaderLength:]))
		bc.SerializePacket(&packetBuffer)
		bts, err := uts.Conn.WriteTo(packetBuffer, uts.RemoteAddr)
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
	for i := 0; i < bc.NumPackets; i++ {
		start := i * bc.MaxPacketLength
		packetBuffer := uts.PacketBuffer[start : start+bc.MaxPacketLength]
		bts, err := uts.Conn.Read(packetBuffer)
		if err != nil {
			return 0, err
		}
		bc.DeSerializePacket(&packetBuffer)
		blockStart := i * bc.PayloadLength
		end := utils.Min(blockStart+bc.PayloadLength, len(bc.Data))
		copy(bc.Data[blockStart:end], packetBuffer)
		// log.Infof("Extracting payload from %d to %d with md5 for %x", blockStart, blockStart+bc.PayloadLength, md5.Sum(packetBuffer))
		n += uint64(bts)
		bc.OnBlockStatusChange(1, bts)
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
