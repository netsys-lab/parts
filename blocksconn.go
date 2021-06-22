package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/martenwallewein/blocks/packet"
	log "github.com/sirupsen/logrus"
)

type BlocksConn struct {
	sync.Mutex
	packets                    [][]byte
	block                      []byte
	localAddr                  string
	remoteAddr                 string
	localStartPort             int
	remoteStartPort            int
	lastRequestedSequenceIndex []int64
	retransferPackets          [][]byte
	missingSequenceNums        []int64
	dataConn                   *net.UDPConn
	ctrlConn                   *net.UDPConn
	mode                       int
	receivedBytes              int64
	lastReceivedBytes          int64
	sentBytes                  int64
	lastSentBytes              int64
	receivedPackets            int64
	processedPackets           int64
	sentPacets                 int64
	BlockId                    int64
	lastReceivedSequenceNumber int64
	nextPacketIndex            *int
	processPacketIndex         *int
	remoteCtrlAddr             *net.UDPAddr
	localCtrlAddr              *net.UDPAddr
	packetPacker               *packet.SCIONPacketPacker
}

func NewBlocksConn(localAddr, remoteAddr string, localStartPort, remoteStartPort int, ctrlConn *net.UDPConn) *BlocksConn {
	sPacketPacker, err := packet.NewSCIONPacketPacker(remoteAddr, localAddr, remoteStartPort)
	if err != nil {
		log.Fatal(err)
	}
	blocksConn := &BlocksConn{
		missingSequenceNums: make([]int64, 0),
		localAddr:           localAddr,
		remoteAddr:          remoteAddr,
		localStartPort:      localStartPort,
		remoteStartPort:     remoteStartPort,
		ctrlConn:            ctrlConn,
		packetPacker:        sPacketPacker,
	}
	val := 0
	val2 := 0
	blocksConn.nextPacketIndex = &val
	blocksConn.processPacketIndex = &val2

	return blocksConn
}

func (b *BlocksConn) createPackets() {
	start := 0
	end := len(b.block)
	sequenceNumber := 1
	pIndex := 0
	bt := make([]byte, 8)
	scionHeaderLen := b.packetPacker.GetHeaderLen()
	for start < end {
		buf := make([]byte, 0)
		next := PACKET_SIZE - BLOCKS_HEADER_SIZE - scionHeaderLen
		// Will write to network.
		// var network bytes.Buffer        // Stand-in for a network connection
		// enc := gob.NewEncoder(&network) // Will write to network.
		min := Min(start+next, end)
		/*p := BlockPacket{
			SequenceNumber: int64(sequenceNumber),
			BlockId:        b.BlockId,
			BlockSize:      0, // int64(end), TODO: Changes header size
			Payload:        b.block[start:min],
		}*/
		b.packetPacker.Pack(&buf, 0, uint16(min-start+BLOCKS_HEADER_SIZE+8))
		binary.BigEndian.PutUint64(bt, uint64(sequenceNumber))
		buf = append(buf, bt...)
		// fmt.Println(buf)
		binary.BigEndian.PutUint64(bt, uint64(b.BlockId))
		buf = append(buf, bt...)

		buf = append(buf, b.block[start:min]...)
		// log.Infof("Send seq %d and md5 for buf %x", uint64(sequenceNumber), md5.Sum(buf))
		// fmt.Println(buf[0:8])
		b.packets[pIndex] = buf
		// log.Infof("Before Send md5 payload %x", md5.Sum(p.Payload))
		// err := encodePacket(&p, b.packets[pIndex])
		// err := enc.Encode(p)
		// b.packets[pIndex] = network.Bytes()
		// if err != nil {
		//	log.Fatalf("error: %s", err)
		//}
		sequenceNumber++
		start += next
		pIndex++
	}
}

func (b *BlocksConn) retransferMissingPackets(missingNums *[]int64) {
	ticker := time.NewTicker(1000 * time.Millisecond)
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
			packet := b.packets[v-1]
			// log.Infof("Sending back %d", v-1)
			bts, err := (*b.dataConn).Write(packet)
			b.receivedBytes += int64(bts)
			b.receivedPackets++
			// time.Sleep(1 * time.Microsecond)
			if err != nil {
				log.Fatal("error:", err)
			}
		}
		b.Lock()
		*missingNums = make([]int64, 0)
		b.Unlock()
		time.Sleep(10 * time.Microsecond)
	}
	ticker.Stop()
	done <- true
}

func (b *BlocksConn) WriteBlock(block []byte, blockId int64) {
	// TODO: Save activeBlockCount and increase immediatly
	// TODO: Not overwrite if actually sending
	b.block = block
	b.BlockId = int64(blockId)
	scionHeaderLen := b.packetPacker.GetHeaderLen()
	b.packets = make([][]byte, CeilForce(int64(len(block)), int64(PACKET_SIZE-BLOCKS_HEADER_SIZE-scionHeaderLen)))
	log.Infof("Writing %d packets", len(b.packets))
	// b.packets[b.activeBlockCount] = make([][]byte, len(block)/PACKET_SIZE)
	// TODO: Waiting queue
	// TODO: sync write calls
	b.mode = MODE_SENDING
	if b.dataConn == nil {
		/*raddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.remoteAddr, b.remoteStartPort))
		if err != nil {
			log.Fatal("error:", err)
		}
		laddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.localAddr, b.localStartPort))
		if err != nil {
			log.Fatal("error:", err)
		}*/
		dstUpdAdr := &net.UDPAddr{
			IP:   b.packetPacker.DestAddr.NextHop.IP,
			Port: b.packetPacker.DestAddr.NextHop.Port,
		}
		updAdr := &net.UDPAddr{
			IP:   b.packetPacker.LocalAddr.IP,
			Port: b.localStartPort,
		}
		var err error
		b.dataConn, err = net.DialUDP("udp", updAdr, dstUpdAdr)
		if err != nil {
			log.Fatal("error:", err)
		}
		fmt.Printf("Dialed to %s", b.dataConn.RemoteAddr())
	}
	fmt.Println(b.ctrlConn)
	/*go func(ctrlCon *net.UDPConn, missingNums *[]int64) {
		b.collectRetransfers(ctrlCon, missingNums)
	}(b.ctrlConn, &b.missingSequenceNums)*/

	// TODO: Synchronize this
	b.createPackets()
	b.sendPackets(b.dataConn)
	b.mode = MODE_RETRANSFER
	b.retransferMissingPackets(&b.missingSequenceNums)
	time.Sleep(100 * time.Second)
}

func (b *BlocksConn) ReadBlock(block []byte, blockId int64) {
	// TODO: Not overwrite if actually receiving
	b.block = block
	b.BlockId = blockId
	// TODO: This assumption here is bullshit, we need to read block size from first packet of block id
	// TODO: How to ensure order of parallel sent blocks? Increasing blockIds?
	scionHeaderLen := b.packetPacker.GetHeaderLen()
	b.packets = make([][]byte, CeilForce(int64(len(block)), int64(PACKET_SIZE-BLOCKS_HEADER_SIZE-scionHeaderLen)))
	*b.nextPacketIndex = 0
	*b.processPacketIndex = 0
	log.Infof("Receiving %d packets", len(b.packets))
	// TODO: Waiting queue
	// TODO: sync write calls
	// TODO: Can not put this into struct for whatever reason
	if b.dataConn == nil {
		/*laddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", b.localAddr, b.localStartPort))
		if err != nil {
			log.Fatal("error:", err)
		}*/
		updAdr := &net.UDPAddr{
			IP:   b.packetPacker.LocalAddr.IP,
			Port: b.localStartPort,
		}
		var err error
		b.dataConn, err = net.ListenUDP("udp", updAdr)
		if err != nil {
			log.Fatal("error:", err)
		}
		fmt.Printf("Listen on %s\n", updAdr.String())
	}

	log.Infof("Before call of receive %p", b.nextPacketIndex)
	go func(conn *net.UDPConn) {
		log.Infof("In call of receive %p", b.nextPacketIndex)
		b.receivePackets(conn, b.nextPacketIndex)
	}(b.dataConn)
	log.Infof("After call of receive %p", b.nextPacketIndex)
	log.Infof("After call of missingnums %p", &b.missingSequenceNums)
	/*go func(ctrlCon *net.UDPConn, missingNums *[]int64) {
		log.Infof("In call of retransfers %p", missingNums)
		b.requestRetransfers(ctrlCon, missingNums)
	}(b.ctrlConn, &b.missingSequenceNums)*/

	b.parsePackets(b.nextPacketIndex, &b.missingSequenceNums)
}

func (b *BlocksConn) receivePackets(conn *net.UDPConn, nextPacketIndex *int) {
	// length := len(b.packets)
	i := 0
	j := 0
	ticker := time.NewTicker(1000 * time.Millisecond)
	done := make(chan bool)
	lastNextPacketIndex := *nextPacketIndex
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				bandwidth := (b.receivedBytes - b.lastReceivedBytes) * 8 / 1024 / 1024
				log.Infof("Read with %d Mbit/s having %d received packets", bandwidth, b.receivedPackets)
				b.lastReceivedBytes = b.receivedBytes
			}
		}
	}()
	for i := range b.packets {
		b.packets[i] = make([]byte, PACKET_SIZE+100)
	}
	log.Infof("In Receive nextPacketIndex addr %p", b.nextPacketIndex)
	for b.mode != MODE_DONE && *b.nextPacketIndex < len(b.packets) {
		//if *b.nextPacketIndex >= length {
		//	*b.nextPacketIndex = 0
		//}
		// b.packets[index][i] = make([]byte, PACKET_SIZE+1000)
		// TODO: Check bytes read
		// fmt.Println("READING")
		bts, err := conn.Read(b.packets[*b.nextPacketIndex])
		b.packets[*b.nextPacketIndex] = b.packets[*b.nextPacketIndex][:bts]
		if err != nil {
			log.Fatal("error:", err)
		}
		if j%100000 == 0 {
			j++
			continue
		}

		b.receivedBytes += int64(bts)
		b.receivedPackets++
		lastNextPacketIndex++
		*b.nextPacketIndex++
		j++
		/*if j > 0 && j%1000 == 0 {
			j++
			log.Infof("CONTIENU %d", j)
			continue
		}*/

		// log.Infof("Receiving md5 %x", md5.Sum(buf[:bts]))
		// b.packets[index][i] = b.packets[index][i][:bts] //make([]byte, bts)
		// copy(b.packets[index][i], buf[:bts])
		// fmt.Println(b.packets[index][i])
		// fmt.Printf("READ BYTES %d\n for index %d\n", bts, i)
		i++
	}
	ticker.Stop()
	done <- true
}

func (b *BlocksConn) parsePackets(nextPacketIndex *int, missingNums *[]int64) {
	byteLen := len(b.block)
	// _bytes2 := make([]byte, 0)
	// startSequenceNumber := 0
	var highestSequenceNumber int64 = 0
	// packetIndex := 0
	var payloadLen int64 = 0
	done := false
	var receivedPackets int64 = 0
	ticker := time.NewTicker(1000 * time.Millisecond)
	d := make(chan bool)
	startTime := time.Now()
	go func() {
		for {
			select {
			case <-d:
				return
			case <-ticker.C:
				log.Infof("having %d parsed packets", b.processedPackets)
			}
		}
	}()
	fmt.Println(byteLen)
	p := BlockPacket{}
	log.Infof("MissingSequenceNums Addr %p", missingNums)
	log.Infof("Processed Packet Index %d", *b.processPacketIndex)
	log.Infof("In call parse nextPacketIndex %p, len %d", b.nextPacketIndex, len(b.packets))
	for *b.processPacketIndex < len(b.packets) {
		// log.Infof("Processed Packet Index %d", *b.processPacketIndex)
		for *b.processPacketIndex == *b.nextPacketIndex {
			// fmt.Printf("nextPacketIndex %d and processedPacketIndex %d\n", *b.nextPacketIndex, *b.processPacketIndex)
			time.Sleep(1 * time.Millisecond)
		}
		//retransfer := false
		// log.Infof("Received md5 %x", md5.Sum(b.packets[*b.processPacketIndex]))
		// err := decodePacket(&p, b.packets[*b.processPacketIndex])
		buf := b.packets[*b.processPacketIndex]
		b.packetPacker.Unpack(&buf)
		p.SequenceNumber = (int64(binary.BigEndian.Uint64(buf[0:8])))
		p.BlockId = (int64(binary.BigEndian.Uint64(buf[8:16])))
		p.Payload = buf[16:]
		// log.Infof("Received seq %d and md5 for buf %x", binary.BigEndian.Uint64(buf[0:8]), md5.Sum(buf))
		// fmt.Println(buf[0:8])
		//if err != nil {
		//		log.Fatalf("error decoding", err)
		//	}
		/*if packetIndex > 0 && packetIndex%100 == 0 {
			b.missingSequenceNums[index] = append(b.missingSequenceNums[index], p.SequenceNumber)
			log.Infof("Appending missing sequence number %d", p.SequenceNumber)
		}*/

		diff := p.SequenceNumber - highestSequenceNumber
		if diff > 1 {
			var off int64 = 1
			for off < diff {
				// log.Infof("Append %d", p.SequenceNumber-off)
				b.Lock()
				*missingNums = AppendIfMissing(*missingNums, p.SequenceNumber-off)
				b.Unlock()
				off++
			}
			// log.Infof("Appending missing sequence number %d to %d for highest number %d", p.SequenceNumber-off, p.SequenceNumber, highestSequenceNumber)
		} else if diff < 0 {
			// retransfer = true
			// log.Infof("Received retransferred sequence number %d", p.SequenceNumber)
			b.Lock()
			*missingNums = RemoveFromSlice(*missingNums, p.SequenceNumber)
			b.Unlock()
			if len(*missingNums) == 0 {
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
		//if retransfer {
		//		log.Infof("Writing SequenceNumber %d to bytes index %d", p.SequenceNumber, (p.SequenceNumber-1)*payloadLen)
		//	}
		//
		startIndex := (p.SequenceNumber - 1) * payloadLen
		// copy(_bytes[startIndex:startIndex+curPayloadLen], p.Payload)
		for i, v := range p.Payload {
			b.block[startIndex+int64(i)] = v
		}
		// log.Infof("Received md5 payload %x", md5.Sum(p.Payload))
		// _bytes2 = append(_bytes2, p.Payload...)
		if diff > 0 {
			highestSequenceNumber += diff
		}

		b.lastReceivedSequenceNumber = highestSequenceNumber
		*b.processPacketIndex++
		b.processedPackets++
		receivedPackets++
		if done {
			b.mode = MODE_DONE
			break
		}
	}
	d <- true
	ticker.Stop()
	elapsed := time.Since(startTime)
	log.Printf("Receive took %s for %s ", elapsed, ByteCountSI(int64(len(b.block))))
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

}

func (b *BlocksConn) sendPackets(conn *net.UDPConn) {
	ticker := time.NewTicker(1000 * time.Millisecond)
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				bandwidth := (b.sentBytes - b.lastSentBytes) * 8 / 1024 / 1024
				log.Infof("Read with %d Mbit/s having %d received packets", bandwidth, b.sentPacets)
				b.lastSentBytes = b.sentBytes
			}
		}
	}()

	for i := range b.packets {
		// log.Infof("Sending md5 %x", md5.Sum(b.packets[index][i]))
		bts, err := conn.Write(b.packets[i])
		b.sentPacets += int64(bts)
		b.sentBytes++
		// time.Sleep(1 * time.Microsecond)
		if err != nil {
			log.Fatal("error:", err)
		}
	}
	ticker.Stop()
	done <- true
}

func (b *BlocksConn) collectRetransfers(ctrlCon *net.UDPConn, missingNums *[]int64) {
	/*go func() {
		b.retransferMissingPackets()
	}()*/
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
		for _, v := range p.MissingSequenceNumbers {
			// log.Infof("Add %d to missing sequenceNumbers for client to send them back later", v)
			b.Lock()
			*missingNums = AppendIfMissing(*missingNums, v)
			b.Unlock()
		}
		// b.missingSequenceNums[index] = append(b.missingSequenceNums[index], p.MissingSequenceNumbers...)
		// log.Infof("Added %d sequenceNumbers to missingSequenceNumbers", len(p.MissingSequenceNumbers))
	}

}

func (b *BlocksConn) requestRetransfers(ctrlCon *net.UDPConn, missingNums *[]int64) {
	ticker := time.NewTicker(1000 * time.Millisecond)
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
					min := Min(start+missingNumsPerPacket, len(*missingNums))
					var network bytes.Buffer        // Stand-in for a network connection
					enc := gob.NewEncoder(&network) // Will write to network.
					p := BlockRequestPacket{
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
}

func decodeReqPacket(p *BlockRequestPacket, buf []byte) error {
	network := bytes.NewBuffer(buf)
	dec := gob.NewDecoder(network)
	return dec.Decode(&p)
}

func decodePacket(p *BlockPacket, buf []byte) error {
	network := bytes.NewBuffer(buf)
	dec := gob.NewDecoder(network)
	return dec.Decode(&p)
}

func encodePacket(p *BlockPacket, buf []byte) error {
	var network bytes.Buffer // Stand-in for a network connection
	enc := gob.NewEncoder(&network)

	err := enc.Encode(*p)
	// err := binary.Write(&network, binary.BigEndian, p)
	if err != nil {
		return err
	}
	copy(buf, network.Bytes())
	return nil
}
