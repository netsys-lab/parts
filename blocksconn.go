package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
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
	}()*/
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
	}
}

func (b *BlocksConn) WriteBlock(block []byte, blockId int64) {
	// TODO: Save activeBlockCount and increase immediatly
	// TODO: Not overwrite if actually sending
	blockContext := socket.BlockContext{}
	blockContext.Data = block
	blockContext.Prepare()
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
	blockContext := socket.BlockContext{}
	blockContext.Prepare()
	// TODO: This assumption here is bullshit, we need to read block size from first packet of block id
	// TODO: How to ensure order of parallel sent blocks? Increasing blockIds?
	log.Infof("Receiving %d packets", blockContext.NumPackets)
	// TODO: Waiting queue
	// TODO: sync write calls
	// TODO: Can not put this into struct for whatever reason
	blockContext.Data = block
	b.TransportSocket.ReadBlock(&blockContext)

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
	/*go func() {
		for {
			select {
			case <-d:
				return
			case <-ticker.C:
				log.Infof("having %d parsed packets", b.processedPackets)
			}
		}
	}()*/
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
		// b.processedPackets++
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
	/*go func() {
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
	}()*/

	for i := range b.packets {
		// log.Infof("Sending md5 %x", md5.Sum(b.packets[index][i]))
		bts, err := conn.Write(b.packets[i])
		b.Metrics.TxBytes += uint64(bts)
		b.Metrics.TxPackets += 1
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
