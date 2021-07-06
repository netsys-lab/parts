package socket

/*
import (
	"fmt"
	"net"

	"github.com/netsec-ethz/scion-apps/pkg/appnet"
	"github.com/scionproto/scion/go/lib/snet"
)

type UnderlaySocket interface {
	GetLocalAddr() *snet.UDPAddr
	WriteRaw([]byte) (int, error)
	ReadRaw([]byte) (int, error)
	WritePacket([]byte) (int, error)
	ReadPacket([]byte) (int, error)
	Connect(string) error
	SetOptions(*SocketOptions)
	WriteRawMultiple([]byte, int) (int, error)
	ReadRawMultiple([]byte, int) (int, error)
	WriteMultiplePackets([]byte, int) (int, error)
	ReadMultiplePackets([]byte, int) (int, error)
	GetHeaderLen() int
	PackPacket(*[]byte, int, uint16) error
	UnpackPacket(*[]byte) (int, error)
	SetPath(*snet.Path) error
	SetDestination(string) error
}

type SCIONSocket struct {
	LocalAddrString string
	LocalAddr       *snet.UDPAddr
	Conn            *net.UDPConn
	DestAddrString  string
	DestAddr        *snet.UDPAddr
	Packer          *SCIONPacketPacker
}

func ParseSCIONAddr(addr string) {

}

func NewSCIONSocket(addr string, port int) (*SCIONSocket, error) {
	completeAddr := fmt.Sprintf("%s:%d", addr, port)
	snetAddr, err := appnet.ResolveUDPAddr(completeAddr)
	if err != nil {
		return nil, err
	}
	scionSocket := SCIONSocket{
		LocalAddrString: completeAddr,
		LocalAddr:       snetAddr,
	}
	return &scionSocket, nil
}

func (s *SCIONSocket) SetDestination(dstAddr string) error {
	snetAddr, err := appnet.ResolveUDPAddr(dstAddr)
	if err != nil {
		return err
	}

	pp, err := NewSCIONPacketPacker(dstAddr, s.LocalAddrString, snetAddr.Host.Port)
	if err != nil {
		return err
	}
	s.Packer = pp
	s.DestAddrString = dstAddr
	s.DestAddr = snetAddr

	return nil
}

func (s *SCIONSocket) SetPath(path *snet.Path) {
	appnet.SetPath(s.DestAddr, *path)
}

func (s *SCIONSocket) GetLocalAddr() *snet.UDPAddr {
	return s.LocalAddr
}

/*func (s *SCIONSocket) WriteRaw(buf []byte) (int, error) {
	payloadLen := len(buf)
	newBuf := make([]byte, s.GetHeaderLen()+payloadLen)
	s.Packer.Pack(&newBuf, 0, uint16(payloadLen))
	n, err := s.Conn.WriteTo(newBuf, s.DestAddr.NextHop)
	return n, err
}

func (s *SCIONSocket) ReadRaw([]byte) (int, error) {

}

func (s *SCIONSocket) WritePart([]byte) (int, error)
func (s *SCIONSocket) ReadPart([]byte) (int, error)
func (s *SCIONSocket) Connect(string) error
func (s *SCIONSocket) SetOptions(*SocketOptions)

/*func (s *SCIONSocket) WriteRawMultiple([]byte, int) (int, error)
func (s *SCIONSocket) ReadRawMultiple([]byte, int) (int, error)
func (s *SCIONSocket) WriteMultiplePackets([]byte, int) (int, error)
func (s *SCIONSocket) ReadMultiplePackets([]byte, int) (int, error)

func (s *SCIONSocket) GetHeaderLen() int
func (s *SCIONSocket) PackPacket(*[]byte, int, uint16) error
func (s *SCIONSocket) UnpackPacket(*[]byte) (int, error)
*/
