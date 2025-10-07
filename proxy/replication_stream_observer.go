package proxy

import (
	"context"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.temporal.io/server/common/log/tag"
)

// ReplicationStreamObserver maintains a count of the active streams. It can be Start-ed to create a thread that automatically
// logs to the provided logger.
type ReplicationStreamObserver struct {
	streamActive   []atomic.Int32
	streamGrowLock sync.Mutex
	logger         loggable
	interval       time.Duration
}

// allow unit tests to test the log output
type loggable interface {
	Warn(msg string, tags ...tag.Tag)
	Info(msg string, tags ...tag.Tag)
}

func NewReplicationStreamObserver(logger loggable) *ReplicationStreamObserver {
	return &ReplicationStreamObserver{
		// 1024 covers most cases without growing. If a larger history count is logged, Notify* will grow the array
		streamActive:   make([]atomic.Int32, 1024),
		streamGrowLock: sync.Mutex{},
		interval:       time.Minute,
		logger:         logger,
	}
}
func (s *ReplicationStreamObserver) ReportStreamValue(idx int32, value int32) {
	if idx < 0 {
		s.logger.Warn("ReplicationStreamObserver NotifyConnect called with negative streamIndex")
		return
	}
	s.streamGrowLock.Lock()
	// We want to grow the minimum number of times, so
	if idx >= int32(len(s.streamActive)) {
		// Each index will be uniformly random in the range [0, maxStreams). Growing by a percentage of index helps
		// minimize the amount of reallocation required. Starting with increasing to 125% of idx to keep memory waste low
		newSize := min(int((idx+1)*9), math.MaxInt32) / 8
		// grow and maximize
		s.streamActive = slices.Grow(s.streamActive, newSize)[:newSize]
	}
	s.streamActive[idx].Add(value)
	s.streamGrowLock.Unlock()
}
func (s *ReplicationStreamObserver) PrintActiveStreams() string {
	sb := strings.Builder{}
	sb.WriteString("[")
	s.streamGrowLock.Lock()
	for i := range s.streamActive {
		if s.streamActive[i].Load() != 0 {
			sb.WriteString(strconv.Itoa(i))
			sb.WriteString(",")
		}
	}
	s.streamGrowLock.Unlock()
	sb.WriteString("]")
	return sb.String()
}
func (s *ReplicationStreamObserver) Start(lifetime context.Context, name string, direction string) {
	go func() {
		for lifetime.Err() == nil {
			s.logger.Info("Logging active AdminService history streams", tag.NewStringTag("direction", direction), tag.NewStringTag("name", name),
				tag.NewStringTag("activeStreams", s.PrintActiveStreams()))
			select {
			case <-time.After(s.interval):
			case <-lifetime.Done():
			}
		}
	}()
}
