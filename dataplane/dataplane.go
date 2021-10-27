package dataplane

import (
	"context"
	"crypto/md5"
	"net"
	"sync"
	"time"

	optimizedconn "github.com/johannwagner/scion-optimized-connection/pkg"
	"github.com/netsec-ethz/scion-apps/pkg/appnet"
	"github.com/scionproto/scion/go/lib/snet"
	log "github.com/sirupsen/logrus"
)

// Ensuring interface compatability at compile time.
var _ TransportDataplane = &SCIONDataplane{}
var _ TransportPacketPacker = &SCIONPacketPacker{}

const (
	DP_STATE_SENDING    = 100
	DP_STATE_RETRANSFER = 101
	DP_STATE_PENDING    = 200
	DP_STATE_RECEIVING  = 300
)

const (
	PACKET_SIZE = 1400
)

type SCIONDataplane struct {
	sync.Mutex
	Conn        *optimizedconn.OptimizedSCIONConn
	remoteAddr  *snet.UDPAddr
	localAddr   *snet.UDPAddr
	listenAddr  *net.UDPAddr
	Header      []byte
	PacketChan  chan []byte
	state       int
	partContext *PartContext
	metrics     *Metrics
}

func NewSCIONDataplane(packetChan chan []byte) *SCIONDataplane {
	return &SCIONDataplane{
		PacketChan: packetChan,
		metrics:    NewMetrics(100),
	}
}

func (sts *SCIONDataplane) SetPartContext(p *PartContext) {
	sts.partContext = p
}

func (sts *SCIONDataplane) LocalAddr() *snet.UDPAddr {
	return sts.localAddr
}

func (sts *SCIONDataplane) Stop() error {
	// TODO: Stop other things?
	return sts.Conn.Close()
}

func (sts *SCIONDataplane) RemoteAddr() *snet.UDPAddr {
	return sts.remoteAddr
}

func (sts *SCIONDataplane) SetDeadline(t time.Time) error {
	return sts.Conn.SetDeadline(t)
}

func (sts *SCIONDataplane) SetReadDeadline(t time.Time) error {
	return sts.Conn.SetReadDeadline(t)
}

func (sts *SCIONDataplane) SetWriteDeadline(t time.Time) error {
	return sts.Conn.SetWriteDeadline(t)
}

func (sts *SCIONDataplane) SetRemoteAddr(addr string) error {
	remoteAddr, err := snet.ParseUDPAddr(addr)
	if err != nil {
		return err
	}

	sts.remoteAddr = remoteAddr
	return nil
}

func (sts *SCIONDataplane) SetState(state int) {
	sts.Lock()
	sts.state = state
	switch state {
	case DP_STATE_SENDING:
	case DP_STATE_RECEIVING:
		sts.metrics.Collect()
		break

	case DP_STATE_PENDING:
		sts.metrics.Stop()
		// TODO: For loop reading control packets
		break
		// TODO: Skip retransfer for metrics?
	case DP_STATE_RETRANSFER:
		sts.metrics.Stop()
		break
	}
	sts.Unlock()
}

func (sts *SCIONDataplane) Dial(laddr, addr string) error {
	remoteAddr, err := snet.ParseUDPAddr(addr)
	if err != nil {
		return err
	}

	listenAddr, err := snet.ParseUDPAddr(laddr)
	if err != nil {
		return err
	}

	sts.listenAddr = listenAddr.Host
	sts.remoteAddr = remoteAddr
	optimizedConn, err := optimizedconn.Dial(sts.listenAddr, remoteAddr)
	if err != nil {
		return err
	}

	sts.Conn = optimizedConn
	sts.state = DP_STATE_PENDING
	return nil
}

func (sts *SCIONDataplane) Listen(addr string) error {
	listenAddr, err := snet.ParseUDPAddr(addr)
	sts.localAddr = listenAddr
	if err != nil {
		return err
	}
	sts.state = DP_STATE_PENDING
	sts.listenAddr = listenAddr.Host
	return nil
}

func (dp *SCIONDataplane) RetransferMissingPackets() {
	log.Infof("Entering retransfer")
	dp.state = DP_STATE_RETRANSFER
	for dp.state == DP_STATE_RETRANSFER {
		// log.Infof("Having %d missing sequenceNums with addr %p", len(b.partContext.MissingSequenceNums), &b.partContext.MissingSequenceNums)
		for i, v := range dp.partContext.MissingSequenceNums {
			if v == 0 {
				log.Fatal("error 0 sequenceNumber")
			}

			off := int(dp.partContext.MissingSequenceNumOffsets[i])
			// log.Infof("Sending back %d with offset %d", v-1, off)
			for j := 0; j < off; j++ {
				// packet := b.packets[v-1]

				// TODO: How to get already cached packet here, otherwise at least payload
				packet := dp.partContext.GetPayloadByPacketIndex(int(v) + j - 1)
				buf := make([]byte, len(packet)+dp.partContext.PartsPacketPacker.GetHeaderLen())
				copy(buf[dp.partContext.PartsPacketPacker.GetHeaderLen():], packet)
				// log.Infof("Retransferring md5 %x for sequenceNumber %d", md5.Sum(packet), v+int64(j))
				dp.partContext.SerializeRetransferPacket(&buf, v+int64(j))
				// log.Infof("Retransfer sequenceNum %d", v)
				bts, err := dp.Write(buf)
				dp.metrics.TxBytes += uint64(bts)
				dp.metrics.TxPackets += 1
				// TODO: Make this faster
				time.Sleep(10 * time.Microsecond)
				if err != nil {
					log.Fatal("error:", err)
				}
			}

		}
		dp.partContext.Lock()
		dp.partContext.MissingSequenceNums = make([]int64, 0)
		dp.partContext.MissingSequenceNumOffsets = make([]int64, 0)
		// log.Infof("Resetting retransfers")
		// TODO: Remove!
		dp.partContext.Unlock()
		time.Sleep(300 * time.Millisecond)
	}
}

func (sts *SCIONDataplane) WritePart(bc *PartContext) (uint64, error) {
	var n uint64 = 0
	log.Infof("Write with %d packets with md5 %x", bc.NumPackets, md5.Sum(bc.Data))
	for i := 0; i < bc.NumPackets; i++ {
		payload := bc.GetPayloadByPacketIndex(i)
		buf := make([]byte, len(payload)+bc.PartsPacketPacker.GetHeaderLen())
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
func (sts *SCIONDataplane) ReadSingle(bc *PartContext) (uint64, error) {

	buffer := make([]byte, bc.RecommendedBufferSize)
	var n uint64 = 0
	for {
		bts, err := bc.ReadFromConn(sts.Conn, buffer)
		if err != nil {
			return 0, err
		}
		err = bc.DeSerializePacket(&buffer)
		if err == nil {
			n += uint64(bts)
			bc.OnPartStatusChange(1, bts)
			break
		} else {
			// Pass to control plane to check if its some control packet
			sts.PacketChan <- buffer
		}
	}
	return n, nil
}

func (sts *SCIONDataplane) WriteSingle(bc *PartContext) (uint64, error) {
	var n uint64 = 0
	log.Infof("Write with %d packets with md5 %x", bc.NumPackets, md5.Sum(bc.Data))
	for i := 0; i < bc.NumPackets; i++ {
		payload := bc.GetPayloadByPacketIndex(i)
		buf := make([]byte, len(payload)+bc.PartsPacketPacker.GetHeaderLen())
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
func (sts *SCIONDataplane) ReadPart(bc *PartContext) (uint64, error) {

	buffer := make([]byte, bc.RecommendedBufferSize)
	var n uint64 = 0
	for i := 0; i < bc.NumPackets; i++ {
		start := i * bc.MaxPacketLength
		packetBuffer := buffer[start : start+bc.MaxPacketLength]
		bts, err := bc.ReadFromConn(sts.Conn, packetBuffer)
		if err != nil {
			return 0, err
		}
		err = bc.DeSerializePacket(&packetBuffer)
		if err == nil {
			n += uint64(bts)
			bc.OnPartStatusChange(1, bts)
		} else {
			// Pass to control plane to check if its some control packet
			sts.PacketChan <- packetBuffer
		}

	}
	return n, nil
}

func (sts *SCIONDataplane) Write(buf []byte) (int, error) {
	return sts.Conn.Write(buf)
}
func (sts *SCIONDataplane) Read(buf []byte) (int, error) {
	return sts.Conn.Read(buf)
}

type SCIONPacketPacker struct {
	packetSerializer *optimizedconn.PacketSerializer
	remoteAddr       *snet.UDPAddr
	localAddr        *snet.UDPAddr
}

func NewSCIONPacketPacker() TransportPacketPacker {
	return &SCIONPacketPacker{}
}

func (up *SCIONPacketPacker) SetRemote(remote *snet.UDPAddr) {
	up.remoteAddr = remote

	// We check, if there is a path.
	if remote.Path.IsEmpty() {
		err := appnet.SetDefaultPath(remote)
		if err != nil {
			panic(err)
		}
	}

	err := up.PrepareHeaderLen()
	if err != nil {
		panic(err)
	}

}

func (up *SCIONPacketPacker) SetLocal(local *snet.UDPAddr) {
	// TODO: error handling
	up.localAddr = local

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