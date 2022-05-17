package shared

import (
	"math/rand"
)

var AppId int64

func init() {
	AppId = int64(rand.Uint64())
}
