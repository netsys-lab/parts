//go:generate zebrapack
package main

import (
	"fmt"
	"net"
	"sync"

	"github.com/martenwallewein/blocks/blockmetrics"
	"github.com/martenwallewein/blocks/control"
)

const (
	NUM_BUFS           = 1
	NUM_ACTIVE_BLOCKS  = 1
	BUF_SIZE           = 1024 * 1024
	PACKET_SIZE        = 1400
	BLOCKS_HEADER_SIZE = 16 // 95 // TODO: Fix this magic number
	MODE_SENDING       = 0
	MODE_RETRANSFER    = 1
	MODE_RECEIVING     = 2
	MODE_DONE          = 3
)

type BlocksSock struct {
	sync.Mutex
	activeBlockCount int
	localAddr        string
	remoteAddr       string
	localStartPort   int
	remoteStartPort  int
	udpCons          []net.Conn
	ctrlConn         *net.UDPConn
	modes            []int
	blockConns       []*BlocksConn
	controlPlane     *control.ControlPlane
	aciveBlockIndex  int
	Metrics          *blockmetrics.Metrics
}

func NewBlocksSock(localAddr, remoteAddr string, localStartPort, remoteStartPort, localCtrlPort, remoteCtrlPort int) *BlocksSock {
	blockSock := &BlocksSock{
		localAddr:       localAddr,
		remoteAddr:      remoteAddr,
		localStartPort:  localStartPort,
		remoteStartPort: remoteStartPort,
		modes:           make([]int, NUM_BUFS),
		blockConns:      make([]*BlocksConn, 0),
		aciveBlockIndex: 0,
	}

	// blockSock.controlPlane = control.NewControlPlane(localCtrlPort, remoteCtrlPort)

	for i := range blockSock.modes {
		blockSock.blockConns = append(blockSock.blockConns, NewBlocksConn(localAddr, remoteAddr, localStartPort+i, remoteStartPort+i, nil))
	}

	blockSock.Metrics = blockmetrics.NewMetrics(1000, NUM_BUFS, func(index int) (uint64, uint64, uint64, uint64) {
		rxBytes := blockSock.blockConns[index].Metrics.RxBytes
		txBytes := blockSock.blockConns[index].Metrics.TxBytes
		rxPackets := blockSock.blockConns[index].Metrics.RxPackets
		txPackets := blockSock.blockConns[index].Metrics.TxPackets

		return rxBytes, txBytes, rxPackets, txPackets
	})

	// gob.Register(BlockPacket{})

	return blockSock
}

func (b *BlocksSock) dial() {
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
			b.blockConns[i].ctrlConn = b.ctrlConn
			b.blockConns[i].remoteCtrlAddr = b.remoteCtrlAddr
			b.blockConns[i].localCtrlAddr = b.localCtrlAddr
		}

		go b.collectRetransfers()
	}*/
}

func (b *BlocksSock) listen() {
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
			b.blockConns[i].ctrlConn = b.ctrlConn
			b.blockConns[i].remoteCtrlAddr = b.remoteCtrlAddr
			b.blockConns[i].localCtrlAddr = b.localCtrlAddr
		}

		go b.requestRetransfers()

	}*/
}

func (b *BlocksSock) WriteBlock(block []byte) {
	halfLen := len(block) / 1
	// TODO: Ensure waiting
	//go func(index int) {
	//	fmt.Printf("WriteBlock for index %d\n", index%NUM_BUFS)
	//		b.blockConns[index%NUM_BUFS].WriteBlock(block[halfLen:], int64(index+1)) // BlockIds positive
	//	}(b.aciveBlockIndex)
	//	b.aciveBlockIndex++
	fmt.Printf("WriteBlock for inde2x %d\n", b.aciveBlockIndex%NUM_BUFS)
	b.blockConns[b.aciveBlockIndex%NUM_BUFS].WriteBlock(block[:halfLen], int64(b.aciveBlockIndex+1)) // BlockIds positive
	b.aciveBlockIndex++
}

func (b *BlocksSock) ReadBlock(block []byte) {
	halfLen := len(block) / 1
	// TODO: Ensure waiting
	//go func(index int) {
	//	fmt.Printf("ReadBlock for index %d\n", index%NUM_BUFS)
	//		b.blockConns[index%NUM_BUFS].ReadBlock(block[halfLen:], int64(index+1)) // BlockIds positive
	//	}(b.aciveBlockIndex)

	fmt.Printf("ReadBlock for inde2x %d\n", b.aciveBlockIndex%NUM_BUFS)
	b.blockConns[b.aciveBlockIndex%NUM_BUFS].ReadBlock(block[:halfLen], int64(b.aciveBlockIndex+1)) // BlockIds positive
	b.aciveBlockIndex++

}

func (b *BlocksSock) collectRetransfers() {
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
		var p BlockRequestPacket
		// TODO: Fix
		decodeReqPacket(&p, buf)
		if err != nil {
			log.Fatal("encode error:", err)
		}

		log.Infof("Got BlockRequestPacket with maxSequenceNumber %d and blockId", p.LastSequenceNumber, p.BlockId)
		// TODO: Add to retransfers
		blocksConn := b.blockConns[(p.BlockId-1)%NUM_BUFS]
		for _, v := range p.MissingSequenceNumbers {
			// log.Infof("Add %d to missing sequenceNumbers for client to send them back later", v)
			blocksConn.Lock()
			blocksConn.missingSequenceNums = AppendIfMissing(blocksConn.missingSequenceNums, v)
			blocksConn.Unlock()
		}
		// b.missingSequenceNums[index] = append(b.missingSequenceNums[index], p.MissingSequenceNumbers...)
		// log.Infof("Added %d sequenceNumbers to missingSequenceNumbers", len(p.MissingSequenceNumbers))
	}
	*/
}

func (b *BlocksSock) requestRetransfers() {
	/*ticker := time.NewTicker(1000 * time.Millisecond)
	done := make(chan bool)
	// log.Infof("In Call of requestRetransfers %p", missingNums)
	// log.Infof("In Call of requestRetransfers go routine %p", missingNums)
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			for _, blocksConn := range b.blockConns {
				missingNumsPerPacket := 100
				missingNumIndex := 0
				start := 0
				// for _, v := range *missingNums {
				//	log.Infof("Having missing SequenceNums %v", v)
				//}

				for missingNumIndex < len(blocksConn.missingSequenceNums) {
					min := Min(start+missingNumsPerPacket, len(blocksConn.missingSequenceNums))
					var network bytes.Buffer        // Stand-in for a network connection
					enc := gob.NewEncoder(&network) // Will write to network.
					p := BlockRequestPacket{
						LastSequenceNumber:     blocksConn.lastReceivedSequenceNumber,
						BlockId:                blocksConn.BlockId,
						MissingSequenceNumbers: (blocksConn.missingSequenceNums)[start:min],
					}

					err := enc.Encode(p)
					if err != nil {
						log.Fatal("encode error:", err)
					}
					//for _, v := range p.MissingSequenceNumbers {
					// log.Infof("Sending missing SequenceNums %v", v)
					//}

					_, err = (b.ctrlConn).WriteTo(network.Bytes(), b.remoteCtrlAddr)
					if err != nil {
						log.Fatal("Write error:", err)
					}

					// log.Infof("Wrote %d ctrl bytes to client", bts)
					missingNumIndex += min
				}
			}

		}
	}
	*/
}
