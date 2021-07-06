package socket

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

type TransportSocket interface {
	// Opens a socket for the transport protocol
	Listen(addr string) error
	// Writes the complete part from partContext.Data in the way this interface likes to
	// Its recommended to send packets from begin to end, because
	// at the moment the retransfer logic relies on that.
	// May be changed in the future
	WritePart(partContext *PartContext) (uint64, error)
	// Read the complete part into partContext.Data
	ReadPart(partContext *PartContext) (uint64, error)
	// Write a packet to the remoteAddr. Needs to insert the transport header for the transport socket
	// Could be done like this:
	// copy(packetBuffer[TransportPacketPacker.GetheaderLength():], payload)
	// TransportPacketPacker.Pack(&packetBuffer, len(payload))
	Write(buf []byte) (int, error)
	// Reads a packet from the remtoeAddr. Needs to remove the transport header.
	Read(buf []byte) (int, error)
	// Connects to the addr, so all WritePart and Write calls send to this destination
	Dial(addr string) error
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
}
