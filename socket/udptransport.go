package socket

import (
	"net"

	log "github.com/sirupsen/logrus"
)

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
	// j := 0
	for i := 0; i < bc.NumPackets; i++ {
		start := i * bc.MaxPacketLength
		packetBuffer := uts.PacketBuffer[start : start+bc.MaxPacketLength]
		// bts, err := uts.Conn.Read(packetBuffer)
		bts, err := bc.ReadFromConn(uts.Conn, packetBuffer)
		if err != nil {
			return 0, err
		}
		// To test retransfers, drop every 1000 packets
		/*if j > 0 && j%1000 == 0 {
			j++
			i--
			continue
		}*/
		bc.DeSerializePacket(&packetBuffer)

		// log.Infof("Extracting payload from %d to %d with md5 for %x", blockStart, blockStart+bc.PayloadLength, md5.Sum(packetBuffer))
		n += uint64(bts)
		bc.OnBlockStatusChange(1, bts)
		// j++
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
