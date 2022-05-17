package dataplane

import (
	"time"

	"github.com/scionproto/scion/go/lib/snet"
)

type PartRequestPacket struct {
	PartId                       int64
	LastSequenceNumber           int64
	NumPacketsPerTx              int
	PacketTxIndex                int
	MissingSequenceNumbers       []int64
	MissingSequenceNumberOffsets []int64
	TransactionId                int64
	RequestSequenceId            int
	NumPackets                   int64
	PartFinished                 bool
}

type PartPacket struct {
	Flags          int32
	AppId          int64
	PartId         int64
	PartPackets    int64
	SequenceNumber int64
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
type OnPartStatusChange func(numMsgs int, completedBytes int)

type TransportDataplane interface {
	// Opens a socket for the transport protocol
	Listen(addr string) error
	// Writes the complete part from partContext.Data in the way this interface likes to
	// Its recommended to send packets from begin to end, because
	// at the moment the retransfer logic relies on that.
	// May be changed in the future
	WritePart(partContext *PartContext) (uint64, error)
	// Read the complete part into partContext.Data
	ReadPart(partContext *PartContext) (uint64, error)
	WriteSingle([]byte, int64) (uint64, error)
	// Read the complete part into partContext.Data
	ReadSingle([]byte) (uint64, error)
	// Write a packet to the remoteAddr. Needs to insert the transport header for the transport socket
	// Could be done like this:
	// copy(packetBuffer[TransportPacketPacker.GetheaderLength():], payload)
	// TransportPacketPacker.Pack(&packetBuffer, len(payload))
	Write(buf []byte) (int, error)
	// Reads a packet from the remtoeAddr. Needs to remove the transport header.
	Read(buf []byte) (int, error)
	// Connects to the addr, so all WritePart and Write calls send to this destination
	Dial(laddr, addr string) error
	SetState(state int)
	LocalAddr() *snet.UDPAddr
	RemoteAddr() *snet.UDPAddr
	SetRemoteAddr(addr string) error
	RetransferMissingPackets()
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	Stop() error
	// Read the complete part into partContext.Data
	NextPartContext() (*PartContext, error)
}

type TransportPacketPacker interface {
	// Parts needs to know how large the header is
	// See scionpacker -> getHeaderFromEmptyPacket method to calculate
	// this beforehand as example
	GetHeaderLen() int
	// Buf is already padded to the GetHeaderLen.
	// This means the header can be copied into the first
	// GetHeaderLen() bytes
	Pack(buf *[]byte, payloadLen int) error
	// Remove the header from the buf and return only payload
	Unpack(buf *[]byte) error
	SetRemote(remoteAddr *snet.UDPAddr)
	SetLocal(localAddr *snet.UDPAddr)
}

type TransportSocketConstructor func(packetChan chan []byte) TransportDataplane
type TransportPackerConstructor func(packetChan chan []byte) TransportPacketPacker
