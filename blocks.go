package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	NUM_BUFS           = 1
	NUM_ACTIVE_BLOCKS  = 1
	BUF_SIZE           = 1024 * 1024
	PACKET_SIZE        = 1400
	BLOCKS_HEADER_SIZE = 103
)

type BlockPacket struct {
	SequenceNumber int64
	BlockId        int64
	BlockSize      int64
	Payload        []byte
}

type BlocksSock struct {
	buffers            [][]byte
	packets            [][][]byte
	blocks             [][]byte
	activeBlockIndizes []int
	activeBlockCount   int
	localAddr          string
	remoteAddr         string
	localStartPort     int
	remoteStartPort    int
	udpCons            []net.Conn
}

func NewBlocksSock(localAddr, remoteAddr string, localStartPort, remoteStartPort int) *BlocksSock {
	blockSock := &BlocksSock{
		buffers:            make([][]byte, NUM_BUFS),
		blocks:             make([][]byte, NUM_ACTIVE_BLOCKS),
		packets:            make([][][]byte, NUM_ACTIVE_BLOCKS),
		activeBlockIndizes: make([]int, NUM_ACTIVE_BLOCKS),
		localAddr:          localAddr,
		remoteAddr:         remoteAddr,
		localStartPort:     localStartPort,
		remoteStartPort:    remoteStartPort,
		udpCons:            make([]net.Conn, NUM_BUFS),
	}

	for i := range blockSock.buffers {
		blockSock.buffers[i] = make([]byte, BUF_SIZE)
	}

	// gob.Register(BlockPacket{})

	return blockSock
}

func (b BlocksSock) WriteBlock(block []byte) {
	// TODO: Not overwrite if actually sending
	b.blocks[b.activeBlockCount] = block
	b.activeBlockIndizes[b.activeBlockCount] = 0
	b.packets[b.activeBlockCount] = make([][]byte, CeilForce(int64(len(block)), PACKET_SIZE-BLOCKS_HEADER_SIZE))
	log.Infof("Writing %d packets", len(b.packets[b.activeBlockCount]))
	// b.packets[b.activeBlockCount] = make([][]byte, len(block)/PACKET_SIZE)
	// TODO: Waiting queue
	// TODO: sync write calls
	// TODO: Resend packets
	b.dial(b.activeBlockCount)
	b.createPackets(b.activeBlockCount)
	b.sendPackets(b.activeBlockCount)
	b.activeBlockCount++

}

func (b BlocksSock) ReadBlock(block []byte) {
	// TODO: Not overwrite if actually receiving
	b.blocks[b.activeBlockCount] = block
	b.activeBlockIndizes[b.activeBlockCount] = 0
	// TODO: This assumption here is bullshit, we need to read block size from first packet of block id
	// TODO: How to ensure order of parallel sent blocks? Increasing blockIds?
	// fmt.Println(len(block) / PACKET_SIZE)
	b.packets[b.activeBlockCount] = make([][]byte, CeilForce(int64(len(block)), PACKET_SIZE-BLOCKS_HEADER_SIZE))
	log.Infof("Receiving %d packets", len(b.packets[b.activeBlockCount]))
	// fmt.Println(len(b.packets[b.activeBlockCount]))
	// TODO: Waiting queue
	// TODO: sync write calls
	b.listen(b.activeBlockCount)
	b.receivePackets(b.activeBlockCount)
	_bytes := b.parsePackets(b.activeBlockCount)
	num := copy(block, _bytes)
	fmt.Printf("Copied %d bytes to block", num)
	b.activeBlockCount++
}

func (b BlocksSock) listen(index int) {
	if b.udpCons[index] != nil {
		return
	}
	laddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.localAddr, b.localStartPort))
	if err != nil {
		log.Fatal("error:", err)
	}
	b.udpCons[index], err = net.ListenUDP("udp", laddr)
	fmt.Printf("Listen on %s for index %d\n", laddr.String(), index)
	if err != nil {
		log.Fatal("error:", err)
	}
}

func (b BlocksSock) dial(index int) {
	if b.udpCons[index] != nil {
		return
	}
	raddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.remoteAddr, b.remoteStartPort))
	if err != nil {
		log.Fatal("error:", err)
	}
	laddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.localAddr, b.localStartPort))
	if err != nil {
		log.Fatal("error:", err)
	}
	b.udpCons[index], err = net.DialUDP("udp", laddr, raddr)
	if err != nil {
		log.Fatal("error:", err)
	}
}

func (b BlocksSock) sendPackets(index int) {
	for i := range b.packets[index] {
		_, err := b.udpCons[index].Write(b.packets[index][i])
		time.Sleep(2 * time.Millisecond)
		if err != nil {
			log.Fatal("error:", err)
		}
	}
}

func (b BlocksSock) receivePackets(index int) {
	for i := range b.packets[index] {
		// TODO: Check bytes read
		// fmt.Println("READING")
		buf := make([]byte, PACKET_SIZE+1000)
		_, err := b.udpCons[index].Read(buf)
		b.packets[index][i] = buf
		// copy(b.packets[index][i], buf)
		// fmt.Println(b.packets[index][i])
		// fmt.Printf("READ BYTES %d\n for index %d\n", bts, i)
		if err != nil {
			log.Fatal("error:", err)
		}
	}
}

func (b BlocksSock) parsePackets(index int) []byte {
	var p BlockPacket
	_bytes := make([]byte, 0)
	for i := range b.packets[index] {
		// fmt.Println(b.packets[index][i])
		network := bytes.NewBuffer(b.packets[index][i])
		dec := gob.NewDecoder(network)
		err := dec.Decode(&p)
		if err != nil {
			log.Fatal("encode error:", err)
		}
		fmt.Printf("Received packet with sequenceNumber %d\n", p.SequenceNumber)

		_bytes = append(_bytes, p.Payload...)
	}

	return _bytes

}

func Max(x, y int) int {
	if x < y {
		return y
	}
	return x
}

func Min(x, y int) int {
	if x > y {
		return y
	}
	return x
}

func CeilForce(x, y int64) int64 {
	res := x / y
	f := float64(x) / float64(y)
	if f > float64(res) {
		return res + 1
	} else {
		return res
	}
}

func (b BlocksSock) createPackets(index int) {
	start := 0
	end := len(b.blocks[index])
	sequenceNumber := 1
	pIndex := 0
	for start < end {
		next := PACKET_SIZE - BLOCKS_HEADER_SIZE
		var network bytes.Buffer        // Stand-in for a network connection
		enc := gob.NewEncoder(&network) // Will write to network.
		min := Min(start+next, len(b.blocks[index]))
		p := BlockPacket{
			SequenceNumber: int64(sequenceNumber),
			BlockId:        int64(index),
			BlockSize:      int64(end),
			Payload:        b.blocks[index][start:min],
		}
		err := enc.Encode(p)
		if err != nil {
			log.Fatal("encode error:", err)
		}
		b.packets[index][pIndex] = network.Bytes()
		sequenceNumber++
		start += next
		pIndex++
	}
}
