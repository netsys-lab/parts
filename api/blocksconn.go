package api

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"
	"time"

	"github.com/martenwallewein/blocks/blockmetrics"
	"github.com/martenwallewein/blocks/control"
	"github.com/martenwallewein/blocks/socket"
	log "github.com/sirupsen/logrus"
)

type BlocksConn struct {
	sync.Mutex
	block                       []byte
	BlockId                     int64
	localAddr                   string
	remoteAddr                  string
	localStartPort              int
	remoteStartPort             int
	lastRequestedSequenceIndex  []int64
	retransferPackets           [][]byte
	missingSequenceNums         []int64
	mode                        int
	Metrics                     blockmetrics.SocketMetrics
	TransportSocket             socket.TransportSocket
	ControlPlane                *control.ControlPlane
	blockContext                *socket.BlockContext
	TestingMode                 bool
	transportSocketContstructor TransportSocketContstructor
	transportPackerConstructor  TransportPackerContstructor
}

func NewBlocksConn(localAddr, remoteAddr string, localStartPort, remoteStartPort int, controlPlane *control.ControlPlane) *BlocksConn {

	blocksConn := &BlocksConn{
		missingSequenceNums:         make([]int64, 0),
		localAddr:                   localAddr,
		remoteAddr:                  remoteAddr,
		localStartPort:              localStartPort,
		remoteStartPort:             remoteStartPort,
		ControlPlane:                controlPlane,
		transportSocketContstructor: func() socket.TransportSocket { return socket.NewUDPTransportSocket() },
		transportPackerConstructor:  func() socket.TransportPacketPacker { return socket.NewUDPTransportPacketPacker() },
	}

	blocksConn.TransportSocket = blocksConn.transportSocketContstructor()

	return blocksConn
}

func (b *BlocksConn) SetTransportSocketConstructor(cons TransportSocketContstructor) {
	b.transportSocketContstructor = cons
}

func (b *BlocksConn) SetTransportPackerConstructor(cons TransportPackerContstructor) {
	b.transportPackerConstructor = cons
}

func (b *BlocksConn) WriteBlock(block []byte, blockId int64) {
	// TODO: Save activeBlockCount and increase immediatly
	// TODO: Not overwrite if actually sending

	rc := control.NewRateControl(
		100,
		2000000000,
		PACKET_SIZE,
	)

	blockContext := socket.BlockContext{
		BlocksPacketPacker:    socket.NewBinaryBlocksPacketPacker(),
		TransportPacketPacker: b.transportPackerConstructor(),
		MaxPacketLength:       PACKET_SIZE,
		BlockId:               blockId,
		Data:                  block,
		OnBlockStatusChange: func(numMsg int, bytes int) {
			if !b.TestingMode {
				rc.Add(numMsg, int64(bytes))
			}
		},
		TestingMode: b.TestingMode,
	}
	b.blockContext = &blockContext
	blockContext.Prepare()
	b.TransportSocket.Listen(fmt.Sprintf("%s:%d", b.localAddr, b.localStartPort))
	b.TransportSocket.Dial(fmt.Sprintf("%s:%d", b.remoteAddr, b.remoteStartPort))

	log.Infof("Writing %d packets", blockContext.NumPackets)
	// b.packets[b.activeBlockCount] = make([][]byte, len(block)/PACKET_SIZE)
	// TODO: Waiting queue
	// TODO: sync write calls
	b.mode = MODE_SENDING
	rc.Start()
	b.TransportSocket.WriteBlock(&blockContext)
	log.Infof("Wrote %d packets, blockLen %d", blockContext.NumPackets, len(block))
	// log.Info(block[len(block)-1000:])
	b.mode = MODE_RETRANSFER
	b.retransferMissingPackets()
	time.Sleep(100 * time.Second)
}

func (b *BlocksConn) ReadBlock(block []byte, blockId int64) {
	// TODO: Not overwrite if actually receiving
	b.block = block
	b.BlockId = blockId
	blockContext := socket.BlockContext{
		BlocksPacketPacker:    socket.NewBinaryBlocksPacketPacker(),
		TransportPacketPacker: b.transportPackerConstructor(),
		MaxPacketLength:       PACKET_SIZE,
		BlockId:               blockId,
		Data:                  block,
		OnBlockStatusChange:   func(numMsg int, bytes int) {},
		TestingMode:           b.TestingMode,
	}
	b.blockContext = &blockContext
	blockContext.Prepare()
	err := b.TransportSocket.Listen(fmt.Sprintf("%s:%d", b.localAddr, b.localStartPort))
	if err != nil {
		log.Infof("Failed to listen on %s", fmt.Sprintf("%s:%d", b.localAddr, b.localStartPort))
		log.Fatal(err)
	}
	b.TransportSocket.Dial(fmt.Sprintf("%s:%d", b.remoteAddr, b.remoteStartPort))
	// TODO: This assumption here is bullshit, we need to read block size from first packet of block id
	// TODO: How to ensure order of parallel sent blocks? Increasing blockIds?
	log.Infof("Receiving %d packets", blockContext.NumPackets)
	// b.requestRetransfers()
	// TODO: Waiting queue
	// TODO: sync write calls
	// TODO: Can not put this into struct for whatever reason

	b.TransportSocket.ReadBlock(&blockContext)
	log.Infof("Received %d packets, blockLen %d", blockContext.NumPackets, len(block))
	// log.Info(block[len(block)-1000:])
	// copy(block, blockContext.Data)
}

func (b *BlocksConn) retransferMissingPackets() {
	log.Infof("Entering retransfer")
	for b.mode == MODE_RETRANSFER {
		// log.Infof("Having %d missing sequenceNums with addr %p", len(b.blockContext.MissingSequenceNums), &b.blockContext.MissingSequenceNums)
		for i, v := range b.blockContext.MissingSequenceNums {
			if v == 0 {
				log.Fatal("error 0 sequenceNumber")
			}

			off := int(b.blockContext.MissingSequenceNumOffsets[i])
			// log.Infof("Sending back %d with offset %d", v-1, off)
			for j := 0; j < off; j++ {
				// packet := b.packets[v-1]

				// TODO: How to get already cached packet here, otherwise at least payload
				packet := b.blockContext.GetPayloadByPacketIndex(int(v) + j - 1)
				buf := make([]byte, len(packet)+b.blockContext.HeaderLength)
				copy(buf[b.blockContext.HeaderLength:], packet)
				// log.Infof("Retransferring md5 %x for sequenceNumber %d", md5.Sum(packet), v)
				b.blockContext.SerializeRetransferPacket(&buf, v)
				// log.Infof("Retransfer sequenceNum %d", v)
				bts, err := (b.TransportSocket).Write(buf)
				b.Metrics.TxBytes += uint64(bts)
				b.Metrics.TxPackets += 1
				// time.Sleep(1 * time.Microsecond)
				if err != nil {
					log.Fatal("error:", err)
				}
			}

		}
		b.blockContext.Lock()
		b.blockContext.MissingSequenceNums = make([]int64, 0)
		// log.Infof("Resetting retransfers")
		b.blockContext.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
}

/*func (b *BlocksConn) collectRetransfers(ctrlCon *net.UDPConn, missingNums *[]int64) {
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
		var p socket.BlockRequestPacket
		// TODO: Fix
		decodeReqPacket(&p, buf)
		if err != nil {
			log.Fatal("encode error:", err)
		}

		log.Infof("Got BlockRequestPacket with maxSequenceNumber %d and blockId", p.LastSequenceNumber, p.BlockId)
		// TODO: Add to retransfers
		for _, v := range p.MissingSequenceNumbers {
			// log.Infof("Add %d to missing sequenceNumbers for client to send them back later", v)
			b.blockContext.Lock()
			b.blockContext.MissingSequenceNums = utils.AppendIfMissing(b.blockContext.MissingSequenceNums, v)
			b.blockContext.Unlock()
		}
		// b.missingSequenceNums[index] = append(b.missingSequenceNums[index], p.MissingSequenceNumbers...)
		// log.Infof("Added %d sequenceNumbers to missingSequenceNumbers", len(p.MissingSequenceNumbers))
	}

}
*/
/*
func (b *BlocksConn) requestRetransfers() {
	ticker := time.NewTicker(1000 * time.Millisecond)
	done := make(chan bool)
	log.Infof("In Call of requestRetransfers %p", b.blockContext.MissingSequenceNums)
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
					p := socket.BlockRequestPacket{
						LastSequenceNumber:     b.blockContext.HighestSequenceNumber,
						BlockId:                b.BlockId,
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
	}(&b.blockContext.MissingSequenceNums)

}*/

func decodeReqPacket(p *socket.BlockRequestPacket, buf []byte) error {
	network := bytes.NewBuffer(buf)
	dec := gob.NewDecoder(network)
	return dec.Decode(&p)
}
