package api

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/martenwallewein/blocks/blockmetrics"
	"github.com/martenwallewein/blocks/control"
	"github.com/martenwallewein/blocks/socket"
	"github.com/martenwallewein/blocks/utils"
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

type TransportSocketContstructor func() socket.TransportSocket
type TransportPackerContstructor func() socket.TransportPacketPacker

type BlocksSock struct {
	sync.Mutex
	activeBlockCount            int
	localAddr                   string
	remoteAddr                  string
	localStartPort              int
	remoteStartPort             int
	udpCons                     []net.Conn
	ctrlConn                    *net.UDPConn
	modes                       []int
	blockConns                  []*BlocksConn
	controlPlane                *control.ControlPlane
	aciveBlockIndex             int
	Metrics                     *blockmetrics.Metrics
	lastBlockRequestPacket      *socket.BlockRequestPacket
	testingMode                 bool
	transportSocketContstructor TransportSocketContstructor
	transportPackerConstructor  TransportPackerContstructor
	MaxSpeed                    int64
	NumCons                     int
}

func NewBlocksSock(localAddr, remoteAddr string, localStartPort, remoteStartPort, localCtrlPort, remoteCtrlPort int, numCons int) *BlocksSock {
	blockSock := &BlocksSock{
		localAddr:       localAddr,
		remoteAddr:      remoteAddr,
		localStartPort:  localStartPort,
		remoteStartPort: remoteStartPort,
		modes:           make([]int, numCons),
		blockConns:      make([]*BlocksConn, 0),
		aciveBlockIndex: 0,
		NumCons:         numCons,
	}
	var err error
	blockSock.controlPlane, err = control.NewControlPlane(localCtrlPort, remoteCtrlPort, localAddr, remoteAddr)
	if err != nil {
		log.Fatal(err)
	}

	for i := range blockSock.modes {
		conn := NewBlocksConn(localAddr, remoteAddr, localStartPort+i, remoteStartPort+i, nil)
		conn.ControlPlane = blockSock.controlPlane
		blockSock.blockConns = append(blockSock.blockConns, conn)

	}

	blockSock.Metrics = blockmetrics.NewMetrics(1000, numCons, func(index int) (uint64, uint64, uint64, uint64) {
		rxBytes := blockSock.blockConns[index].Metrics.RxBytes
		txBytes := blockSock.blockConns[index].Metrics.TxBytes
		rxPackets := blockSock.blockConns[index].Metrics.RxPackets
		txPackets := blockSock.blockConns[index].Metrics.TxPackets

		return rxBytes, txBytes, rxPackets, txPackets
	})

	// gob.Register(BlockPacket{})

	return blockSock
}
func (b *BlocksSock) SetMaxSpeed(maxSpeed int64) {
	b.MaxSpeed = maxSpeed
}

func (b *BlocksSock) SetTransportSocketConstructor(cons TransportSocketContstructor) {
	b.transportSocketContstructor = cons
	for _, v := range b.blockConns {
		v.SetTransportSocketConstructor(cons)
	}
}

func (b *BlocksSock) SetTransportPackerConstructor(cons TransportPackerContstructor) {
	b.transportPackerConstructor = cons
	for _, v := range b.blockConns {
		v.SetTransportPackerConstructor(cons)
	}
}

func (b *BlocksSock) EnableTestingMode() {
	b.testingMode = true
	for _, v := range b.blockConns {
		v.TestingMode = true
	}
}

func (b *BlocksSock) Dial() {
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
	go b.collectRetransfers()
}

func (b *BlocksSock) Listen() {
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
	go b.requestRetransfers()
}

func (b *BlocksSock) WriteBlock(block []byte) {
	blockLen := len(block)
	// TODO: Ensure waiting
	//go func(index int) {
	//	fmt.Printf("WriteBlock for index %d\n", index%NUM_BUFS)
	//		b.blockConns[index%NUM_BUFS].WriteBlock(block[halfLen:], int64(index+1)) // BlockIds positive
	//	}(b.aciveBlockIndex)
	//	b.aciveBlockIndex++
	var wg sync.WaitGroup
	partLen := (blockLen / b.NumCons)
	for i := 0; i < b.NumCons; i++ {
		wg.Add(1)
		go func(index int, wg *sync.WaitGroup) {
			fmt.Printf("WriteBlock for index %d\n", index%b.NumCons)
			start := partLen * index
			end := utils.Min(start+partLen, blockLen)
			b.blockConns[index%b.NumCons].WriteBlock(block[start:end], int64(index+1)) // BlockIds positive
			wg.Done()
		}(b.aciveBlockIndex, &wg)
		b.aciveBlockIndex++
	}

	wg.Wait()
	// fmt.Printf("WriteBlock for index %d\n", b.aciveBlockIndex%NUM_BUFS)
	// b.blockConns[b.aciveBlockIndex%NUM_BUFS].WriteBlock(block[:halfLen], int64(b.aciveBlockIndex+1)) // BlockIds positive
	// b.aciveBlockIndex++
}

func (b *BlocksSock) ReadBlock(block []byte) {
	blockLen := len(block)
	var wg sync.WaitGroup
	partLen := (blockLen / b.NumCons)
	for i := 0; i < b.NumCons; i++ {
		wg.Add(1)
		go func(index int, wg *sync.WaitGroup) {
			fmt.Printf("EadBlock for index %d\n", index%b.NumCons)
			start := partLen * index
			end := utils.Min(start+partLen, blockLen)
			log.Infof("Receiving for %d blockLen, start %d, %d partLen and end %d", len(block[start:end]), start, partLen, end)
			b.blockConns[index%b.NumCons].ReadBlock(block[start:end], int64(index+1)) // BlockIds positive
			wg.Done()
		}(b.aciveBlockIndex, &wg)
		b.aciveBlockIndex++
	}

	wg.Wait()

}

func (b *BlocksSock) collectRetransfers() {
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
		var p socket.BlockRequestPacket
		// TODO: Fix
		decodeReqPacket(&p, buf)
		if err != nil {
			log.Fatal("encode error:", err)
		}

		// log.Infof("Got BlockRequestPacket with maxSequenceNumber %d, %d missingNums and blockId %d", p.LastSequenceNumber, len(p.MissingSequenceNumbers), p.BlockId)
		// TODO: Add to retransfers
		blocksConn := b.blockConns[(p.BlockId-1)%int64(b.NumCons)]
		for i, v := range p.MissingSequenceNumbers {
			// log.Infof("Add %d to missing sequenceNumbers for client to send them back later", v)
			if b.lastBlockRequestPacket != nil && utils.IndexOf(v, b.lastBlockRequestPacket.MissingSequenceNumbers) >= 0 {
				// We continue here, because we want to avoid duplicate retransfers.
				// Might be improved later
			}
			index := utils.IndexOf(v, blocksConn.blockContext.MissingSequenceNums)
			if index < 0 {
				blocksConn.Lock()
				blocksConn.blockContext.MissingSequenceNums = append(blocksConn.blockContext.MissingSequenceNums, v)
				blocksConn.blockContext.MissingSequenceNumOffsets = append(blocksConn.blockContext.MissingSequenceNumOffsets, p.MissingSequenceNumberOffsets[i])
				blocksConn.Unlock()
			}

		}
		b.lastBlockRequestPacket = &p
		// b.missingSequenceNums[index] = append(b.missingSequenceNums[index], p.MissingSequenceNumbers...)
		// log.Infof("Added %d sequenceNumbers to missingSequenceNumbers", len(p.MissingSequenceNumbers))
	}

}

func (b *BlocksSock) requestRetransfers() {
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
			for _, blocksConn := range b.blockConns {
				if blocksConn.blockContext == nil {
					continue
				}
				missingNumsPerPacket := 200
				missingNumIndex := 0
				// log.Infof("Having %d missing Sequence Numbers for con index %d", len(blocksConn.blockContext.MissingSequenceNums), i)
				start := 0

				for missingNumIndex < len(blocksConn.blockContext.MissingSequenceNums) {
					min := utils.Min(start+missingNumsPerPacket, len(blocksConn.blockContext.MissingSequenceNums))
					var network bytes.Buffer        // Stand-in for a network connection
					enc := gob.NewEncoder(&network) // Will write to network.
					p := socket.BlockRequestPacket{
						BlockId:                      blocksConn.BlockId,
						LastSequenceNumber:           blocksConn.blockContext.HighestSequenceNumber,
						MissingSequenceNumbers:       (blocksConn.blockContext.MissingSequenceNums)[start:min],
						MissingSequenceNumberOffsets: (blocksConn.blockContext.MissingSequenceNumOffsets)[start:min],
						TransactionId:                txId,
					}

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
				}
			}

		}
	}

}
