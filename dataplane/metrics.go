package dataplane

import (
	"time"
)

type SocketMetrics struct {
	RxBytes   uint64
	TxBytes   uint64
	RxPackets uint64
	TxPackets uint64
}

type Metrics struct {
	RxBytes             uint64
	TxBytes             uint64
	RxPackets           uint64
	TxPackets           uint64
	RxBytesLast         uint64
	TxBytesLast         uint64
	RxPacketsLast       uint64
	TxPacketsLast       uint64
	RxBandwidth         uint64
	TxBandwidth         uint64
	RxBytesOverTime     []uint64
	TxBytesOverTime     []uint64
	RxBandwidthOverTime []uint64
	TxBandwidthOverTime []uint64
	RxPacketsOverTime   []uint64
	TxPacketsOverTime   []uint64
	timeInterval        int
	signalChan          chan bool
}

func NewMetrics(timeInterval int) *Metrics {
	m := Metrics{
		timeInterval:        timeInterval,
		signalChan:          make(chan bool),
		RxBytesOverTime:     make([]uint64, 0),
		TxBytesOverTime:     make([]uint64, 0),
		RxBandwidthOverTime: make([]uint64, 0),
		TxBandwidthOverTime: make([]uint64, 0),
		RxPacketsOverTime:   make([]uint64, 0),
		TxPacketsOverTime:   make([]uint64, 0),
	}
	return &m
}

// TODO: Avoid redundand measurements on duplicate call
func (m *Metrics) Collect() {
	go func() {
		for {
			select {
			case res := <-m.signalChan:
				if res {
					return
				}
			case <-time.After(time.Duration(m.timeInterval) * time.Millisecond):
				m.RxBandwidth = (m.RxBytes - m.RxBytesLast)
				m.TxBandwidth = (m.TxBytes - m.TxBytesLast)
				m.RxBandwidthOverTime = append(m.RxBandwidthOverTime, m.RxBandwidth)
				m.TxBandwidthOverTime = append(m.TxBandwidthOverTime, m.TxBandwidth)

				m.RxPacketsOverTime = append(m.RxPacketsOverTime, m.RxPackets)
				m.TxPacketsOverTime = append(m.TxPacketsOverTime, m.TxPackets)

				m.RxBytesOverTime = append(m.RxBytesOverTime, m.RxBytes)
				m.TxBytesOverTime = append(m.TxBytesOverTime, m.TxBytes)

				m.RxBytesLast = m.RxBytes
				m.TxBytesLast = m.TxBytes
			}
		}
	}()
}

func (m *Metrics) Stop() {
	m.signalChan <- true
}
