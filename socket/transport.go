package socket

import (
	"net"
	"sync"

	"github.com/martenwallewein/blocks/utils"
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
}

func (b *BlockContext) Prepare() {
	b.NumPackets = utils.CeilForceInt(len(b.Data), b.MaxPacketLength)
	b.HeaderLength = b.TransportPacketPacker.GetHeaderLen() + b.BlocksPacketPacker.GetHeaderLen()
	b.PayloadLength = b.MaxPacketLength - b.HeaderLength
	b.RecommendedBufferSize = len(b.Data) + (b.NumPackets * b.HeaderLength)
}

func (b *BlockContext) SerializePacket(packetBuffer *[]byte) {
	b.TransportPacketPacker.Pack(packetBuffer, b.PayloadLength)
	b.BlocksPacketPacker.Pack(packetBuffer, b)
}

func (b *BlockContext) DeSerializePacket(packetBuffer *[]byte) {
	b.BlocksPacketPacker.Unpack(packetBuffer, b)
	b.TransportPacketPacker.Unpack(packetBuffer)
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
	Listen(addr string)
	WriteBlock(block []byte, blockContext *BlockContext) (uint64, error)
	ReadBlock(block []byte, blockContext *BlockContext) (uint64, error)
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
	Unpack(buf *[]byte, blockContect *BlockContext) error
}

type UDPTransportSocket struct {
	Conn         *net.UDPConn
	LocalAddr    *net.UDPAddr
	RemoteAddr   *net.UDPAddr
	PacketBuffer []byte
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
		copy(packetBuffer, bc.Data[blockStart:blockStart+bc.PayloadLength])
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
		copy(bc.Data[blockStart:blockStart+bc.PayloadLength], packetBuffer)
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
