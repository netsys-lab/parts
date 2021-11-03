package controlplane

import (
	"bytes"
	"encoding/gob"
	"sync"
	"time"

	"github.com/martenwallewein/parts/dataplane"
	"github.com/martenwallewein/parts/utils"
	log "github.com/sirupsen/logrus"
)

const (
	CP_STATE_SENDING    = 100
	CP_STATE_RETRANSFER = 101
	CP_STATE_PENDING    = 200
	CP_STATE_RECEIVING  = 300
)

type ControlPlane struct {
	sync.Mutex
	Dataplane                 dataplane.TransportDataplane
	packetChan                chan []byte
	state                     int
	PartContext               *dataplane.PartContext
	Ratecontrol               *RateControl
	currentPartId             int64
	stopCtrlPacketsChan       chan bool
	stopRetransferPacketsChan chan bool
}

func NewControlPlane(
	dp dataplane.TransportDataplane,
	packetChan chan []byte,
) (*ControlPlane, error) {
	// Make that thing Transport safe
	cp := ControlPlane{
		Dataplane:                 dp,
		packetChan:                packetChan,
		stopCtrlPacketsChan:       make(chan bool, 0),
		stopRetransferPacketsChan: make(chan bool),
		// TODO: Fix these parameters
		Ratecontrol: NewRateControl(),
	}

	return &cp, nil
}

func (cp *ControlPlane) SetPartContext(p *dataplane.PartContext) {
	cp.PartContext = p
}

func (cp *ControlPlane) SetState(state int) {
	cp.Lock()
	cp.state = state
	cp.Unlock()
}

func (cp *ControlPlane) Accept() error {
	hs := NewPartsHandshake()
	cp.Dataplane.Read(hs.raw)
	err := hs.Decode()
	if err != nil {
		return err
	}

	if cp.Dataplane.RemoteAddr() == nil {
		cp.Dataplane.SetRemoteAddr(hs.LocalAddr)
	}

	return nil
}

// Starts the whole controlplane
// TODO: Control channel
func (cp *ControlPlane) Run() {
	/*stopCtrlPacketsChan := make(chan bool)
	stopRetransferPacketsChan := make(chan bool)
	go cp.readControlPackets(&stopCtrlPacketsChan)
	go cp.requestRetransfers(&stopRetransferPacketsChan)*/
}

func (cp *ControlPlane) StartWritepart() {
	go cp.HandleAckPackets(&cp.stopCtrlPacketsChan)
}

func (cp *ControlPlane) StartReadpart() {
	go cp.requestRetransfers(&cp.stopRetransferPacketsChan)
}

func (cp *ControlPlane) FinishWritepart() {
	log.Infof("CP %p", &cp.stopCtrlPacketsChan)
	// cp.stopCtrlPacketsChan <- true TODO: Fix this channel issue
}

func (cp *ControlPlane) FinishReadpart() {
	cp.sendPartFinish() // TODO: error handling
	cp.stopRetransferPacketsChan <- true
}

func (cp *ControlPlane) HandleAckPackets(stopChan *chan bool) {
	ch := make(chan []byte)
	eCh := make(chan error)
	go func() {
		for {
			buffer := make([]byte, dataplane.PACKET_SIZE)
			_, err := cp.Dataplane.Read(buffer)
			if err != nil {
				eCh <- err
			} else {
				ch <- buffer
			}
		}
	}()
	var err error
	for {
		select {
		case <-*stopChan:
			return
		case err = <-eCh:
			if err != nil {
				log.Error("Failed to read ack packet %v", err)
				continue
			}
			break
		case buffer := <-ch:
			log.Infof("CP %p", stopChan)
			ackPacket, err := cp.parsePartAckPacket(buffer)
			if err != nil {
				log.Error("Failed to read ack packet %v", err)
				continue
			}
			finished := cp.handlePartAckPacket(ackPacket)
			if finished {
				return
			}
			break
		}
	}

}

func (cp *ControlPlane) readControlPackets(stopChan *chan bool) {
	for {
		select {
		case <-*stopChan:
			return
		case packet := <-cp.packetChan:
			switch cp.state {
			// Must be new handshake packet
			case CP_STATE_PENDING:
				break
			// Must be Ack packet
			case CP_STATE_SENDING:
				ackPacket, err := cp.parsePartAckPacket(packet)
				if err != nil {
					log.Error("Failed to read ack packet %v", err)
					continue
				}
				cp.handlePartAckPacket(ackPacket)
				break
			}
		}
	}
}

func (cp *ControlPlane) NextPartId() int64 {
	cp.currentPartId += 1
	return cp.currentPartId
}

func (cp *ControlPlane) InitialHandshake(data []byte) error {
	nextPartId := cp.NextPartId()
	hs := NewPartsHandshake()

	// TODO: Await Answer
	hs.PartTransfers = make([]PartTransfer, 1)
	hs.PartTransfers[0] = PartTransfer{
		PartId:   uint64(nextPartId),
		PartSize: uint64(len(data)),
		Port:     0,
	}
	hs.LocalAddr = cp.Dataplane.LocalAddr().String()

	err := hs.Encode()
	if err != nil {
		return err
	}

	cp.Dataplane.Write(hs.raw)
	return nil
}

func (cp *ControlPlane) Handshake(data []byte) (*dataplane.PartContext, error) {
	nextPartId := cp.NextPartId()
	hs := NewPartsHandshake()

	// TODO: Await Answer
	hs.PartTransfers = make([]PartTransfer, 1)
	hs.PartTransfers[0] = PartTransfer{
		PartId:   uint64(nextPartId),
		PartSize: uint64(len(data)),
		Port:     0,
	}
	hs.LocalAddr = cp.Dataplane.LocalAddr().String()

	err := hs.Encode()
	if err != nil {
		return nil, err
	}

	cp.Dataplane.Write(hs.raw)

	// TODO: Check PartId
	retHs := NewPartsHandshake()
	cp.Dataplane.Read(retHs.raw)
	err = hs.Decode()
	if err != nil {
		return nil, err
	}

	partContext := dataplane.PartContext{
		PartsPacketPacker:         dataplane.NewBinaryPartsPacketPacker(),
		TransportPacketPacker:     dataplane.NewSCIONPacketPacker(),
		MaxPacketLength:           dataplane.PACKET_SIZE,
		PartId:                    int64(hs.PartTransfers[0].PartId),
		Data:                      data,
		MissingSequenceNums:       make([]int64, 0),
		MissingSequenceNumOffsets: make([]int64, 0),
		OnPartStatusChange: func(numMsg int, bytes int) {
			// log.Infof("Print on PartsConn %d", b.PartId)
			cp.Ratecontrol.Add(numMsg, int64(bytes))
		},
	}
	partContext.TransportPacketPacker.SetLocal(cp.Dataplane.LocalAddr())
	partContext.TransportPacketPacker.SetRemote(cp.Dataplane.RemoteAddr())
	partContext.Prepare()

	return &partContext, nil
}

func (cp *ControlPlane) AwaitHandshake(b []byte) (*dataplane.PartContext, error) {

	hs := NewPartsHandshake()
	cp.Dataplane.Read(hs.raw)
	err := hs.Decode()
	if err != nil {
		return nil, err
	}

	if cp.Dataplane.RemoteAddr() == nil {
		cp.Dataplane.SetRemoteAddr(hs.LocalAddr)
	}

	retHs := NewPartsHandshake()

	// TODO: Await Answer
	retHs.PartTransfers = make([]PartTransfer, 1)
	retHs.PartTransfers[0] = PartTransfer{
		PartId:   uint64(hs.PartTransfers[0].PartId),
		PartSize: uint64(hs.PartTransfers[0].PartSize),
		Port:     0,
	}
	retHs.LocalAddr = cp.Dataplane.LocalAddr().String()

	err = retHs.Encode()
	if err != nil {
		return nil, err
	}

	cp.Dataplane.Write(retHs.raw)

	partContext := dataplane.PartContext{
		PartsPacketPacker:         dataplane.NewBinaryPartsPacketPacker(),
		TransportPacketPacker:     dataplane.NewSCIONPacketPacker(),
		MaxPacketLength:           dataplane.PACKET_SIZE,
		PartId:                    int64(hs.PartTransfers[0].PartId),
		MissingSequenceNums:       make([]int64, 0),
		MissingSequenceNumOffsets: make([]int64, 0),
		Data:                      b,
		OnPartStatusChange: func(numMsg int, bytes int) {
			// log.Infof("Print on PartsConn %d", b.PartId)
			// cp.Ratecontrol.Add(numMsg, int64(bytes))
		},
	}

	partContext.TransportPacketPacker.SetLocal(cp.Dataplane.LocalAddr())
	partContext.TransportPacketPacker.SetRemote(cp.Dataplane.RemoteAddr())
	partContext.Prepare()

	return &partContext, nil
}

// Collecting retransfers
func (cp *ControlPlane) handlePartAckPacket(p *PartAckPacket) bool {
	// Part finished, can get to next part
	if p.PartFinished {
		cp.state = CP_STATE_PENDING
		cp.Dataplane.SetState(dataplane.DP_STATE_PENDING)
		return true
		// partsConn.mode = MODE_DONE
		// continue
	}

	cp.Ratecontrol.AddAckMessage(p)

	// log.Info(p)

	// log.Infof("Got PartRequestPacket with maxSequenceNumber %d, %d missingNums and partId %d", p.LastSequenceNumber, len(p.MissingSequenceNumbers), p.PartId)
	// TODO: Add to retransfers

	// partsConn.RateControl.AddAckMessage(p)
	for i, v := range p.MissingSequenceNumbers {
		// log.Infof("Add %d to missing sequenceNumbers for client to send them back later", v)
		// if b.lastPartRequestPacket != nil && utils.IndexOf(v, b.lastPartRequestPacket.MissingSequenceNumbers) >= 0 {
		// We continue here, because we want to avoid duplicate retransfers.
		// Might be improved later
		// }
		index := utils.IndexOf(v, cp.PartContext.MissingSequenceNums)
		if index < 0 {
			cp.Lock()
			cp.PartContext.MissingSequenceNums = append(cp.PartContext.MissingSequenceNums, v)
			cp.PartContext.MissingSequenceNumOffsets = append(cp.PartContext.MissingSequenceNumOffsets, p.MissingSequenceNumberOffsets[i])
			cp.Unlock()
		}

	}

	return false
}

func (cp *ControlPlane) sendPartFinish() error {
	var network bytes.Buffer        // Stand-in for a network connection
	enc := gob.NewEncoder(&network) // Will write to network.
	// log.Infof("Requesting from %d to %d having %d (%d), partId %d", start, min, len(partsConn.partContext.MissingSequenceNums), partsConn.partContext.MissingSequenceNums, partsConn.PartId)
	p := dataplane.PartRequestPacket{
		PartId:            cp.PartContext.PartId,
		NumPacketsPerTx:   0,
		PacketTxIndex:     0,
		TransactionId:     0,
		NumPackets:        int64(cp.PartContext.RecvPackets),
		RequestSequenceId: 0,
		PartFinished:      true,
	}
	err := enc.Encode(p)
	if err != nil {
		return err
	}
	_, err = cp.Dataplane.Write(network.Bytes())
	return err
}

func (cp *ControlPlane) parsePartAckPacket(packet []byte) (*PartAckPacket, error) {
	network := bytes.NewBuffer(packet)
	dec := gob.NewDecoder(network)
	p := PartAckPacket{}
	err := dec.Decode(&p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (cp *ControlPlane) requestRetransfers(stopChan *chan bool) {
	// TODO: After x packets, or timeout after x milliseconds
	ticker := time.NewTicker(10 * time.Millisecond)
	done := make(chan bool)
	// log.Infof("In Call of requestRetransfers %p", missingNums)
	// log.Infof("In Call of requestRetransfers go routine %p", missingNums)
	var txId int64 = 1
	requestSequenceId := 1
	for {
		select {
		case <-done:
			return
		case <-*stopChan:
			return
		case <-ticker.C:
			if cp.PartContext == nil {
				continue
			}

			// This happens only in receiving state
			// if cp.state != CP_STATE_RECEIVING {
			//	continue
			// }

			// If we have no packets received, we dont need to request retransfers
			if cp.PartContext.HighestSequenceNumber < 1 {
				continue
			}

			missingNumsPerPacket := 100
			missingNumIndex := 0
			// log.Infof("Having %d missing Sequence Numbers for con index %d", len(cp.PartContext.MissingSequenceNums), 0)
			start := 0
			index := 0
			for len(cp.PartContext.MissingSequenceNums) == 0 || missingNumIndex < len(cp.PartContext.MissingSequenceNums) {
				min := utils.Min(start+missingNumsPerPacket, len(cp.PartContext.MissingSequenceNums))
				var network bytes.Buffer        // Stand-in for a network connection
				enc := gob.NewEncoder(&network) // Will write to network.
				// log.Infof("Requesting from %d to %d having %d (%d), partId %d", start, min, len(partsConn.partContext.MissingSequenceNums), partsConn.partContext.MissingSequenceNums, partsConn.PartId)
				p := dataplane.PartRequestPacket{
					PartId:                       cp.PartContext.PartId,
					NumPacketsPerTx:              utils.CeilForceInt(len(cp.PartContext.MissingSequenceNums), missingNumsPerPacket),
					PacketTxIndex:                index,
					LastSequenceNumber:           cp.PartContext.HighestSequenceNumber,
					MissingSequenceNumbers:       (cp.PartContext.MissingSequenceNums)[start:min],
					MissingSequenceNumberOffsets: (cp.PartContext.MissingSequenceNumOffsets)[start:min],
					TransactionId:                txId,
					NumPackets:                   int64(cp.PartContext.RecvPackets),
					RequestSequenceId:            requestSequenceId,
				}
				// log.Info("PACKET")
				// log.Info(p)
				index++
				err := enc.Encode(p)
				if err != nil {
					log.Error("encode error:", err)
				}
				_, err = cp.Dataplane.Write(network.Bytes())
				// TODO: Remove this!
				time.Sleep(1 * time.Millisecond)
				missingNumIndex += min
				start += min

				if min == 0 {
					break
				}
			}
			requestSequenceId++
		}
	}
}

/*
// TBD: For Bittorrent later, maybe share control plane between socket instances
func (cp *ControlPlane) AddControlPeer(remoteCtrlPort int, remoteCtrlAddr *net.UDPAddr) {
	cp.Peers = append(cp.Peers, ControlPeer{remoteCtrlPort: remoteCtrlPort, remoteCtrlAddr: remoteCtrlAddr})
}

// TBD: Extract to ReadRetransfers, etc
func (cp *ControlPlane) Read(buf []byte) (int, error) {
	return cp.ControlSocket.Read(buf)
}

// TBD: Extract to WriteRetransfers, etc
func (cp *ControlPlane) Write(buf []byte) (int, error) {
	return cp.ControlSocket.Write(buf)
}*/
