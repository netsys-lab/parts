package socket

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
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
)

type PacketPacker interface {
	GetHeaderLen() int
	Pack([]byte, int) error
	Unpack([]byte) error
}

type SCIONPacketPacker struct {
	DstAddrString   string
	LocalAddrString string
	DestAddr        *snet.UDPAddr
	Header          []byte
	LocalIA         *addr.IA
	LocalAddr       *net.UDPAddr
}

func (spp *SCIONPacketPacker) SetFirstPath() {
	snet_udp_addr, err := appnet.ResolveUDPAddr(spp.DstAddrString)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	spp.DestAddr = snet_udp_addr
	paths, err := appnet.DefNetwork().PathQuerier.Query(context.Background(), snet_udp_addr.IA)
	for i := range paths {
		fmt.Println("Path", i, ":", paths[i])
	}
	// sel_path, err := appnet.ChoosePathByMetric(appnet.Shortest, snet_udp_addr.IA)
	// sel_path, err := appnet.ChoosePathInteractive(snet_udp_addr.IA)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	appnet.SetPath(snet_udp_addr, paths[0])
}

func NewSCIONPacketPacker(dst string, localAddr string, dstPort int) (*SCIONPacketPacker, error) {

	spp := SCIONPacketPacker{
		DstAddrString:   dst,
		LocalAddrString: localAddr,
	}
	var err error
	lAddr, err := appnet.ResolveUDPAddr(localAddr)
	if err != nil {
		return nil, err
	}
	spp.LocalAddr = &net.UDPAddr{
		IP:   lAddr.Host.IP,
		Port: lAddr.Host.Port,
	}
	spp.LocalIA = &lAddr.IA
	spp.SetFirstPath()
	spp.DestAddr.Host.Port = dstPort
	fmt.Println(spp.DestAddr)

	spp.Header, err = spp.getHeaderFromEmptyPacket(make([]byte, 0))
	fmt.Printf("Get Payload size %d\n", binary.BigEndian.Uint16(spp.Header[6:8]))
	if err != nil {
		return nil, err
	}
	fmt.Printf("SCION HEADER WITH LEN %d:\n", len(spp.Header))
	fmt.Println(spp.Header)

	return &spp, nil
}

func (spp *SCIONPacketPacker) GetHeaderLen() int {
	return len(spp.Header)
}
func (spp *SCIONPacketPacker) Pack(buf *[]byte, payloadLen uint16) {
	*buf = append(*buf, spp.Header...)
	binary.BigEndian.PutUint16((*buf)[6:8], payloadLen)
	// fmt.Printf("Having payloadLen %d\n", payloadLen)
}

// This is really dirty, no checks at all
// But to start with this should be fine
func (spp *SCIONPacketPacker) Unpack(buf *[]byte) (int, error) {
	scnLen := (*buf)[5]
	targetOffset := scnLen * 4
	// SCION UDP HEADER is 8 byte, ignore previos stuff
	targetOffset += 8
	*buf = (*buf)[targetOffset:]
	return len(*buf), nil
}

// This is the quick and dirty hack for packing SCION packets
/*func (spp *SCIONPacketPacker) Pack(buf []byte, payloadStart int) {
	// TODO: Cache Header
	// off := spp.GetHeaderLen()
	var (
		dst     snet.SCIONAddress
		port    int
		path    spath.Path
		nextHop *net.UDPAddr
	)

	dst, port, path = snet.SCIONAddress{IA: spp.DestAddr.IA, Host: addr.HostFromIP(spp.DestAddr.Host.IP)},
		spp.DestAddr.Host.Port, spp.DestAddr.Path
	nextHop = spp.DestAddr.NextHop
	if nextHop == nil && spp.LocalIA.Equal(spp.DestAddr.IA) {
		nextHop = &net.UDPAddr{
			IP:   spp.DestAddr.Host.IP,
			Port: underlay.EndhostPort,
			Zone: spp.DestAddr.Host.Zone,
		}
	}

	pkt := &snet.Packet{
		Bytes: buf,
		PacketInfo: snet.PacketInfo{
			Destination: dst,
			Source: snet.SCIONAddress{IA: *spp.LocalIA,
				Host: addr.HostFromIP(spp.LocalAddr.IP)},
			Path: path,
			Payload: snet.UDPPayload{
				SrcPort: uint16(spp.LocalAddr.Port),
				DstPort: uint16(port),
				Payload: buf[payloadStart:],
			},
		},
	}

	pkt.Serialize()
	copy(buf, pkt.Bytes)
}*/

func (spp *SCIONPacketPacker) getHeaderFromEmptyPacket(pl []byte) ([]byte, error) {
	// TODO: Cache Header
	// off := spp.GetHeaderLen()
	var (
		dst     snet.SCIONAddress
		port    int
		path    spath.Path
		nextHop *net.UDPAddr
	)

	dst, port, path = snet.SCIONAddress{IA: spp.DestAddr.IA, Host: addr.HostFromIP(spp.DestAddr.Host.IP)},
		spp.DestAddr.Host.Port, spp.DestAddr.Path
	nextHop = spp.DestAddr.NextHop
	if nextHop == nil && spp.LocalIA.Equal(spp.DestAddr.IA) {
		nextHop = &net.UDPAddr{
			IP:   spp.DestAddr.Host.IP,
			Port: underlay.EndhostPort,
			Zone: spp.DestAddr.Host.Zone,
		}
	}
	fmt.Println(dst)
	fmt.Println(port)
	p := &snet.Packet{
		Bytes: make([]byte, 1400),
		PacketInfo: snet.PacketInfo{
			Destination: dst,
			Source: snet.SCIONAddress{IA: *spp.LocalIA,
				Host: addr.HostFromIP(spp.LocalAddr.IP)},
			Path: path,
			Payload: snet.UDPPayload{
				SrcPort: uint16(spp.LocalAddr.Port),
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

/*func (spp *SCIONPacketPacker) Unpack(buf []byte) (int, error) {
	payloadStart := 0
	p := snet.Packet{
		Bytes: buf,
	}

	var (
		scionLayer slayers.SCION
		udpLayer   slayers.UDP
		scmpLayer  slayers.SCMP
	)
	parser := gopacket.NewDecodingLayerParser(
		slayers.LayerTypeSCION, &scionLayer, &udpLayer, &scmpLayer,
	)
	parser.IgnoreUnsupported = true
	decoded := make([]gopacket.LayerType, 3)
	if err := parser.DecodeLayers(p.Bytes, &decoded); err != nil {
		return 0, err
	}
	if len(decoded) < 2 {
		return 0, serrors.New("L4 not decoded")
	}
	l4 := decoded[len(decoded)-1]
	if l4 != slayers.LayerTypeSCMP && l4 != slayers.LayerTypeSCIONUDP {
		return 0, serrors.New("unknown L4 layer decoded", "type", l4)
	}
	dstAddr, err := scionLayer.DstAddr()
	if err != nil {
		return 0, serrors.WrapStr("extracting destination address", err)
	}
	dstHost, err := netAddrToHostAddr(dstAddr)
	if err != nil {
		return 0, serrors.WrapStr("converting dst address to HostAddr", err)
	}
	srcAddr, err := scionLayer.SrcAddr()
	if err != nil {
		return 0, serrors.WrapStr("extracting source address", err)
	}
	srcHost, err := netAddrToHostAddr(srcAddr)
	if err != nil {
		return 0, serrors.WrapStr("converting src address to HostAddr", err)
	}
	p.Destination = snet.SCIONAddress{IA: scionLayer.DstIA, Host: dstHost}
	p.Source = snet.SCIONAddress{IA: scionLayer.SrcIA, Host: srcHost}
	// A path of length 4 is an empty path, because it only contains the mandatory
	// minimal header.
	if l := scionLayer.Path.Len(); l > 4 {
		pathCopy := make([]byte, scionLayer.Path.Len())
		if err := scionLayer.Path.SerializeTo(pathCopy); err != nil {
			return 0, serrors.WrapStr("extracting path", err)
		}
		p.Path = spath.Path{Raw: pathCopy, Type: scionLayer.PathType}
	} else {
		p.Path = spath.Path{}
	}
	switch l4 {
	case slayers.LayerTypeSCIONUDP:
		p.Payload = snet.UDPPayload{
			SrcPort: uint16(udpLayer.SrcPort),
			DstPort: uint16(udpLayer.DstPort),
			Payload: udpLayer.Payload,
		}
		// TODO: ReadFrom?
		copy(buf, udpLayer.Payload)
		return 0, nil
	}
	return payloadStart, serrors.New("unhandled SCMP type", "type", scmpLayer.TypeCode, "src", p.Source)
}
*/

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