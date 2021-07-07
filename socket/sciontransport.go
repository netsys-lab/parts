package socket

import (
	"fmt"
	optimizedconn "github.com/johannwagner/scion-optimized-connection/pkg"
	"github.com/scionproto/scion/go/lib/snet"
	"net"

	log "github.com/sirupsen/logrus"
)

// Ensuring interface compatability at compile time.
var _ TransportSocket = &SCIONTransportSocket{}
var _ TransportPacketPacker = &SCIONPacketPacker{}

type SCIONTransportSocket struct {
	Conn *optimizedconn.OptimizedSCIONConn

	remoteAddr *snet.UDPAddr
	listenAddr *net.UDPAddr
}

func NewSCIONTransport() *SCIONTransportSocket {
	return &SCIONTransportSocket{}
}


func (sts *SCIONTransportSocket) Dial(addr string, port int) error {
	log.Infof("addr=%v, port=%v", addr, port)
	addrStr := fmt.Sprintf("%s:%d", addr, port)

 	remoteAddr, err := snet.ParseUDPAddr(addrStr)

	if err != nil {
		return err
	}

	sts.remoteAddr = remoteAddr
	optimizedConn, err := optimizedconn.Dial(sts.listenAddr, remoteAddr)

	if err != nil {
		return err
	}

	sts.Conn = optimizedConn

	return nil
}

func(sts *SCIONTransportSocket) Listen(addr string, port int) error {

	log.Infof("addr=%v, port=%v", addr, port)
	addrStr := fmt.Sprintf("%s:%d", addr, port)

	listenAddr, err := snet.ParseUDPAddr(addrStr)

	if err != nil {
		return err
	}

	sts.listenAddr = listenAddr.Host
	return nil
}
func (sts *SCIONTransportSocket) WritePart(bc *PartContext) (uint64, error) {
	buffer := make([]byte, bc.RecommendedBufferSize)

	var n uint64 = 0
	log.Infof("Write with %d packets", bc.NumPackets)
	for i := 0; i < bc.NumPackets; i++ {
		start := i * bc.MaxPacketLength
		packetBuffer := buffer[start : start+bc.MaxPacketLength]
		payload := bc.GetPayloadByPacketIndex(i)
		copy(packetBuffer[bc.HeaderLength:], payload)

		bc.SerializePacket(&packetBuffer)
		bts, err := bc.WriteToConn(sts.Conn, packetBuffer)
		bc.OnPartStatusChange(1, bts)
		if err != nil {
			return 0, err
		}
		n += uint64(bts)
	}
	return n, nil
}
func (sts *SCIONTransportSocket) ReadPart(bc *PartContext) (uint64, error) {

	buffer := make([]byte, bc.RecommendedBufferSize)
	var n uint64 = 0
	// j := 0
	for i := 0; i < bc.NumPackets; i++ {
		start := i * bc.MaxPacketLength
		packetBuffer := buffer[start : start+bc.MaxPacketLength]
		bts, err := bc.ReadFromConn(sts.Conn, packetBuffer)
		if err != nil {
			return 0, err
		}
		bc.DeSerializePacket(&packetBuffer)

		// log.Infof("Extracting payload from %d to %d with md5 for %x", partStart, partStart+bc.PayloadLength, md5.Sum(packetBuffer))
		n += uint64(bts)
		bc.OnPartStatusChange(1, bts)
		// j++
	}
	return n, nil
}



func (sts *SCIONTransportSocket) Write(buf []byte) (int, error) {
	return sts.Conn.Write(buf)
}
func (sts *SCIONTransportSocket) Read(buf []byte) (int, error) {
	return sts.Conn.Read(buf)
}

type SCIONPacketPacker struct {
}

func NewSCIONPacketPacker() TransportPacketPacker {
	return &SCIONPacketPacker{}
}

func (up *SCIONPacketPacker) GetHeaderLen() int {
	return 0
}

func (up *SCIONPacketPacker) Pack(buf *[]byte, payloadLen int) error {
	return nil
}
func (up *SCIONPacketPacker) Unpack(buf *[]byte) error {
	return nil
}
