package parts

import (
	"crypto/md5"
	"net"
	"sync"
	"time"

	"github.com/martenwallewein/parts/controlplane"
	"github.com/martenwallewein/parts/dataplane"
	log "github.com/sirupsen/logrus"
)

type Conn interface {
	net.Conn
}

var _ Conn = (*PartsConn)(nil)

type PartsConn struct {
	sync.Mutex
	part         []byte
	PartId       int64
	localAddr    string
	remoteAddr   string
	dataplane    dataplane.TransportDataplane
	controlplane controlplane.ControlPlane
	/*localStartPort             int
	remoteStartPort            int
	lastRequestedSequenceIndex []int64
	retransferPackets          [][]byte
	missingSequenceNums        []int64
	mode                       int
	Metrics                    partmetrics.SocketMetrics
	TransportSocket            socket.TransportSocket
	ControlPlane               *control.ControlPlane
	partContext                *socket.PartContext
	TestingMode                bool
	// transportSocketConstructor socket.TransportSocketConstructor
	// transportPackerConstructor socket.TransportPackerConstructor
	MaxSpeed    int64
	RateControl *control.RateControl
	NumCons     int*/
}

func newPartsConn() (*PartsConn, error) {
	partsConn := &PartsConn{}
	packetChan := make(chan []byte, 64)
	partsConn.dataplane = dataplane.NewSCIONDataplane(packetChan)
	cp, err := controlplane.NewControlPlane(partsConn.dataplane, packetChan)
	if err != nil {
		return nil, err
	}
	partsConn.controlplane = *cp
	/*partsConn.RateControl = control.NewRateControl(
		100,
		partsConn.MaxSpeed,
		PACKET_SIZE,
		numCons,
	)*/

	return partsConn, nil
}

func Listen(localAddr string) (*PartsConn, error) {
	pc, err := newPartsConn()
	if err != nil {
		return nil, err
	}
	err = pc.dataplane.Listen(localAddr)
	if err != nil {
		return nil, err
	}

	return pc, nil
}

func Dial(localAddr, remoteAddr string) (*PartsConn, error) {
	pc, err := newPartsConn()
	if err != nil {
		return nil, err
	}
	err = pc.dataplane.Dial(localAddr, remoteAddr)
	if err != nil {
		return nil, err
	}

	// Setting dataplane remote
	err = pc.controlplane.InitialHandshake(make([]byte, 0))
	if err != nil {
		return nil, err
	}

	return pc, nil
}

func (p *PartsConn) Accept() error {
	return p.controlplane.Accept()
}
func (p *PartsConn) Read(b []byte) (n int, err error) {
	// TODO: Not overwrite if actually receiving
	// p.part = b
	// p.PartId = partId

	// Check if Read is a single packet or larger one
	if len(b) >= dataplane.PACKET_PAYLOAD_SIZE { // TODO correct size here...
		// p.controlplane.A
		partContext, err := p.controlplane.AwaitHandshake(b)
		if err != nil {
			return 0, err
		}
		p.controlplane.SetPartContext(partContext)
		p.controlplane.Ratecontrol.Start()
		p.controlplane.StartReadpart()
		_, err = p.dataplane.ReadPart(partContext)
		if err != nil {
			return 0, err
		}
		p.controlplane.FinishReadpart()
		// secondsBandwidth := (int64(len(partContext.Data)/1024/1024) * 8) / int64(elapsedTime/time.Second)
		log.Debugf("Received %d packets, partLen %d and md5 %x", partContext.NumPackets, len(b), md5.Sum(b))
		return len(b), nil
		// p.partContext = partContext ?
	} else {
		n, err := p.dataplane.ReadSingle(b)
		return int(n), err
	}
}

func (p *PartsConn) Write(b []byte) (n int, err error) {
	// TODO: Save activePartCount and increase immediatly
	// TODO: Not overwrite if actually sending
	if len(b) >= dataplane.PACKET_PAYLOAD_SIZE {
		partContext, err := p.controlplane.Handshake(b)
		if err != nil {
			return 0, err
		}
		p.controlplane.SetPartContext(partContext)
		p.controlplane.Ratecontrol.Start()
		log.Debugf("Writing %d packets", partContext.NumPackets)
		// b.packets[b.activePartCount] = make([][]byte, len(part)/PACKET_SIZE)
		// TODO: Waiting queue
		// TODO: sync write calls

		// b.RateControl.Start()
		p.controlplane.StartWritepart()
		n, err := p.dataplane.WritePart(partContext)
		if err != nil {
			return 0, err
		}

		log.Debugf("Wrote %d packets, partLen %d", partContext.NumPackets, len(b))
		p.dataplane.RetransferMissingPackets()
		p.controlplane.Ratecontrol.Stop()
		p.controlplane.FinishWritepart()
		return int(n), nil
	} else {
		partid := p.controlplane.NextPartId()
		n, err := p.dataplane.WriteSingle(b, partid)
		return int(n), err
	}

	// time.Sleep(100 * time.Second)

}

func (p *PartsConn) Close() error {
	return p.dataplane.Stop()
}

func (p *PartsConn) LocalAddr() net.Addr {
	return p.dataplane.LocalAddr()
}

func (p *PartsConn) RemoteAddr() net.Addr {
	return p.dataplane.RemoteAddr()
}

func (p *PartsConn) SetDeadline(t time.Time) error {
	return p.dataplane.SetDeadline(t)
}

func (p *PartsConn) SetReadDeadline(t time.Time) error {
	return p.dataplane.SetReadDeadline(t)
}

func (p *PartsConn) SetWriteDeadline(t time.Time) error {
	return p.dataplane.SetWriteDeadline(t)
}
