package link

import (
	"fmt"
	"sync"
	"time"
)

const (
	snowflakeNodeBits     = 10
	snowflakeSequenceBits = 12

	snowflakeMaxNode     = int64(-1) ^ (int64(-1) << snowflakeNodeBits)
	snowflakeMaxSequence = int64(-1) ^ (int64(-1) << snowflakeSequenceBits)

	snowflakeNodeShift      = snowflakeSequenceBits
	snowflakeTimestampShift = snowflakeSequenceBits + snowflakeNodeBits
)

var snowflakeEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type Snowflake struct {
	mu        sync.Mutex
	nodeID    int64
	lastMs    int64
	sequence  int64
	epochUnix int64
	now       func() time.Time
}

type SnowflakeParts struct {
	Timestamp time.Time
	NodeID    int64
	Sequence  int64
}

func NewSnowflake(nodeID int64) (*Snowflake, error) {
	if nodeID < 0 || nodeID > snowflakeMaxNode {
		return nil, fmt.Errorf("node id must be between 0 and %d", snowflakeMaxNode)
	}

	return &Snowflake{
		nodeID:    nodeID,
		epochUnix: snowflakeEpoch.UnixMilli(),
		now:       time.Now,
	}, nil
}

func (s *Snowflake) NextID() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	nowMs := s.now().UnixMilli() - s.epochUnix
	if nowMs < s.lastMs {
		nowMs = s.lastMs
	}

	if nowMs == s.lastMs {
		s.sequence = (s.sequence + 1) & snowflakeMaxSequence
		if s.sequence == 0 {
			nowMs = s.waitNextMillis(s.lastMs)
		}
	} else {
		s.sequence = 0
	}

	s.lastMs = nowMs
	return uint64((nowMs << snowflakeTimestampShift) |
		(s.nodeID << snowflakeNodeShift) |
		s.sequence)
}

func ParseSnowflake(id uint64) SnowflakeParts {
	seq := int64(id) & snowflakeMaxSequence
	node := (int64(id) >> snowflakeNodeShift) & snowflakeMaxNode
	ms := int64(id) >> snowflakeTimestampShift

	return SnowflakeParts{
		Timestamp: time.UnixMilli(snowflakeEpoch.UnixMilli() + ms).UTC(),
		NodeID:    node,
		Sequence:  seq,
	}
}

func (s *Snowflake) waitNextMillis(lastMs int64) int64 {
	for {
		nowMs := s.now().UnixMilli() - s.epochUnix
		if nowMs > lastMs {
			return nowMs
		}
		time.Sleep(time.Millisecond)
	}
}
