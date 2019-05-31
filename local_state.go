package p2p

import (
	"github.com/DSiSc/repository"
	"sync/atomic"
)

var localState atomic.Value

func init() {
	localState.Store(uint64(0))
}

// LocalState get local current state
func LocalState() uint64 {
	bc, err := repository.NewLatestStateRepository()
	if err != nil {
		return localState.Load().(uint64)
	}
	currentHeight := bc.GetCurrentBlockHeight()
	localState.Store(currentHeight)
	return currentHeight
}
