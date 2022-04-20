package controlplane

import (
	"sync"
	"time"

	"github.com/netsys-lab/parts/dataplane"
	"github.com/netsys-lab/parts/utils"
	log "github.com/sirupsen/logrus"
)

// #include <stdio.h>
// #include <time.h>
// int sleep(int ns)
//{
//   struct timespec tim, tim2;
//   tim.tv_sec = 0;
//   tim.tv_nsec = ns;
// if(nanosleep(&tim , &tim2) < 0 )
//   {
//      return -1;
//   }
//   return 0;
//}
// import "C"

const (
	MAX_SLOW_START_ITERATIONS     = 3
	START_CONGESTION_WINDOW       = 100
	PACKET_WAITING_TIME           = 200000 // 200000
	CONGESTION_WINDOW_MAX         = 150000
	PACKET_LOSS_THRESHOLD_PERCENT = 5 // TODO: Something is with this number
	RC_STATE_STARTUP              = 1
	RC_STATE_DRAIN                = 2
	RC_STATE_PROBE                = 3
	RC_STATE_IDLE                 = 4
	DRAIN_INCREASE_WINDOW         = 20
)

type RateControl struct {
	sync.Mutex
	congestionWindow        int
	state                   int
	lastIntervalBytes       int64
	lastIntervalPackets     int64
	lastPacketTxIndex       int
	packetLossPercent       int64 // between 1 and 100
	windowCompletionTime    time.Duration
	lastMissingSequenceNums int64
	curNumMissingPackets    int64
	curNumCheckedPackets    int64
	lastNumMissingPackets   int64
	drainIncreaseCount      int64
	drainLossCount          int64
	probeIncreaseCount      int64
	partContext             *dataplane.PartContext
}

func NewRateControl() *RateControl {
	rc := RateControl{
		state:                RC_STATE_IDLE,
		congestionWindow:     START_CONGESTION_WINDOW,
		windowCompletionTime: PACKET_WAITING_TIME,
	}

	return &rc
}

func (rc *RateControl) SetPartContext(p *dataplane.PartContext) {
	rc.partContext = p
}

// This happens every 5-10 milliseconds, so we ensure its a more or less
// fix window size, at least for larger parts
func (rc *RateControl) AddAckMessage(msg *PartAckPacket) {
	rc.Lock()
	// Check if an Ack is missing
	// if (rc.lastPacketTxIndex + 1) == msg.RequestSequenceId {
	//if rc.lastIntervalPackets > 0 {
	// Packet x/x
	rc.curNumMissingPackets += utils.Sum(msg.MissingSequenceNumberOffsets)
	// log.Debugf("Got packet index %d of %d", msg.PacketTxIndex, msg.NumPacketsPerTx)
	if msg.PacketTxIndex >= (msg.NumPacketsPerTx - 1) {
		missingPackets := rc.curNumMissingPackets - rc.lastNumMissingPackets
		// rc.packetLossPercent = int64(len(msg.MissingSequenceNumbers)) - rc.lastMissingSequenceNums // TODO: Correct size // ((rc.lastIntervalPackets - int64(msg.NumPacketsPerTx)) * 100) / rc.lastIntervalPackets
		// Avoid integer divde by zero
		if msg.NumPackets != rc.curNumCheckedPackets {
			rc.packetLossPercent = (missingPackets * 100) / (msg.NumPackets - rc.curNumCheckedPackets)
		}

		if rc.packetLossPercent > 0 {
			log.Debugf("PartId %d: Calculating missPackets %d * 100 / %d - %d ", rc.partContext.PartId, missingPackets, msg.NumPackets, rc.curNumCheckedPackets)
			log.Debugf("PartId %d: Got %d for packetloss", rc.partContext.PartId, rc.packetLossPercent)
		}
		rc.lastNumMissingPackets = rc.curNumMissingPackets
		rc.curNumMissingPackets = 0
		rc.curNumCheckedPackets = msg.NumPackets

		switch rc.state {
		case RC_STATE_PROBE:
			if rc.packetLossPercent >= PACKET_LOSS_THRESHOLD_PERCENT {
				log.Debugf("Back to drain phase")
				rc.congestionWindow = rc.congestionWindow - rc.congestionWindow/4
				rc.state = RC_STATE_DRAIN
				rc.probeIncreaseCount = 0
			} else {
				rc.probeIncreaseCount += 1
				if rc.probeIncreaseCount%DRAIN_INCREASE_WINDOW == 0 {
					rc.congestionWindow = rc.congestionWindow + rc.congestionWindow/5
					if rc.congestionWindow > CONGESTION_WINDOW_MAX {
						rc.congestionWindow = CONGESTION_WINDOW_MAX
						break
					}
					log.Debugf("Increasing congestion window in probe phase")
				}

			}
			break
		case RC_STATE_DRAIN:
			rc.drainLossCount += 1
			// We know there is packet loss (we already decreased the window and we wait a bit)
			if rc.drainLossCount > 1 && rc.drainLossCount <= 5 {
				break
			}
			if rc.packetLossPercent >= PACKET_LOSS_THRESHOLD_PERCENT {
				rc.congestionWindow = rc.congestionWindow - rc.congestionWindow/3
				log.Debugf("Decreasing window in drain phase to  %d", rc.congestionWindow)
				rc.drainIncreaseCount = 0
			} else {
				rc.drainIncreaseCount += 1
				// no packet loss, increase window
				if rc.drainIncreaseCount%DRAIN_INCREASE_WINDOW == 0 {
					rc.state = RC_STATE_PROBE
					log.Debugf("Going to probe phase")

				}
			}
			break
		case RC_STATE_STARTUP:
			if rc.packetLossPercent >= PACKET_LOSS_THRESHOLD_PERCENT {
				// Go to drain phase, remove congestionwindow
				rc.state = RC_STATE_DRAIN
				if rc.congestionWindow <= CONGESTION_WINDOW_MAX {
					rc.congestionWindow = rc.congestionWindow - rc.congestionWindow/2
				} else {

				}
				log.Debugf("Moving to drain phase %d", rc.congestionWindow)
			} else {
				rc.congestionWindow += rc.congestionWindow / 3
				if rc.congestionWindow > CONGESTION_WINDOW_MAX {
					rc.congestionWindow = CONGESTION_WINDOW_MAX
					break
				}
				log.Debugf("Increasing congestionWindow to %d", rc.congestionWindow)
			}

			break
		}
	}

	rc.lastMissingSequenceNums = int64(len(msg.MissingSequenceNumbers))
	rc.lastPacketTxIndex = msg.RequestSequenceId
	rc.lastIntervalBytes = 0
	rc.lastIntervalPackets = 0
	rc.Unlock()
}

func (rc *RateControl) Recalculate(msg *dataplane.PartRequestPacket) {

}

func (rc *RateControl) Add(numPackets int, numBytes int64) {
	rc.lastIntervalBytes = numBytes
	rc.lastIntervalPackets += int64(numPackets)

	// log.Infof("Sleeping")
	// log.Infof("Wait for %d", rc.AveragePacketWaitingTime)
	if rc.lastIntervalPackets%int64(rc.congestionWindow) == 0 && rc.windowCompletionTime != 0 { // TODO: Check this constant
		time.Sleep(rc.windowCompletionTime)
		// log.Printf("Sleeping")
		// log.Infof("Sleeping")
		// TODO: ENable rate control
		// C.sleep(C.int(rc.AveragePacketWaitingTime * 100))
	}

}

func (rc *RateControl) Start() {
	rc.Lock()
	rc.state = RC_STATE_STARTUP
	rc.Unlock()
	/*if rc.AveragePacketWaitingTime == 0 && !rc.IsServer {
		// this is a bit hacky, but for now it should work
		switch rc.NumCons {
		case 1:
			rc.AveragePacketWaitingTime = 1000
			rc.DecreaseWaitingTime = 300
			break
		case 2:
			rc.AveragePacketWaitingTime = 2000
			rc.DecreaseWaitingTime = 200
			break
		case 3:
			rc.AveragePacketWaitingTime = 4000
			rc.DecreaseWaitingTime = 300
			break
		case 4:
			rc.AveragePacketWaitingTime = 6000
			rc.DecreaseWaitingTime = 500
			break
		case 5:
			rc.AveragePacketWaitingTime = 10000
			rc.DecreaseWaitingTime = 300
			break
		case 6:
			rc.AveragePacketWaitingTime = 11000
			rc.DecreaseWaitingTime = 300
			break
		case 7:
			rc.AveragePacketWaitingTime = 12000
			rc.DecreaseWaitingTime = 300
			break
		case 8:
			rc.AveragePacketWaitingTime = 14000
			rc.DecreaseWaitingTime = 300
			break
		case 9:
			rc.AveragePacketWaitingTime = 17000
			rc.DecreaseWaitingTime = 300
			break
		case 10:
			rc.AveragePacketWaitingTime = 20000
			rc.DecreaseWaitingTime = 300
			break
		}
		// rc.AveragePacketWaitingTime = time.Duration(utils.Max64(int64(time.Duration(rc.NumCons*2000)), 8000)) // TODO: Validate
		// rc.DecreaseWaitingTime = 500
	}
	rc.FirstPacket = false
	/*rc.Ticker = time.NewTicker(rc.TimeInterval * time.Millisecond)
	rc.recalculateRate()
	go func() {
		for {
			select {
			case <-rc.CtrlChan:
				return
			case <-rc.Ticker.C:
				rc.recalculateRate()
			}
		}
	}()
	rc.Running = true
	*/
}

func (rc *RateControl) recalculateRate() {
	/*rc.Lock()

	if rc.MaxSpeed == 0 {
		return
	}

	bytesPerSecond := rc.MaxSpeed / 8
	targetPacketPerSecond := bytesPerSecond / int64(rc.PacketSize)
	// fmt.Printf("Packets Per Second %d, maxSpeed %d\n", targetPacketPerSecond, rc.MaxSpeed)
	divider := float64(rc.TimeInterval) / float64(1000)
	targetPacketsPerTimeInterval := int64(float64(targetPacketPerSecond) * divider)
	timeIntervalNs := time.Duration(rc.TimeInterval) * time.Millisecond
	// fmt.Printf("Packets Per TimeInterval %d\n", targetPacketsPerTimeInterval)
	nsPerPacket := timeIntervalNs / time.Duration(targetPacketsPerTimeInterval)
	rc.AveragePacketWaitingTime = nsPerPacket / 3 // Assumption: around 2/3 time needed to send packet
	// log.Infof("Got %d AveragePacketWaitingTime per packet, nsPerPacket %d", rc.AveragePacketWaitingTime, nsPerPacket)
	// bytesPerSecond := rc.MaxSpeed / 8
	// targetBytes := bytesPerSecond * int64(rc.TimeInterval)
	// targetPacketPerSecond := bytesPerSecond / int64(rc.PacketSize)
	// maxTimeForPacket := int64(rc.TimeInterval) / int64(targetPacketsPerMillisecond)

	// Calclulate AveragePacketWaitingTime here...
	// maxSpeed per second: 1.000.000.000 for 1gbit/s
	// maxSpeed per millisecond 1.000.000 for 1Gbit/s
	// maxSpeed in bytes/millisecond = 1.000.000 / 8 125000
	// timeInterval: 1000 milliseconds
	// numBytes in 1000 milliseconds
	/*
		bytesPerMilliSecond := rc.MaxSpeed / int64(time.Millisecond) / 8
		targetBytes := bytesPerMilliSecond * int64(rc.TimeInterval)
		targetPacketsPerMillisecond := targetBytes / int64(rc.PacketSize)
		maxTimeForPacket := int64(rc.TimeInterval) / int64(targetPacketsPerMillisecond)
		log.Infof("RC: Having maxSpeed %d, bytesPerMilliSecond %d, targetBytes %d, targetPackets %d, maxTimeForPacket %d", rc.MaxSpeed, bytesPerMilliSecond, targetBytes, targetPacketsPerMillisecond, maxTimeForPacket)
		if rc.LastIntervalBytes == 0 {
			if rc.SlowStartCount < MAX_SLOW_START_ITERATIONS {
				dur := time.Duration(maxTimeForPacket - int64(maxTimeForPacket)/int64(2+rc.SlowStartCount))
				rc.AveragePacketWaitingTime = &dur
				log.Infof("Calculated AveragePacketWaitingTime %s, maxTimePerPacket %d", dur, maxTimeForPacket)
				rc.SlowStartCount++
			}
		} else {
			if rc.SlowStartCount < MAX_SLOW_START_ITERATIONS {
				dur := time.Duration(maxTimeForPacket - int64(maxTimeForPacket)/int64(2+rc.SlowStartCount))
				rc.AveragePacketWaitingTime = &dur
				log.Infof("Calculated AveragePacketWaitingTime %s, maxTimePerPacket %d", dur, maxTimeForPacket)
				rc.SlowStartCount++
			} else {
				timeForPacket := int64(rc.TimeInterval) * int64(time.Millisecond) / rc.LastIntervalPackets

				// Probably here we have max speed information from receiver
				// Faster than maxSpeed, reduce speed
				if timeForPacket < maxTimeForPacket {
					dur := time.Duration(maxTimeForPacket - timeForPacket)
					rc.AveragePacketWaitingTime = &dur
				} else {
					// If slower than maxSpeed (assumed that maxSpeed is really a useful maximum)
					// There is nothing we can do
				}

			}
		}
	rc.LastIntervalBytes = 0
	rc.LastIntervalPackets = 0
	rc.Unlock()
	*/
}

func (rc *RateControl) Stop() {
	//rc.Ticker.Stop()
	///rc.CtrlChan <- false
	rc.Lock()
	rc.state = RC_STATE_IDLE
	rc.Unlock()
}
