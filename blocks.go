//go:generate zebrapack
package main

import (
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
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

type BlockRequestPacket struct {
	BlockId                int64
	LastSequenceNumber     int64
	MissingSequenceNumbers []int64
}

type BlockPacket struct {
	SequenceNumber int64
	BlockId        int64
	BlockSize      int64
	Payload        []byte
}

type BlocksSock struct {
	buffers                    [][]byte
	packets                    [][][]byte
	blocks                     [][]byte
	activeBlockIndizes         []int
	activeBlockCount           int
	localAddr                  string
	remoteAddr                 string
	localStartPort             int
	remoteStartPort            int
	localCtrlPort              int
	remoteCtrlPort             int
	lastReceivedSequenceNumber []int64
	lastRequestedSequenceIndex []int64
	retransferPackets          [][][]byte
	missingSequenceNums        [][]int64
	startSequenceNumbers       []int64
	udpCons                    []net.Conn
	ctrlConn                   *net.UDPConn
	modes                      []int
	receivedBytes              int64
	lastReceivedBytes          int64
	sentBytes                  int64
	lastSentBytes              int64
	receivedPackets            int64
	processedPackets           int64
	sentPacets                 int64
	blockConns                 []*BlocksConn
	remoteCtrlAddr             *net.UDPAddr
	localCtrlAddr              *net.UDPAddr
}

func NewBlocksSock(localAddr, remoteAddr string, localStartPort, remoteStartPort, localCtrlPort, remoteCtrlPort int) *BlocksSock {
	blockSock := &BlocksSock{
		buffers:                    make([][]byte, NUM_BUFS),
		blocks:                     make([][]byte, NUM_ACTIVE_BLOCKS),
		packets:                    make([][][]byte, NUM_ACTIVE_BLOCKS),
		retransferPackets:          make([][][]byte, NUM_ACTIVE_BLOCKS),
		missingSequenceNums:        make([][]int64, NUM_ACTIVE_BLOCKS),
		lastReceivedSequenceNumber: make([]int64, NUM_ACTIVE_BLOCKS),
		lastRequestedSequenceIndex: make([]int64, NUM_ACTIVE_BLOCKS),
		startSequenceNumbers:       make([]int64, NUM_ACTIVE_BLOCKS),
		activeBlockIndizes:         make([]int, NUM_ACTIVE_BLOCKS),
		localAddr:                  localAddr,
		remoteAddr:                 remoteAddr,
		localStartPort:             localStartPort,
		remoteStartPort:            remoteStartPort,
		localCtrlPort:              localCtrlPort,
		remoteCtrlPort:             remoteCtrlPort,
		udpCons:                    make([]net.Conn, NUM_BUFS),
		modes:                      make([]int, NUM_BUFS),
		blockConns:                 make([]*BlocksConn, 0),
	}

	for i := range blockSock.buffers {
		blockSock.buffers[i] = make([]byte, BUF_SIZE)
		blockSock.blockConns = append(blockSock.blockConns, NewBlocksConn(localAddr, remoteAddr, localStartPort+i, remoteStartPort+i, nil))
	}

	// gob.Register(BlockPacket{})

	return blockSock
}

func (b BlocksSock) WriteBlock(block []byte) {
	if b.ctrlConn == nil {

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

		b.blockConns[0].ctrlConn = b.ctrlConn
		b.blockConns[0].remoteCtrlAddr = b.remoteCtrlAddr
		b.blockConns[0].localCtrlAddr = b.localCtrlAddr
		b.blockConns[0].WriteBlock(block)
	}
}

func (b BlocksSock) ReadBlock(block []byte) {
	if b.ctrlConn == nil {

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
	}
	b.blockConns[0].ctrlConn = b.ctrlConn
	b.blockConns[0].remoteCtrlAddr = b.remoteCtrlAddr
	b.blockConns[0].localCtrlAddr = b.localCtrlAddr
	fmt.Println(b.blockConns[0].ctrlConn)
	b.blockConns[0].ReadBlock(block)
}
