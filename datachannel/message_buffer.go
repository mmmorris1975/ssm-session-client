package datachannel

import (
	"container/list"
	"errors"
	"sync"
)

var ErrBufferFull = errors.New("buffer full")

type MessageBuffer interface {
	Len() int
	Add(msg *AgentMessage) error
	Remove(seqNum int64)
	Get(seqNum int64) *AgentMessage
	Next() *AgentMessage
}

type messageBuffer struct {
	mu     sync.RWMutex
	size   int
	buf    *list.List
	seqMap map[int64]*list.Element
	cursor *list.Element
}

func (m *messageBuffer) Len() int {
	return m.buf.Len()
}

func (m *messageBuffer) Add(msg *AgentMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Len() == m.size {
		return ErrBufferFull
	}

	el := m.buf.PushBack(msg)
	m.seqMap[msg.SequenceNumber] = el

	return nil
}

func (m *messageBuffer) Remove(seqNum int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if v, ok := m.seqMap[seqNum]; ok {
		if v != nil {
			m.buf.Remove(v)
		}
		delete(m.seqMap, seqNum)
	}
}

func (m *messageBuffer) Get(seqNum int64) *AgentMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if v, ok := m.seqMap[seqNum]; ok {
		if v != nil {
			return v.Value.(*AgentMessage)
		}
	}
	return nil
}

func (m *messageBuffer) Next() *AgentMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var el *list.Element
	if m.cursor == nil {
		el = m.buf.Front()
	} else {
		el = m.cursor.Next()
	}
	m.cursor = el

	if el != nil {
		return el.Value.(*AgentMessage)
	}
	return nil
}

func NewMessageBuffer(size int) *messageBuffer {
	mb := new(messageBuffer)
	mb.size = size
	mb.buf = list.New()
	mb.seqMap = make(map[int64]*list.Element)

	return mb
}
