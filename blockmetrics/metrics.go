package blockmetrics

import (
	"time"
)

type SocketMetrics struct {
	RxBytes   uint64
	TxBytes   uint64
	RxPackets uint64
	TxPackets uint64
}

type GetMetricFunc func(index int) (uint64, uint64, uint64, uint64)

type Metrics struct {
	RxBytes                      uint64
	TxBytes                      uint64
	RxBandwidth                  uint64
	TxBandwidth                  uint64
	RxPackets                    uint64
	TxPackets                    uint64
	RxBytesPerSocket             []uint64
	TxBytesPerSocket             []uint64
	RxBandwidthPerSocket         []uint64
	TxBandwidthPerSocket         []uint64
	RxPacketsPerSocket           []uint64
	TxPacketsPerSocket           []uint64
	RxBytesOverTime              []uint64
	TxBytesOverTime              []uint64
	RxBandwidthOverTime          []uint64
	TxBandwidthOverTime          []uint64
	RxPacketsOverTime            []uint64
	TxPacketsOverTime            []uint64
	RxBytesPerSocketOverTime     [][]uint64
	TxBytesPerSocketOverTime     [][]uint64
	RxBandwidthPerSocketOverTime [][]uint64
	TxBandwidthPerSocketOverTime [][]uint64
	RxPacketsPerSocketOverTime   [][]uint64
	TxPacketsPerSocketOverTime   [][]uint64
	timeInterval                 int
	signalChan                   chan bool
	getMetricFunc                GetMetricFunc
	numSockets                   int
}

func NewMetrics(timeInterval, numSockets int, getMetricFunc GetMetricFunc) *Metrics {
	m := Metrics{
		timeInterval:                 timeInterval,
		getMetricFunc:                getMetricFunc,
		signalChan:                   make(chan bool),
		numSockets:                   numSockets,
		RxBytesPerSocket:             make([]uint64, numSockets),
		TxBytesPerSocket:             make([]uint64, numSockets),
		RxBandwidthPerSocket:         make([]uint64, numSockets),
		TxBandwidthPerSocket:         make([]uint64, numSockets),
		RxPacketsPerSocket:           make([]uint64, numSockets),
		TxPacketsPerSocket:           make([]uint64, numSockets),
		RxBytesPerSocketOverTime:     make([][]uint64, numSockets),
		TxBytesPerSocketOverTime:     make([][]uint64, numSockets),
		RxBandwidthPerSocketOverTime: make([][]uint64, numSockets),
		TxBandwidthPerSocketOverTime: make([][]uint64, numSockets),
		RxPacketsPerSocketOverTime:   make([][]uint64, numSockets),
		TxPacketsPerSocketOverTime:   make([][]uint64, numSockets),
	}

	for i := 0; i < numSockets; i++ {
		m.RxBytesPerSocketOverTime[i] = make([]uint64, numSockets)
		m.TxBytesPerSocketOverTime[i] = make([]uint64, numSockets)
		m.RxBandwidthPerSocketOverTime[i] = make([]uint64, numSockets)
		m.TxBandwidthPerSocketOverTime[i] = make([]uint64, numSockets)
		m.RxPacketsPerSocketOverTime[i] = make([]uint64, numSockets)
		m.TxPacketsPerSocketOverTime[i] = make([]uint64, numSockets)
	}
	return &m
}

// TODO: Avoid redundand measurements on duplicate call
func (m *Metrics) Collect() {
	go func() {
		for {
			select {
			case res := <-m.signalChan:
				if res == true {
					return
				}
			case <-time.After(time.Duration(m.timeInterval) * time.Millisecond):
				var cTxBytes uint64 = 0
				var cRxBytes uint64 = 0
				var cRxPackets uint64 = 0
				var cTxPackets uint64 = 0
				for i := 0; i < m.numSockets; i++ {
					txBytes, rxBytes, txPackets, rxPackets := m.getMetricFunc(i)

					m.RxBandwidthPerSocket[i] = rxBytes - m.RxBandwidthPerSocket[i]
					m.TxBandwidthPerSocket[i] = txBytes - m.TxBandwidthPerSocket[i]
					m.RxBandwidthPerSocketOverTime[i] = append(m.RxBandwidthPerSocketOverTime[i], m.RxBandwidthPerSocket[i])
					m.TxBandwidthPerSocketOverTime[i] = append(m.TxBandwidthPerSocketOverTime[i], m.TxBandwidthPerSocket[i])

					cRxBytes += rxBytes
					cTxBytes += txBytes
					cRxPackets += rxPackets
					cTxPackets += txPackets

					m.RxBytesPerSocket[i] += rxBytes
					m.TxBytesPerSocket[i] += txBytes
					m.RxPacketsPerSocket[i] += rxPackets
					m.TxPacketsPerSocket[i] += txPackets

					m.RxBytesPerSocketOverTime[i] = append(m.RxBytesPerSocketOverTime[i], m.RxBytesPerSocket[i])
					m.TxBytesPerSocketOverTime[i] = append(m.TxBytesPerSocketOverTime[i], m.TxBytesPerSocket[i])
					m.RxPacketsPerSocketOverTime[i] = append(m.RxPacketsPerSocketOverTime[i], m.RxPacketsPerSocket[i])
					m.TxPacketsPerSocketOverTime[i] = append(m.TxPacketsPerSocketOverTime[i], m.TxPacketsPerSocket[i])
				}

				m.RxBandwidth = (cRxBytes - m.RxBytes)
				m.TxBandwidth = (cTxBytes - m.TxBytes)
				m.RxBandwidthOverTime = append(m.RxBandwidthOverTime, m.RxBandwidth)
				m.TxBandwidthOverTime = append(m.TxBandwidthOverTime, m.TxBandwidth)
				m.RxBytes += cRxBytes
				m.TxBytes += cTxBytes
				m.RxPackets += cRxPackets
				m.TxPackets += cTxPackets

				m.RxPacketsOverTime = append(m.RxPacketsOverTime, m.RxPackets)
				m.TxPacketsOverTime = append(m.TxPacketsOverTime, m.TxPackets)

				m.RxBytesOverTime = append(m.RxBytesOverTime, m.RxBytes)
				m.TxBytesOverTime = append(m.TxBytesOverTime, m.TxBytes)
			}
		}
	}()
}

func (m *Metrics) Stop() {
	m.signalChan <- true
}
