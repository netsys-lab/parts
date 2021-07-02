package main

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/martenwallewein/blocks/blockmetrics"
	"github.com/martenwallewein/blocks/socket"
	log "github.com/sirupsen/logrus"
)

type BlocksConn struct {
	sync.Mutex
	block                      []byte
	BlockId                    int64
	localAddr                  string
	remoteAddr                 string
	localStartPort             int
	remoteStartPort            int
	lastRequestedSequenceIndex []int64
	retransferPackets          [][]byte
	missingSequenceNums        []int64
	mode                       int
	Metrics                    blockmetrics.SocketMetrics
	TransportSocket            socket.TransportSocket
}

func NewBlocksConn(localAddr, remoteAddr string, localStartPort, remoteStartPort int, ctrlConn *net.UDPConn) *BlocksConn {

	blocksConn := &BlocksConn{
		missingSequenceNums: make([]int64, 0),
		localAddr:           localAddr,
		remoteAddr:          remoteAddr,
		localStartPort:      localStartPort,
		remoteStartPort:     remoteStartPort,
	}

	blocksConn.TransportSocket = socket.NewUDPTransportSocket()

	return blocksConn
}

func (b *BlocksConn) retransferMissingPackets(missingNums *[]int64) {
	/*ticker := time.NewTicker(1000 * time.Millisecond)
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				bandwidth := (b.receivedBytes - b.lastReceivedBytes) * 8 / 1024 / 1024
				log.Infof("Retransfer with %d Mbit/s having %d received packets", bandwidth, b.receivedPackets)
				b.lastReceivedBytes = b.receivedBytes
			}
		}
	}()
	log.Infof("Entering retransfer")
	for b.mode == MODE_RETRANSFER {
		for _, v := range *missingNums {
			if v == 0 {
				log.Fatal("error 0 sequenceNumber")
			}
			// packet := b.packets[v-1]
			// log.Infof("Sending back %d", v-1)
			// TODO: How to get packet here, or at least payload
			bts, err := (*&b.TransportSocket).Write([]byte{})
			b.Metrics.TxBytes += uint64(bts)
			b.Metrics.TxPackets += 1
			// time.Sleep(1 * time.Microsecond)
			if err != nil {
				log.Fatal("error:", err)
			}
		}
		b.Lock()
		*missingNums = make([]int64, 0)
		b.Unlock()
		time.Sleep(10 * time.Microsecond)
	}*/
}

func (b *BlocksConn) WriteBlock(block []byte, blockId int64) {
	// TODO: Save activeBlockCount and increase immediatly
	// TODO: Not overwrite if actually sending
	blockContext := socket.BlockContext{
		BlocksPacketPacker:    socket.NewBinaryBlocksPacketPacker(),
		TransportPacketPacker: socket.NewUDPTransportPacketPacker(),
		MaxPacketLength:       PACKET_SIZE,
		BlockId:               blockId,
		Data:                  block,
	}

	blockContext.Prepare()
	b.TransportSocket.Listen(fmt.Sprintf("%s:%d", b.localAddr, b.localStartPort))
	b.TransportSocket.Dial(fmt.Sprintf("%s:%d", b.remoteAddr, b.remoteStartPort))

	log.Infof("Writing %d packets", blockContext.NumPackets)
	// b.packets[b.activeBlockCount] = make([][]byte, len(block)/PACKET_SIZE)
	// TODO: Waiting queue
	// TODO: sync write calls
	b.mode = MODE_SENDING
	b.TransportSocket.WriteBlock(&blockContext)
	b.mode = MODE_RETRANSFER
	b.retransferMissingPackets(&b.missingSequenceNums)
	time.Sleep(100 * time.Second)
}

func (b *BlocksConn) ReadBlock(block []byte, blockId int64) {
	// TODO: Not overwrite if actually receiving
	b.block = block
	b.BlockId = blockId
	blockContext := socket.BlockContext{
		BlocksPacketPacker:    socket.NewBinaryBlocksPacketPacker(),
		TransportPacketPacker: socket.NewUDPTransportPacketPacker(),
		MaxPacketLength:       PACKET_SIZE,
		BlockId:               blockId,
		Data:                  block,
	}
	blockContext.Prepare()
	b.TransportSocket.Listen(fmt.Sprintf("%s:%d", b.localAddr, b.localStartPort))
	b.TransportSocket.Dial(fmt.Sprintf("%s:%d", b.remoteAddr, b.remoteStartPort))
	// TODO: This assumption here is bullshit, we need to read block size from first packet of block id
	// TODO: How to ensure order of parallel sent blocks? Increasing blockIds?
	log.Infof("Receiving %d packets", blockContext.NumPackets)
	// TODO: Waiting queue
	// TODO: sync write calls
	// TODO: Can not put this into struct for whatever reason
	blockContext.Data = block
	b.TransportSocket.ReadBlock(&blockContext)

}

func (b *BlocksConn) collectRetransfers(ctrlCon *net.UDPConn, missingNums *[]int64) {
	/*go func() {
		b.retransferMissingPackets()
	}()
	for {
		buf := make([]byte, PACKET_SIZE+100)
		bts, err := (*b.ctrlConn).Read(buf)
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
			b.Lock()
			*missingNums = utils.AppendIfMissing(*missingNums, v)
			b.Unlock()
		}
		// b.missingSequenceNums[index] = append(b.missingSequenceNums[index], p.MissingSequenceNumbers...)
		// log.Infof("Added %d sequenceNumbers to missingSequenceNumbers", len(p.MissingSequenceNumbers))
	}*/

}

func (b *BlocksConn) requestRetransfers(ctrlCon *net.UDPConn, missingNums *[]int64) {
	/*ticker := time.NewTicker(1000 * time.Millisecond)
	done := make(chan bool)
	log.Infof("In Call of requestRetransfers %p", missingNums)
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
						LastSequenceNumber:     b.lastReceivedSequenceNumber,
						BlockId:                b.BlockId,
						MissingSequenceNumbers: (*missingNums)[start:min],
					}

					err := enc.Encode(p)
					if err != nil {
						log.Fatal("encode error:", err)
					}
					//for _, v := range p.MissingSequenceNumbers {
					// log.Infof("Sending missing SequenceNums %v", v)
					//}

					_, err = (*ctrlCon).WriteTo(network.Bytes(), b.remoteCtrlAddr)
					if err != nil {
						log.Fatal("Write error:", err)
					}

					// log.Infof("Wrote %d ctrl bytes to client", bts)
					missingNumIndex += min
				}

			}
		}
	}(missingNums)
	*/
}
