package harness

import (
	"os"
	"syscall"
)

type osSignal = os.Signal

var osInterrupt os.Signal = syscall.SIGINT
