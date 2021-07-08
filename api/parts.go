package api

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/martenwallewein/parts/control"
	"github.com/martenwallewein/parts/partmetrics"
	"github.com/martenwallewein/parts/socket"
	"github.com/martenwallewein/parts/utils"
	log "github.com/sirupsen/logrus"
)

const (
	NUM_ACTIVE_BLOCKS  = 1
	BUF_SIZE           = 1024 * 1024
	PACKET_SIZE        = 1400
	BLOCKS_HEADER_SIZE = 16 // 95 // TODO: Fix this magic number
	MODE_SENDING       = 0
	MODE_RETRANSFER    = 1
	MODE_RECEIVING     = 2
	MODE_DONE          = 3
)

type PartsSock struct {
	sync.Mutex
	activePartCount            int
	localAddr                  string
	remoteAddr                 string
	localStartPort             int
	remoteStartPort            int
	udpCons                    []net.Conn
	ctrlConn                   *net.UDPConn
	modes                      []int
	partConns                  []*PartsConn
	controlPlane               *control.ControlPlane
	acivePartIndex             int
	Metrics                    *partmetrics.Metrics
	lastPartRequestPacket      *socket.PartRequestPacket
	testingMode                bool
	transportSocketConstructor socket.TransportSocketConstructor
	transportPackerConstructor socket.TransportPackerConstructor
	MaxSpeed                   int64
	NumCons                    int
}

func NewPartsSock(
	localAddr, remoteAddr string,
	localStartPort, remoteStartPort, localCtrlPort, remoteCtrlPort int,
	numCons int,
	transportSocketConstructor socket.TransportSocketConstructor,
	transportPackerConstructor socket.TransportPackerConstructor,
) *PartsSock {
	partSock := &PartsSock{
		localAddr:                  localAddr,
		remoteAddr:                 remoteAddr,
		localStartPort:             localStartPort,
		remoteStartPort:            remoteStartPort,
		modes:                      make([]int, numCons),
		partConns:                  make([]*PartsConn, 0),
		acivePartIndex:             0,
		NumCons:                    numCons,
		transportSocketConstructor: transportSocketConstructor,
		transportPackerConstructor: transportPackerConstructor,
	}
	var err error
	partSock.controlPlane, err = control.NewControlPlane(
		localCtrlPort, remoteCtrlPort, localAddr, remoteAddr,
		transportSocketConstructor,
		transportPackerConstructor,
	)

	if err != nil {
		log.Fatal(err)
	}

	for i := range partSock.modes {
		conn := NewPartsConn(
			localAddr, remoteAddr, localStartPort+i, remoteStartPort+i,
			partSock.controlPlane,
			transportSocketConstructor,
			transportPackerConstructor,
		)
		partSock.partConns = append(partSock.partConns, conn)

	}

	partSock.Metrics = partmetrics.NewMetrics(1000, numCons, func(index int) (uint64, uint64, uint64, uint64) {
		rxBytes := partSock.partConns[index].Metrics.RxBytes
		txBytes := partSock.partConns[index].Metrics.TxBytes
		rxPackets := partSock.partConns[index].Metrics.RxPackets
		txPackets := partSock.partConns[index].Metrics.TxPackets

		return rxBytes, txBytes, rxPackets, txPackets
	})

	// gob.Register(PartPacket{})

	return partSock
}
func (b *PartsSock) SetMaxSpeed(maxSpeed int64) {
	b.MaxSpeed = maxSpeed
	for _, v := range b.partConns {
		v.MaxSpeed = maxSpeed
	}
}

func (b *PartsSock) EnableTestingMode() {
	b.testingMode = true
	for _, v := range b.partConns {
		v.TestingMode = true
	}
}

func (b *PartsSock) Dial() {
	/*if b.ctrlConn == nil {

		raddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.remoteAddr, b.remoteCtrlPort))
		if err != nil {
			log.Fatal("error:", err)
		}
		laddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.localAddr, b.localCtrlPort))
		if err != nil {
			log.Fatal("error:", err)
		}
		b.localCtrlAddr = laddr
		b.remoteCtrlAddr = raddr
		b.ctrlConn, err = net.ListenUDP("udp", laddr)
		if err != nil {
			log.Fatal("error:", err)
		}
		log.Infof("Dial Ctrl from %s to %s", laddr.String(), raddr.String())
		fmt.Println(b.ctrlConn)

		for i := range b.modes {
			b.partConns[i].ctrlConn = b.ctrlConn
			b.partConns[i].remoteCtrlAddr = b.remoteCtrlAddr
			b.partConns[i].localCtrlAddr = b.localCtrlAddr
		}

		go b.collectRetransfers()
	}*/
	go b.collectRetransfers()
}

func (b *PartsSock) Listen() {
	/*if b.ctrlConn == nil {

		laddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.localAddr, b.localCtrlPort))
		if err != nil {
			log.Fatal("error:", err)
		}

		raddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.remoteAddr, b.remoteCtrlPort))
		if err != nil {
			log.Fatal("error:", err)
		}

		b.localCtrlAddr = laddr
		b.remoteCtrlAddr = raddr

		ctrlConn, err := net.ListenUDP("udp", laddr)
		fmt.Printf("Listen Ctrl on %s \n", laddr.String())
		if err != nil {
			log.Fatal("error:", err)
		}
		b.ctrlConn = ctrlConn
		for i := range b.modes {
			b.partConns[i].ctrlConn = b.ctrlConn
			b.partConns[i].remoteCtrlAddr = b.remoteCtrlAddr
			b.partConns[i].localCtrlAddr = b.localCtrlAddr
		}

		go b.requestRetransfers()

	}*/
	go b.requestRetransfers()
}

func (b *PartsSock) WritePart(part []byte) {
	bLen := len(part)
	// TODO: Ensure waiting
	//go func(index int) {
	//	fmt.Printf("WritePart for index %d\n", index%NUM_BUFS)
	//		b.partConns[index%NUM_BUFS].WritePart(part[halfLen:], int64(index+1)) // PartIds positive
	//	}(b.acivePartIndex)
	//	b.acivePartIndex++
	var wg sync.WaitGroup
	partLen := (bLen / b.NumCons)
	for i := 0; i < b.NumCons; i++ {
		wg.Add(1)
		go func(index int, wg *sync.WaitGroup) {
			fmt.Printf("WritePart for index %d\n", index%b.NumCons)
			start := partLen * index
			end := utils.Min(start+partLen, bLen)
			b.partConns[index%b.NumCons].WritePart(part[start:end], int64(index+1)) // PartIds positive
			wg.Done()
		}(b.acivePartIndex, &wg)
		b.acivePartIndex++
	}

	wg.Wait()
	// fmt.Printf("WritePart for index %d\n", b.acivePartIndex%NUM_BUFS)
	// b.partConns[b.acivePartIndex%NUM_BUFS].WritePart(part[:halfLen], int64(b.acivePartIndex+1)) // PartIds positive
	// b.acivePartIndex++
}

func (b *PartsSock) ReadPart(part []byte) {
	bLen := len(part)
	var wg sync.WaitGroup
	partLen := (bLen / b.NumCons)
	for i := 0; i < b.NumCons; i++ {
		wg.Add(1)
		go func(index int, wg *sync.WaitGroup) {
			fmt.Printf("EadPart for index %d\n", index%b.NumCons)
			start := partLen * index
			end := utils.Min(start+partLen, bLen)
			log.Infof("Receiving for %d partLen, start %d, %d partLen and end %d", len(part[start:end]), start, partLen, end)
			b.partConns[index%b.NumCons].ReadPart(part[start:end], int64(index+1)) // PartIds positive
			wg.Done()
		}(b.acivePartIndex, &wg)
		b.acivePartIndex++
	}

	wg.Wait()

}

func (b *PartsSock) collectRetransfers() {
	/*go func() {
		b.retransferMissingPackets()
	}()*/
	for {
		buf := make([]byte, PACKET_SIZE+100)
		_, err := b.controlPlane.Read(buf)
		if err != nil {
			log.Fatal("error:", err)
		}

		// log.Infof("Received %d ctrl bytes", bts)
		var p socket.PartRequestPacket
		// TODO: Fix
		decodeReqPacket(&p, buf)
		if err != nil {
			log.Fatal("encode error:", err)
		}

		// log.Info(p)

		// log.Infof("Got PartRequestPacket with maxSequenceNumber %d, %d missingNums and partId %d", p.LastSequenceNumber, len(p.MissingSequenceNumbers), p.PartId)
		// TODO: Add to retransfers
		partsConn := b.partConns[(p.PartId-1)%int64(b.NumCons)]
		for i, v := range p.MissingSequenceNumbers {
			// log.Infof("Add %d to missing sequenceNumbers for client to send them back later", v)
			if b.lastPartRequestPacket != nil && utils.IndexOf(v, b.lastPartRequestPacket.MissingSequenceNumbers) >= 0 {
				// We continue here, because we want to avoid duplicate retransfers.
				// Might be improved later
			}
			index := utils.IndexOf(v, partsConn.partContext.MissingSequenceNums)
			if index < 0 {
				partsConn.Lock()
				partsConn.partContext.MissingSequenceNums = append(partsConn.partContext.MissingSequenceNums, v)
				partsConn.partContext.MissingSequenceNumOffsets = append(partsConn.partContext.MissingSequenceNumOffsets, p.MissingSequenceNumberOffsets[i])
				partsConn.Unlock()
			}

		}
		b.lastPartRequestPacket = &p
		// b.missingSequenceNums[index] = append(b.missingSequenceNums[index], p.MissingSequenceNumbers...)
		// log.Infof("Added %d sequenceNumbers to missingSequenceNumbers", len(p.MissingSequenceNumbers))
	}

}

func (b *PartsSock) requestRetransfers() {
	ticker := time.NewTicker(1000 * time.Millisecond)
	done := make(chan bool)
	// log.Infof("In Call of requestRetransfers %p", missingNums)
	// log.Infof("In Call of requestRetransfers go routine %p", missingNums)
	var txId int64 = 1
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			for _, partsConn := range b.partConns {
				if partsConn.partContext == nil {
					continue
				}
				missingNumsPerPacket := 200
				missingNumIndex := 0
				// log.Infof("Having %d missing Sequence Numbers for con index %d", len(partsConn.partContext.MissingSequenceNums), i)
				start := 0

				for missingNumIndex < len(partsConn.partContext.MissingSequenceNums) {
					min := utils.Min(start+missingNumsPerPacket, len(partsConn.partContext.MissingSequenceNums))
					var network bytes.Buffer        // Stand-in for a network connection
					enc := gob.NewEncoder(&network) // Will write to network.
					log.Infof("Requesting from %d to %d having %d (%d)", start, min, len(partsConn.partContext.MissingSequenceNums), partsConn.partContext.MissingSequenceNums)
					p := socket.PartRequestPacket{
						PartId:                       partsConn.PartId,
						LastSequenceNumber:           partsConn.partContext.HighestSequenceNumber,
						MissingSequenceNumbers:       (partsConn.partContext.MissingSequenceNums)[start:min],
						MissingSequenceNumberOffsets: (partsConn.partContext.MissingSequenceNumOffsets)[start:min],
						TransactionId:                txId,
					}
					// log.Info("PACKET")
					// log.Info(p)

					err := enc.Encode(p)
					if err != nil {
						log.Fatal("encode error:", err)
					}
					/*for _, v := range p.MissingSequenceNumbers {
						log.Infof("Sending missing SequenceNums %v", v)
					}*/
					// log.Infof("Sending missing %d SequenceNums", len(p.MissingSequenceNumbers))
					_, err = b.controlPlane.Write(network.Bytes())
					time.Sleep(100 * time.Millisecond)
					// _, err = (b.ctrlConn).WriteTo(network.Bytes(), b.remoteCtrlAddr)
					// if err != nil {
					//	log.Fatal("Write error:", err)
					//}

					// log.Infof("Wrote %d ctrl bytes to client", bts)
					missingNumIndex += min
					start += min
				}
			}

		}
	}

}
