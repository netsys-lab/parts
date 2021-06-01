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
	BLOCKS_HEADER_SIZE = 95 // TODO: Fix this magic number
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
	ctrlConn                   net.Conn
	modes                      []int
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
	}

	for i := range blockSock.buffers {
		blockSock.buffers[i] = make([]byte, BUF_SIZE)
	}

	// gob.Register(BlockPacket{})

	return blockSock
}

func (b BlocksSock) WriteBlock(block []byte) {
	// TODO: Save activeBlockCount and increase immediatly
	// TODO: Not overwrite if actually sending
	b.blocks[b.activeBlockCount] = block
	b.activeBlockIndizes[b.activeBlockCount] = 0
	b.packets[b.activeBlockCount] = make([][]byte, CeilForce(int64(len(block)), PACKET_SIZE-BLOCKS_HEADER_SIZE))
	b.retransferPackets[b.activeBlockCount] = make([][]byte, CeilForce(int64(len(block)), PACKET_SIZE-BLOCKS_HEADER_SIZE))
	log.Infof("Writing %d packets", len(b.packets[b.activeBlockCount]))
	// b.packets[b.activeBlockCount] = make([][]byte, len(block)/PACKET_SIZE)
	// TODO: Waiting queue
	// TODO: sync write calls
	// TODO: Resend packets
	b.modes[b.activeBlockCount] = MODE_SENDING
	b.dial(b.activeBlockCount)
	b.ctrlConn = b.listenCtrl(b.activeBlockCount)
	fmt.Println(b.ctrlConn)
	go func(index int, ctrlCon *net.Conn) {
		b.collectRetransfers(index, ctrlCon)
	}(b.activeBlockCount, &b.ctrlConn)

	// TODO: Synchronize this
	b.createPackets(b.activeBlockCount)
	b.sendPackets(b.activeBlockCount)
	b.modes[b.activeBlockCount] = MODE_RETRANSFER
	b.retransferMissingPackets(b.activeBlockCount)
	b.activeBlockCount++
	time.Sleep(100 * time.Second)
}

func (b BlocksSock) ReadBlock(block *[]byte) {
	// TODO: Not overwrite if actually receiving
	b.blocks[b.activeBlockCount] = *block
	b.activeBlockIndizes[b.activeBlockCount] = 0
	// TODO: This assumption here is bullshit, we need to read block size from first packet of block id
	// TODO: How to ensure order of parallel sent blocks? Increasing blockIds?
	b.packets[b.activeBlockCount] = make([][]byte, CeilForce(int64(len(*block)), PACKET_SIZE-BLOCKS_HEADER_SIZE))
	b.retransferPackets[b.activeBlockCount] = make([][]byte, CeilForce(int64(len(*block)), PACKET_SIZE-BLOCKS_HEADER_SIZE))
	log.Infof("Receiving %d packets", len(b.packets[b.activeBlockCount]))
	// TODO: Waiting queue
	// TODO: sync write calls
	b.listen(b.activeBlockCount)
	b.ctrlConn = b.dialCtrl(b.activeBlockCount)
	fmt.Println(b.ctrlConn)
	go func(index int) {
		b.receivePackets(index)
	}(b.activeBlockCount)
	go func(index int, ctrlCon *net.Conn) {
		fmt.Println(*ctrlCon)
		b.requestRetransfers(index, ctrlCon)
	}(b.activeBlockCount, &b.ctrlConn)
	_bytes := b.parsePackets(b.activeBlockCount)
	// log.Infof("After parsePackets md5 %x", md5.Sum(_bytes))
	num := copy(*block, _bytes)
	fmt.Printf("Copied %d bytes to block\n", num)
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

func (b BlocksSock) listenCtrl(index int) *net.UDPConn {
	if b.ctrlConn != nil {
		return nil
	}
	laddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.localAddr, b.localCtrlPort))
	if err != nil {
		log.Fatal("error:", err)
	}
	ctrlConn, err := net.ListenUDP("udp", laddr)
	fmt.Printf("Listen Ctrl on %s for index %d\n", laddr.String(), index)
	if err != nil {
		log.Fatal("error:", err)
	}

	return ctrlConn
}

func (b BlocksSock) dialCtrl(index int) *net.UDPConn {
	if b.ctrlConn != nil {
		return nil
	}
	raddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.remoteAddr, b.remoteCtrlPort))
	if err != nil {
		log.Fatal("error:", err)
	}
	laddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.localAddr, b.localCtrlPort))
	if err != nil {
		log.Fatal("error:", err)
	}

	con, err := net.DialUDP("udp", laddr, raddr)
	if err != nil {
		log.Fatal("error:", err)
	}
	log.Infof("Dial Ctrl from %s to %s", laddr.String(), raddr.String())
	fmt.Println(con)
	return con
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
		// log.Infof("Sending md5 %x", md5.Sum(b.packets[index][i]))
		_, err := b.udpCons[index].Write(b.packets[index][i])
		time.Sleep(100 * time.Microsecond)
		if err != nil {
			log.Fatal("error:", err)
		}
	}
}

func (b BlocksSock) receivePackets(index int) {
	length := len(b.packets[index])
	i := 0
	j := 0
	for b.modes[index] != MODE_DONE {
		if i >= length {
			i = 0
		}
		// b.packets[index][i] = make([]byte, PACKET_SIZE+1000)
		// TODO: Check bytes read
		// fmt.Println("READING")

		buf := make([]byte, PACKET_SIZE+1000)
		bts, err := b.udpCons[index].Read(buf)
		if err != nil {
			log.Fatal("error:", err)
		}

		if j > 0 && j%1000 == 0 {
			j++
			log.Infof("CONTIENU %d", j)
			continue
		}

		// log.Infof("Receiving md5 %x", md5.Sum(buf[:bts]))
		b.packets[index][i] = buf[:bts] //make([]byte, bts)
		// copy(b.packets[index][i], buf[:bts])
		// fmt.Println(b.packets[index][i])
		// fmt.Printf("READ BYTES %d\n for index %d\n", bts, i)
		i++
		j++
	}
}

func (b BlocksSock) collectRetransfers(index int, ctrlCon *net.Conn) {
	go func(index int) {
		b.retransferMissingPackets(index)
	}(index)
	for {
		buf := make([]byte, PACKET_SIZE+1000)
		bts, err := b.ctrlConn.Read(buf)
		if err != nil {
			log.Fatal("error:", err)
		}

		log.Infof("Received %d ctrl bytes", bts)
		var p BlockRequestPacket
		network := bytes.NewBuffer(buf)
		dec := gob.NewDecoder(network)
		err = dec.Decode(&p)
		if err != nil {
			log.Fatal("encode error:", err)
		}

		log.Infof("Got BlockRequestPacket with maxSequenceNumber %d and blockId", p.LastSequenceNumber, p.BlockId)
		// TODO: Add to retransfers
		for _, v := range p.MissingSequenceNumbers {
			log.Debugf("Add %d to missing sequenceNumbers for client to send them back later", v)
			b.missingSequenceNums[index] = AppendIfMissing(b.missingSequenceNums[index], v)
		}
		// b.missingSequenceNums[index] = append(b.missingSequenceNums[index], p.MissingSequenceNumbers...)
		log.Infof("Added %d sequenceNumbers to missingSequenceNumbers", len(p.MissingSequenceNumbers))
	}

}

func (b BlocksSock) requestRetransfers(index int, ctrlCon *net.Conn) {
	ticker := time.NewTicker(1000 * time.Millisecond)
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				missingNumsPerPacket := 100
				missingNumIndex := 0
				start := 0
				for missingNumIndex < len(b.missingSequenceNums[index]) {
					min := Min(start+missingNumsPerPacket, len(b.missingSequenceNums[index]))
					var network bytes.Buffer        // Stand-in for a network connection
					enc := gob.NewEncoder(&network) // Will write to network.
					p := BlockRequestPacket{
						LastSequenceNumber:     b.lastReceivedSequenceNumber[index],
						BlockId:                int64(index),
						MissingSequenceNumbers: b.missingSequenceNums[index][start:min],
					}

					err := enc.Encode(p)
					if err != nil {
						log.Fatal("encode error:", err)
					}

					bts, err := (*ctrlCon).Write(network.Bytes())
					if err != nil {
						log.Fatal("encode error:", err)
					}

					log.Infof("Wrote %d ctrl bytes to client", bts)
					missingNumIndex += min
				}

			}
		}
	}()
}

func indexOf(element int64, data []int64) int {
	for k, v := range data {
		if element == v {
			return k
		}
	}
	return -1 //not found.
}

func removeFromSlice(s []int64, i int64) []int64 {
	s[indexOf(i, s)] = s[len(s)-1]
	return s[:len(s)-1]
}

func (b BlocksSock) parsePackets(index int) []byte {
	var p BlockPacket
	_bytes := make([]byte, len(b.blocks[index]))
	_bytes2 := make([]byte, 0)
	startSequenceNumber := 0
	var highestSequenceNumber int64 = 0
	packetIndex := 0
	var payloadLen int64 = 0
	done := false
	for packetIndex < len(b.packets[index]) {
		for b.packets[index][packetIndex] == nil {

		}
		retransfer := false
		// log.Infof("Received md5 %x", md5.Sum(b.packets[index][packetIndex]))
		network := bytes.NewBuffer(b.packets[index][packetIndex])
		dec := gob.NewDecoder(network)
		err := dec.Decode(&p)
		if err != nil {
			log.Infof("PacketIndex %d ", packetIndex)
			log.Errorf("encode error:", err)
		}
		// fmt.Printf("Received packet with sequenceNumber %d\n", p.SequenceNumber)
		if startSequenceNumber == 0 {
			startSequenceNumber = int(p.SequenceNumber)
		}
		diff := p.SequenceNumber - highestSequenceNumber

		/*if packetIndex > 0 && packetIndex%100 == 0 {
			b.missingSequenceNums[index] = append(b.missingSequenceNums[index], p.SequenceNumber)
			log.Infof("Appending missing sequence number %d", p.SequenceNumber)
		}*/

		if diff > 1 {
			var off int64 = 1
			for off < diff {
				b.missingSequenceNums[index] = AppendIfMissing(b.missingSequenceNums[index], p.SequenceNumber-off)
				log.Infof("Appending missing sequence number %d for highest number %d", p.SequenceNumber-off, highestSequenceNumber)
				off++
			}
		} else if diff < 0 {
			retransfer = true
			log.Infof("Received retransferred sequence number %d", p.SequenceNumber)
			b.missingSequenceNums[index] = removeFromSlice(b.missingSequenceNums[index], p.SequenceNumber)
			if len(b.missingSequenceNums[index]) == 0 {
				done = true
			}
		}
		// TODO: This must support reordering
		// _bytes[highestSequenceNumber-int64(startSequenceNumber)] = p.Payload
		// TODO: Performance fix!
		if payloadLen == 0 {
			payloadLen = int64(len(p.Payload))
		}
		// curPayloadLen := int64(len(p.Payload))
		if retransfer {
			log.Infof("Writing SequenceNumber %d to bytes index %d", p.SequenceNumber, (p.SequenceNumber-int64(startSequenceNumber))*payloadLen)
		}
		//
		startIndex := (p.SequenceNumber - int64(startSequenceNumber)) * payloadLen
		// copy(_bytes[startIndex:startIndex+curPayloadLen], p.Payload)
		for i, v := range p.Payload {
			_bytes[startIndex+int64(i)] = v
		}
		// log.Infof("Received md5 payload %x", md5.Sum(p.Payload))
		_bytes2 = append(_bytes2, p.Payload...)
		if diff > 0 {
			highestSequenceNumber += diff
		}

		b.lastReceivedSequenceNumber[index] = highestSequenceNumber
		packetIndex++

		if done {
			b.modes[index] = MODE_DONE
			break
		}
	}

	/*for i := range b.packets[index] {
		// fmt.Println(b.packets[index][i])
		network := bytes.NewBuffer(b.packets[index][i])
		dec := gob.NewDecoder(network)
		err := dec.Decode(&p)
		if err != nil {
			log.Fatal("encode error:", err)
		}
		fmt.Printf("Received packet with sequenceNumber %d\n", p.SequenceNumber)

		_bytes = append(_bytes, p.Payload...)
	}*/
	// log.Infof("Final md5 _bytes2 %x", md5.Sum(_bytes2))

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

func AppendIfMissing(slice []int64, i int64) []int64 {
	for _, ele := range slice {
		if ele == i {
			return slice
		}
	}
	return append(slice, i)
}

func (b BlocksSock) createPackets(index int) {
	start := 0
	end := len(b.blocks[index])
	sequenceNumber := 1
	b.startSequenceNumbers[index] = int64(sequenceNumber)
	pIndex := 0
	for start < end {
		next := PACKET_SIZE - BLOCKS_HEADER_SIZE
		var network bytes.Buffer        // Stand-in for a network connection
		enc := gob.NewEncoder(&network) // Will write to network.
		min := Min(start+next, end)
		p := BlockPacket{
			SequenceNumber: int64(sequenceNumber),
			BlockId:        int64(index),
			BlockSize:      0, // int64(end), TODO: Changes header size
			Payload:        b.blocks[index][start:min],
		}
		// log.Infof("Before Send md5 payload %x", md5.Sum(p.Payload))
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

func (b BlocksSock) retransferMissingPackets(index int) {
	for b.modes[index] == MODE_RETRANSFER {
		for _, v := range b.missingSequenceNums[index] {
			packet := b.packets[index][v-b.startSequenceNumbers[index]]
			_, err := b.udpCons[index].Write(packet)
			// time.Sleep(1 * time.Microsecond)
			if err != nil {
				log.Fatal("error:", err)
			}
		}
		b.missingSequenceNums[index] = make([]int64, 0)
		time.Sleep(100 * time.Microsecond)
	}
}
