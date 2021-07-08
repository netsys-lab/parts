package socket

import (
	"context"
	"crypto/md5"
	"fmt"
	"net"
	"os"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	optimizedconn "github.com/johannwagner/scion-optimized-connection/pkg"
	"github.com/netsec-ethz/scion-apps/pkg/appnet"
	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/common"
	"github.com/scionproto/scion/go/lib/serrors"
	"github.com/scionproto/scion/go/lib/slayers"
	slayerspath "github.com/scionproto/scion/go/lib/slayers/path"
	"github.com/scionproto/scion/go/lib/slayers/path/scion"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/spath"
	"github.com/scionproto/scion/go/lib/topology/underlay"

	log "github.com/sirupsen/logrus"
)

// Ensuring interface compatability at compile time.
// var _ TransportSocket = &SCIONTransportSocket{}
// var _ TransportPacketPacker = &SCIONPacketPacker{}

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
	Header     []byte
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

	up.SetFirstPath()
	up.Header, _ = up.getHeaderFromEmptyPacket(make([]byte, 0))
	log.Infof("Created header with len %d", len(up.Header))

}

func (up *SCIONPacketPacker) SetLocal(local string, localPort int) {
	addrStr := fmt.Sprintf("%s:%d", local, localPort)
	localAddr, _ := snet.ParseUDPAddr(addrStr)
	up.localAddr = localAddr
}

func (up *SCIONPacketPacker) GetHeaderLen() int {
	return len(up.Header)
}

func (up *SCIONPacketPacker) Pack(buf *[]byte, payloadLen int) error {
	return nil
}
func (up *SCIONPacketPacker) Unpack(buf *[]byte) error {
	return nil
}

func (sts *SCIONPacketPacker) SetFirstPath() {
	paths, err := appnet.DefNetwork().PathQuerier.Query(context.Background(), sts.remoteAddr.IA)
	for i := range paths {
		fmt.Println("Path", i, ":", paths[i])
	}
	// sel_path, err := appnet.ChoosePathByMetric(appnet.Shortest, snet_udp_addr.IA)
	// sel_path, err := appnet.ChoosePathInteractive(snet_udp_addr.IA)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	appnet.SetPath(sts.remoteAddr, paths[0])
}

func (sts *SCIONPacketPacker) getHeaderFromEmptyPacket(pl []byte) ([]byte, error) {
	// TODO: Cache Header
	// off := spp.GetHeaderLen()
	var (
		dst     snet.SCIONAddress
		port    int
		path    spath.Path
		nextHop *net.UDPAddr
	)

	dst, port, path = snet.SCIONAddress{IA: sts.remoteAddr.IA, Host: addr.HostFromIP(sts.remoteAddr.Host.IP)},
		sts.remoteAddr.Host.Port, sts.remoteAddr.Path
	nextHop = sts.remoteAddr.NextHop
	// TODO: Fix AS internal
	if nextHop == nil && sts.localAddr.IA.Equal(sts.remoteAddr.IA) {
		nextHop = &net.UDPAddr{
			IP:   sts.remoteAddr.Host.IP,
			Port: underlay.EndhostPort,
			Zone: sts.remoteAddr.Host.Zone,
		}
	}
	fmt.Println(dst)
	fmt.Println(port)
	p := &snet.Packet{
		Bytes: make([]byte, 1400),
		PacketInfo: snet.PacketInfo{
			Destination: dst,
			Source: snet.SCIONAddress{IA: *&sts.localAddr.IA,
				Host: addr.HostFromIP(sts.localAddr.Host.IP)},
			Path: path,
			Payload: snet.UDPPayload{
				SrcPort: uint16(sts.localAddr.Host.Port),
				DstPort: uint16(port),
				Payload: pl,
			},
		},
	}
	p.Prepare()
	var packetLayers []gopacket.SerializableLayer

	var scionLayer slayers.SCION
	scionLayer.Version = 0
	// XXX(scrye): Do not set TrafficClass, to keep things simple while we
	// transition to HeaderV2. These should be added once the transition is
	// complete.

	// TODO(lukedirtwalker): Currently just set a pseudo value for the flow ID
	// until we have a better idea of how to set this correctly.
	scionLayer.FlowID = 1
	scionLayer.DstIA = p.Destination.IA
	scionLayer.SrcIA = p.Source.IA
	netDstAddr, err := hostAddrToNetAddr(p.Destination.Host)
	if err != nil {
		return nil, serrors.WrapStr("converting destination addr.HostAddr to net.Addr", err,
			"address", p.Destination.Host)
	}
	if err := scionLayer.SetDstAddr(netDstAddr); err != nil {
		return nil, serrors.WrapStr("setting destination address", err)
	}
	netSrcAddr, err := hostAddrToNetAddr(p.Source.Host)
	if err != nil {
		return nil, serrors.WrapStr("converting source addr.HostAddr to net.Addr", err,
			"address", p.Source.Host)
	}
	if err := scionLayer.SetSrcAddr(netSrcAddr); err != nil {
		return nil, serrors.WrapStr("settting source address", err)
	}

	scionLayer.PathType = p.Path.Type
	scionLayer.Path, err = slayerspath.NewPath(p.Path.Type)
	if err != nil {
		return nil, err
	}
	if err = scionLayer.Path.DecodeFromBytes(p.Path.Raw); err != nil {
		return nil, serrors.WrapStr("decoding path", err)
	}
	// XXX this is for convenience when debugging with delve
	if p.Path.Type == scion.PathType {
		sp := scionLayer.Path.(*scion.Raw)
		scionLayer.Path, err = sp.ToDecoded()
		if err != nil {
			return nil, err
		}
	}

	packetLayers = append(packetLayers, &scionLayer)
	scionLayer.NextHdr = common.L4UDP
	udpPayLoad := p.Payload.(snet.UDPPayload)
	udp := slayers.UDP{
		UDP: layers.UDP{
			SrcPort: layers.UDPPort(udpPayLoad.SrcPort),
			DstPort: layers.UDPPort(udpPayLoad.DstPort),
		},
	}
	udp.SetNetworkLayerForChecksum(&scionLayer)
	packetLayers = append(packetLayers, []gopacket.SerializableLayer{&udp, gopacket.Payload(udpPayLoad.Payload)}...)

	buffer := gopacket.NewSerializeBuffer()
	options := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	if err := gopacket.SerializeLayers(buffer, options, packetLayers...); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func netAddrToHostAddr(a net.Addr) (addr.HostAddr, error) {
	switch aImpl := a.(type) {
	case *net.IPAddr:
		return addr.HostFromIP(aImpl.IP), nil
	case addr.HostSVC:
		return aImpl, nil
	default:
		return nil, serrors.New("address not supported", "a", a)
	}
}

func hostAddrToNetAddr(a addr.HostAddr) (net.Addr, error) {
	switch aImpl := a.(type) {
	case addr.HostSVC:
		return aImpl, nil
	case addr.HostIPv4, addr.HostIPv6:
		return &net.IPAddr{IP: aImpl.IP()}, nil
	default:
		return nil, serrors.New("address not supported", "a", a)
	}
}
