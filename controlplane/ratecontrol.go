package controlplane

import (
	"sync"
	"time"

	"github.com/martenwallewein/parts/dataplane"
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
	MAX_SLOW_START_ITERATIONS = 3
)

type RateControl struct {
	sync.Mutex
	TimeInterval                  time.Duration
	LastIntervalPackets           int64
	LastIntervalBytes             int64
	MaxSpeed                      int64 // Always bits/s
	AveragePacketWaitingTime      time.Duration
	LastIntervalWaitingTime       *time.Duration
	Ticker                        *time.Ticker
	CtrlChan                      chan bool
	LastPacketTime                time.Time
	SlowStartCount                int
	PacketSize                    int
	DecreaseWaitingTime           int
	LimitReached                  bool
	Running                       bool
	NumReceivedMissingNumbers     int
	LastNumReceivedMissingNumbers int
	LastTx                        int64
	NumPacketsPerTx               int
	NumCons                       int
	FirstPacketTime               time.Time
	FirstPacket                   bool
	IsServer                      bool
}

func NewRateControl(
	timeInterval time.Duration,
	maxSpeed int64,
	packetSize int,
	numCons int,
) *RateControl {
	rc := RateControl{
		TimeInterval:   timeInterval,
		MaxSpeed:       maxSpeed,
		CtrlChan:       make(chan bool, 0),
		SlowStartCount: 0, // The number of iterations until slow start is done
		PacketSize:     packetSize,
		NumCons:        numCons,
	}

	return &rc
}

func (rc *RateControl) AddAckMessage(msg dataplane.PartRequestPacket) {
	// log.Info(msg)
	if !rc.Running {
		return
	}

	if rc.LastTx == msg.TransactionId {
		if msg.PacketTxIndex == 0 {
			rc.NumReceivedMissingNumbers = len(msg.MissingSequenceNumbers)
		} else {
			rc.NumReceivedMissingNumbers += len(msg.MissingSequenceNumbers)
		}

		// Last packet per tx means we have all infos available
		if msg.PacketTxIndex >= msg.NumPacketsPerTx-1 {
			rc.Recalculate(&msg)
			rc.LastNumReceivedMissingNumbers = rc.NumReceivedMissingNumbers
		}
	} else { // New Transaction
		rc.LastTx = msg.TransactionId
		rc.NumReceivedMissingNumbers = len(msg.MissingSequenceNumbers)
	}
}

func (rc *RateControl) Recalculate(msg *dataplane.PartRequestPacket) {
	// Decrease
	if (rc.NumReceivedMissingNumbers - 10) > rc.LastNumReceivedMissingNumbers {
		// log.Infof("Run into packet loss, reducing speed")
		rc.LimitReached = true
		rc.AveragePacketWaitingTime += time.Duration(rc.DecreaseWaitingTime)
	} else if rc.NumReceivedMissingNumbers <= 10 && !rc.LimitReached { // Increase
		if rc.DecreaseWaitingTime > int(rc.AveragePacketWaitingTime) {
			// log.Infof("Having max speed, no rate limit enforced")
			rc.AveragePacketWaitingTime = 0
		} else {
			rc.AveragePacketWaitingTime -= time.Duration(rc.DecreaseWaitingTime)
			// log.Infof("Increasing waiting time speed to %d", rc.AveragePacketWaitingTime)
		}
	} else {
		// log.Infof("Achieved max speed, not rate limit enforced")

	}
}

func (rc *RateControl) Add(numPackets int, numBytes int64) {
	rc.LastIntervalBytes = numBytes
	rc.LastIntervalPackets += int64(numPackets)

	if !rc.FirstPacket {
		rc.FirstPacket = true
		rc.FirstPacketTime = time.Now()
		// log.Infof("setting rc time %s", rc.FirstPacketTime)
	}

	// if rc.MaxSpeed == 0 {
	//	return
	// }
	/*if rc.LastIntervalWaitingTime != nil {
		*rc.LastIntervalWaitingTime += time.Since(rc.LastPacketTime)
	}

	if rc.L

	rc.LastPacketTime = time.Now()*/
	// log.Infof("Sleeping")
	// log.Infof("Wait for %d", rc.AveragePacketWaitingTime)
	if rc.LastIntervalPackets%100 == 0 && rc.AveragePacketWaitingTime != 0 {
		time.Sleep(rc.AveragePacketWaitingTime * 100)
		// log.Infof("Sleeping")
		// TODO: ENable rate control
		// C.sleep(C.int(rc.AveragePacketWaitingTime * 100))
	}

}

func (rc *RateControl) Start() {
	if rc.AveragePacketWaitingTime == 0 && !rc.IsServer {
		// this is a bit hacky, but for now it should work
		switch rc.NumCons {
		case 1:
			rc.AveragePacketWaitingTime = 1000
			rc.DecreaseWaitingTime = 100
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
	}()*/
	rc.Running = true
}

func (rc *RateControl) recalculateRate() {
	rc.Lock()

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
		}*/
	rc.LastIntervalBytes = 0
	rc.LastIntervalPackets = 0
	rc.Unlock()
}

func (rc *RateControl) Stop() {
	//rc.Ticker.Stop()
	///rc.CtrlChan <- false
	rc.Running = false
}
