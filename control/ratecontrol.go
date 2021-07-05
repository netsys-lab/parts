package control

import (
	"sync"
	"time"
)

const (
	MAX_SLOW_START_ITERATIONS = 3
)

type RateControl struct {
	sync.Mutex
	TimeInterval             time.Duration
	LastIntervalPackets      int64
	LastIntervalBytes        int64
	MaxSpeed                 int64 // Always bits/s
	AveragePacketWaitingTime *time.Duration
	LastIntervalWaitingTime  *time.Duration
	Ticker                   *time.Ticker
	CtrlChan                 chan bool
	LastPacketTime           time.Time
	SlowStartCount           int
}

func NewRateControl(
	timeInterval time.Duration,
	maxSpeed int64,
) *RateControl {
	rc := RateControl{
		TimeInterval:   timeInterval,
		MaxSpeed:       maxSpeed,
		CtrlChan:       make(chan bool, 0),
		SlowStartCount: 0, // The number of iterations until slow start is done
	}

	return &rc
}

func (rc *RateControl) Add(numPackets int, numBytes int64) {
	rc.LastIntervalBytes = numBytes
	rc.LastIntervalPackets += int64(numPackets)
	/*if rc.LastIntervalWaitingTime != nil {
		*rc.LastIntervalWaitingTime += time.Since(rc.LastPacketTime)
	}

	rc.LastPacketTime = time.Now()*/
	if rc.AveragePacketWaitingTime != nil {
		time.Sleep(*rc.AveragePacketWaitingTime)
	}

}

func (rc *RateControl) Start() {
	rc.Ticker = time.NewTicker(rc.TimeInterval)
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
}

func (rc *RateControl) recalculateRate() {
	rc.Lock()
	// Calclulate AveragePacketWaitingTime here...
	// maxSpeed per second: 1.000.000.000 for 1gbit/s
	// maxSpeed per millisecond 1.000.000 for 1Gbit/s
	// maxSpeed in bytes/millisecond = 1.000.000 / 8 125000
	// timeInterval: 1000 milliseconds
	// numBytes in 1000 milliseconds
	bytesPerMilliSecond := rc.MaxSpeed / int64(time.Millisecond) / 8
	targetBytes := bytesPerMilliSecond * int64(rc.TimeInterval)
	packetSize := rc.LastIntervalBytes / rc.LastIntervalPackets
	targetPackets := targetBytes / packetSize
	timeForPacket := int64(rc.TimeInterval) * int64(time.Millisecond) / rc.LastIntervalPackets
	maxTimeForPacket := int64(rc.TimeInterval) * int64(time.Millisecond) / int64(targetPackets)

	// First try with 33% sending speed
	if rc.SlowStartCount < MAX_SLOW_START_ITERATIONS {
		dur := time.Duration(maxTimeForPacket - int64(maxTimeForPacket)/int64(2+rc.SlowStartCount))
		rc.AveragePacketWaitingTime = &dur
		rc.SlowStartCount++
	} else { // Probably here we have max speed information from receiver
		// Faster than maxSpeed, reduce speed
		if timeForPacket < maxTimeForPacket {
			dur := time.Duration(maxTimeForPacket - timeForPacket)
			rc.AveragePacketWaitingTime = &dur
		} else {
			// If slower than maxSpeed (assumed that maxSpeed is really a useful maximum)
			// There is nothing we can do
		}
	}

	rc.LastIntervalBytes = 0
	rc.LastIntervalPackets = 0
	rc.Unlock()
}

func (rc *RateControl) Stop() {
	rc.Ticker.Stop()
	rc.CtrlChan <- false
}
