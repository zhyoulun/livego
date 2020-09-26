package utils

import (
	"fmt"
	"sync/atomic"
)

var sessionID int64 = 0

func GenSessionIDString() string {
	return fmt.Sprintf("%d", atomic.AddInt64(&sessionID, 1))
}
