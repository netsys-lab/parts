package main

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

func IndexOf(element int64, data []int64) int {
	for k, v := range data {
		if element == v {
			return k
		}
	}
	return -1 //not found.
}

func RemoveFromSlice(s []int64, i int64) []int64 {
	index := IndexOf(i, s)
	if index == -1 {
		log.Infof("Received index -1 for val %d", i)
		return s
	}
	s[index] = s[len(s)-1]
	return s[:len(s)-1]
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

func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}
