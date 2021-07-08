package api

import (
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"fmt"
	"sync"
	"time"

	"github.com/martenwallewein/parts/control"
	"github.com/martenwallewein/parts/partmetrics"
	"github.com/martenwallewein/parts/socket"
	log "github.com/sirupsen/logrus"
)

type PartsConn struct {
	sync.Mutex
	part                       []byte
	PartId                     int64
	localAddr                  string
	remoteAddr                 string
	localStartPort             int
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
	transportSocketConstructor socket.TransportSocketConstructor
	transportPackerConstructor socket.TransportPackerConstructor
	MaxSpeed                   int64
}

func NewPartsConn(localAddr, remoteAddr string, localStartPort, remoteStartPort int,
	controlPlane *control.ControlPlane,
	transportSocketConstructor socket.TransportSocketConstructor,
	transportPackerConstructor socket.TransportPackerConstructor,
) *PartsConn {

	partsConn := &PartsConn{
		missingSequenceNums:        make([]int64, 0),
		localAddr:                  localAddr,
		remoteAddr:                 remoteAddr,
		localStartPort:             localStartPort,
		remoteStartPort:            remoteStartPort,
		ControlPlane:               controlPlane,
		transportSocketConstructor: transportSocketConstructor,
		transportPackerConstructor: transportPackerConstructor,
	}

	partsConn.TransportSocket = partsConn.transportSocketConstructor()

	return partsConn
}

func (b *PartsConn) SetMaxSpeed(maxSpeed int64) {
	b.MaxSpeed = maxSpeed
}

func (b *PartsConn) WritePart(part []byte, partId int64) {
	// TODO: Save activePartCount and increase immediatly
	// TODO: Not overwrite if actually sending

	rc := control.NewRateControl(
		100,
		b.MaxSpeed,
		PACKET_SIZE,
	)

	partContext := socket.PartContext{
		PartsPacketPacker:     socket.NewBinaryPartsPacketPacker(),
		TransportPacketPacker: b.transportPackerConstructor(),
		MaxPacketLength:       PACKET_SIZE,
		PartId:                partId,
		Data:                  part,
		OnPartStatusChange: func(numMsg int, bytes int) {
			if !b.TestingMode {
				rc.Add(numMsg, int64(bytes))
			}
		},
		TestingMode: b.TestingMode,
	}
	partContext.TransportPacketPacker.SetLocal(b.localAddr, b.localStartPort)
	partContext.TransportPacketPacker.SetRemote(b.remoteAddr, b.remoteStartPort)
	b.partContext = &partContext
	b.TransportSocket.Listen(b.localAddr, b.localStartPort)
	b.TransportSocket.Dial(b.remoteAddr, b.remoteStartPort)
	partContext.Prepare()
	log.Infof("Writing %d packets", partContext.NumPackets)
	// b.packets[b.activePartCount] = make([][]byte, len(part)/PACKET_SIZE)
	// TODO: Waiting queue
	// TODO: sync write calls
	b.mode = MODE_SENDING
	rc.Start()
	b.TransportSocket.WritePart(&partContext)
	log.Infof("Wrote %d packets, partLen %d", partContext.NumPackets, len(part))
	// log.Info(part[len(part)-1000:])
	b.mode = MODE_RETRANSFER
	b.retransferMissingPackets()
	time.Sleep(100 * time.Second)
}

func (b *PartsConn) ReadPart(part []byte, partId int64) {
	// TODO: Not overwrite if actually receiving
	b.part = part
	b.PartId = partId
	partContext := socket.PartContext{
		PartsPacketPacker:     socket.NewBinaryPartsPacketPacker(),
		TransportPacketPacker: b.transportPackerConstructor(),
		MaxPacketLength:       PACKET_SIZE,
		PartId:                partId,
		Data:                  part,
		OnPartStatusChange:    func(numMsg int, bytes int) {},
		TestingMode:           b.TestingMode,
	}
	partContext.TransportPacketPacker.SetLocal(b.localAddr, b.localStartPort)
	partContext.TransportPacketPacker.SetRemote(b.remoteAddr, b.remoteStartPort)
	b.partContext = &partContext
	partContext.Prepare()
	err := b.TransportSocket.Listen(b.localAddr, b.localStartPort)
	if err != nil {
		log.Infof("Failed to listen on %s", fmt.Sprintf("%s:%d", b.localAddr, b.localStartPort))
		log.Fatal(err)
	}
	b.TransportSocket.Dial(b.remoteAddr, b.remoteStartPort)
	// TODO: This assumption here is bullshit, we need to read part size from first packet of part id
	// TODO: How to ensure order of parallel sent parts? Increasing partIds?
	log.Infof("Receiving %d packets", partContext.NumPackets)
	// b.requestRetransfers()
	// TODO: Waiting queue
	// TODO: sync write calls
	// TODO: Can not put this into struct for whatever reason

	b.TransportSocket.ReadPart(&partContext)
	log.Infof("Received %d packets, partLen %d and md5 %x", partContext.NumPackets, len(part), md5.Sum(part))
	// log.Info(part[len(part)-1000:])
	// copy(part, partContext.Data)
}

func (b *PartsConn) retransferMissingPackets() {
	log.Infof("Entering retransfer")
	for b.mode == MODE_RETRANSFER {
		// log.Infof("Having %d missing sequenceNums with addr %p", len(b.partContext.MissingSequenceNums), &b.partContext.MissingSequenceNums)
		for i, v := range b.partContext.MissingSequenceNums {
			if v == 0 {
				log.Fatal("error 0 sequenceNumber")
			}

			off := int(b.partContext.MissingSequenceNumOffsets[i])
			// log.Infof("Sending back %d with offset %d", v-1, off)
			for j := 0; j < off; j++ {
				// packet := b.packets[v-1]

				// TODO: How to get already cached packet here, otherwise at least payload
				packet := b.partContext.GetPayloadByPacketIndex(int(v) + j - 1)
				buf := make([]byte, len(packet)+b.partContext.PartsPacketPacker.GetHeaderLen())
				copy(buf[b.partContext.PartsPacketPacker.GetHeaderLen():], packet)
				// log.Infof("Retransferring md5 %x for sequenceNumber %d", md5.Sum(packet), v)
				b.partContext.SerializeRetransferPacket(&buf, v)
				// log.Infof("Retransfer sequenceNum %d", v)
				bts, err := (b.TransportSocket).Write(buf)
				b.Metrics.TxBytes += uint64(bts)
				b.Metrics.TxPackets += 1
				time.Sleep(1 * time.Microsecond)
				if err != nil {
					log.Fatal("error:", err)
				}
			}

		}
		b.partContext.Lock()
		b.partContext.MissingSequenceNums = make([]int64, 0)
		// log.Infof("Resetting retransfers")
		b.partContext.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
}

/*func (b *PartsConn) collectRetransfers(ctrlCon *net.UDPConn, missingNums *[]int64) {
	/*go func() {
		b.retransferMissingPackets()
	}()
	for {
		buf := make([]byte, PACKET_SIZE+100)
		bts, err := b.ControlPlane.Read(buf)
		if err != nil {
			log.Fatal("error:", err)
		}

		log.Infof("Received %d ctrl bytes", bts)
		var p socket.PartRequestPacket
		// TODO: Fix
		decodeReqPacket(&p, buf)
		if err != nil {
			log.Fatal("encode error:", err)
		}

		log.Infof("Got PartRequestPacket with maxSequenceNumber %d and partId", p.LastSequenceNumber, p.PartId)
		// TODO: Add to retransfers
		for _, v := range p.MissingSequenceNumbers {
			// log.Infof("Add %d to missing sequenceNumbers for client to send them back later", v)
			b.partContext.Lock()
			b.partContext.MissingSequenceNums = utils.AppendIfMissing(b.partContext.MissingSequenceNums, v)
			b.partContext.Unlock()
		}
		// b.missingSequenceNums[index] = append(b.missingSequenceNums[index], p.MissingSequenceNumbers...)
		// log.Infof("Added %d sequenceNumbers to missingSequenceNumbers", len(p.MissingSequenceNumbers))
	}

}
*/
/*
func (b *PartsConn) requestRetransfers() {
	ticker := time.NewTicker(1000 * time.Millisecond)
	done := make(chan bool)
	log.Infof("In Call of requestRetransfers %p", b.partContext.MissingSequenceNums)
	go func(missingNums *[]int64) {
		log.Infof("In Call of requestRetransfers go routine %p", missingNums)
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				missingNumsPerPacket := 100
				missingNumIndex := 0
				start := 0
				// for _, v := range *missingNums {
				//	log.Infof("Having missing SequenceNums %v", v)
				//}

				for missingNumIndex < len(*missingNums) {
					min := utils.Min(start+missingNumsPerPacket, len(*missingNums))
					var network bytes.Buffer        // Stand-in for a network connection
					enc := gob.NewEncoder(&network) // Will write to network.
					p := socket.PartRequestPacket{
						LastSequenceNumber:     b.partContext.HighestSequenceNumber,
						PartId:                b.PartId,
						MissingSequenceNumbers: (*missingNums)[start:min],
					}

					err := enc.Encode(p)
					if err != nil {
						log.Fatal("encode error:", err)
					}
					/*for _, v := range p.MissingSequenceNumbers {
						log.Infof("Sending missing SequenceNums %v", v)
					}
					_, err = b.ControlPlane.Write(network.Bytes())
					if err != nil {
						log.Fatal("Write error:", err)
					}

					// log.Infof("Wrote %d ctrl bytes to client", bts)
					missingNumIndex += min
				}

			}
		}
	}(&b.partContext.MissingSequenceNums)

}*/

func decodeReqPacket(p *socket.PartRequestPacket, buf []byte) error {
	network := bytes.NewBuffer(buf)
	dec := gob.NewDecoder(network)
	return dec.Decode(&p)
}
