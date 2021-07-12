package socket

import (
	"context"
	"crypto/md5"
	"fmt"
	optimizedconn "github.com/johannwagner/scion-optimized-connection/pkg"
	"github.com/netsec-ethz/scion-apps/pkg/appnet"
	"github.com/scionproto/scion/go/lib/snet"
	log "github.com/sirupsen/logrus"
	"net"
)

// Ensuring interface compatability at compile time.
var _ TransportSocket = &SCIONTransportSocket{}
var _ TransportPacketPacker = &SCIONPacketPacker{}

type SCIONTransportSocket struct {
	Conn *optimizedconn.OptimizedSCIONConn

	remoteAddr *snet.UDPAddr
	localAddr  *snet.UDPAddr
	listenAddr *net.UDPAddr
	Header     []byte
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

func (sts *SCIONTransportSocket) Listen(addr string, port int) error {

	log.Infof("addr=%v, port=%v", addr, port)
	addrStr := fmt.Sprintf("%s:%d", addr, port)

	listenAddr, err := snet.ParseUDPAddr(addrStr)
	sts.localAddr = listenAddr
	if err != nil {
		return err
	}

	sts.listenAddr = listenAddr.Host
	return nil
}
func (sts *SCIONTransportSocket) WritePart(bc *PartContext) (uint64, error) {
	// buffer := make([]byte, bc.RecommendedBufferSize)

	var n uint64 = 0
	log.Infof("Write with %d packets with md5 %x", bc.NumPackets, md5.Sum(bc.Data))
	for i := 0; i < bc.NumPackets; i++ {
		// start := i * bc.MaxPacketLength
		// packetBuffer := buffer[start : start+bc.MaxPacketLength]
		// log.Infof("Write on %d", bc.PartId)
		payload := bc.GetPayloadByPacketIndex(i)
		buf := make([]byte, len(payload)+bc.PartsPacketPacker.GetHeaderLen())
		// log.Infof("Copy %d bytes to packetbuffer %d with md5 %x", len(payload), len(buf), md5.Sum(payload))
		copy(buf[bc.PartsPacketPacker.GetHeaderLen():], payload)
		bc.SerializePacket(&buf)
		bts, err := bc.WriteToConn(sts.Conn, buf)
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
		// log.Infof("Received bytes %d", bts)
		bc.DeSerializePacket(&packetBuffer)

		// log.Infof("Extracting payload from %d to %d with md5 for %x", partStart, partStart+bc.PayloadLength, md5.Sum(packetBuffer))
		n += uint64(bts)
		bc.OnPartStatusChange(1, bts)
		// j++

		// log.Infof("Received payload at index %v", i)
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
	packetSerializer *optimizedconn.PacketSerializer
	remoteAddr *snet.UDPAddr
	localAddr  *snet.UDPAddr
}

func NewSCIONPacketPacker() TransportPacketPacker {
	return &SCIONPacketPacker{}
}

func (up *SCIONPacketPacker) SetRemote(remote string, remotePort int) {
	addrStr := fmt.Sprintf("%s:%d", remote, remotePort)
	remoteAddr, _ := snet.ParseUDPAddr(addrStr)
	up.remoteAddr = remoteAddr

	// We check, if there is a path.
	if remoteAddr.Path.IsEmpty() {
		err := appnet.SetDefaultPath(remoteAddr)
		if err != nil {
			panic(err)
		}
	}

	err := up.PrepareHeaderLen()
	if err != nil {
		panic(err)
	}

}

func (up *SCIONPacketPacker) SetLocal(local string, localPort int) {
	addrStr := fmt.Sprintf("%s:%d", local, localPort)
	localAddr, _ := snet.ParseUDPAddr(addrStr)
	up.localAddr = localAddr

	err := up.PrepareHeaderLen()
	if err != nil {
		panic(err)
	}
}

func (up *SCIONPacketPacker) PrepareHeaderLen() error {

	// Skipping, if it is not fully initialized
	if up.localAddr == nil || up.remoteAddr == nil {
		return nil
	}

	connectivityContext, err := optimizedconn.PrepareConnectivityContext(context.Background())
	if err != nil {
		return err
	}


	up.packetSerializer, err = optimizedconn.NewPacketSerializer(connectivityContext.LocalIA, up.localAddr.Host, up.remoteAddr)

	if err != nil {
		return err
	}

	return nil
}


func (up *SCIONPacketPacker) GetHeaderLen() int {
	return up.packetSerializer.GetHeaderLen()
}

func (up *SCIONPacketPacker) Pack(buf *[]byte, payloadLen int) error {
	return nil
}
func (up *SCIONPacketPacker) Unpack(buf *[]byte) error {
	return nil
}

