package common

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type StreamEndReason string

const (
	StreamEndReasonNone        StreamEndReason = ""
	StreamEndReasonDone        StreamEndReason = "done"
	StreamEndReasonTimeout     StreamEndReason = "timeout"
	StreamEndReasonClientGone  StreamEndReason = "client_gone"
	StreamEndReasonScannerErr  StreamEndReason = "scanner_error"
	StreamEndReasonHandlerStop StreamEndReason = "handler_stop"
	StreamEndReasonEOF         StreamEndReason = "eof"
	StreamEndReasonPanic       StreamEndReason = "panic"
	StreamEndReasonPingFail    StreamEndReason = "ping_fail"
)

const maxStreamErrorEntries = 20

type StreamErrorEntry struct {
	Message   string
	Timestamp time.Time
}

type StreamStatus struct {
	EndReason StreamEndReason
	EndError  error
	endOnce   sync.Once

	mu         sync.Mutex
	Errors     []StreamErrorEntry
	ErrorCount int
	terminal   string
	received   int
}

// MarkTerminal 记录协议层的终结事件（Responses 的 response.completed / response.incomplete、
// Claude 的 message_stop 等）。
//
// 它和 EndReason 是两个维度：EndReason 说的是传输层怎么断的（eof / timeout / client_gone），
// Terminal 说的是上游有没有把话说完。不能用 EndReason 兼任：扫描器读完 body 会立刻写
// EndReason=eof，此时 dataHandler 往往还在处理排队的最后几个分片，endOnce 让 eof 稳定胜出，
// 终结标记根本写不进去。
//
// 对不发 data: [DONE] 的协议（Responses / Claude），"终结事件缺失 + end_reason=eof"就是
// 流被截断的判定依据。
func (s *StreamStatus) MarkTerminal(event string) {
	if s == nil || event == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminal == "" {
		s.terminal = event
	}
}

// TerminalEvent 返回收到的第一个终结事件类型，没收到过则为空。
func (s *StreamStatus) TerminalEvent() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminal
}

// SetReceived 由扫描器 goroutine 在退出时快照它收到的 data 帧数。
//
// 不直接让日志去读 RelayInfo.ReceivedResponseCount：清理逻辑等 goroutine 退出是有 5 秒
// 上限的，超时那条降级路径上扫描器可能还在自增，跨 goroutine 读就成了数据竞争。
func (s *StreamStatus) SetReceived(n int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.received = n
}

// Received 返回扫描器收到的 data 帧数快照。
func (s *StreamStatus) Received() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.received
}

func NewStreamStatus() *StreamStatus {
	return &StreamStatus{}
}

func (s *StreamStatus) SetEndReason(reason StreamEndReason, err error) {
	if s == nil {
		return
	}
	s.endOnce.Do(func() {
		s.EndReason = reason
		s.EndError = err
	})
}

func (s *StreamStatus) RecordError(msg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ErrorCount++
	if len(s.Errors) < maxStreamErrorEntries {
		s.Errors = append(s.Errors, StreamErrorEntry{
			Message:   msg,
			Timestamp: time.Now(),
		})
	}
}

func (s *StreamStatus) HasErrors() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount > 0
}

func (s *StreamStatus) TotalErrorCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount
}

func (s *StreamStatus) IsNormalEnd() bool {
	if s == nil {
		return true
	}
	return s.EndReason == StreamEndReasonDone ||
		s.EndReason == StreamEndReasonEOF ||
		s.EndReason == StreamEndReasonHandlerStop
}

func (s *StreamStatus) Summary() string {
	if s == nil {
		return "StreamStatus<nil>"
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "reason=%s", s.EndReason)
	if s.EndError != nil {
		fmt.Fprintf(b, " end_error=%q", s.EndError.Error())
	}
	s.mu.Lock()
	if s.ErrorCount > 0 {
		fmt.Fprintf(b, " soft_errors=%d", s.ErrorCount)
	}
	s.mu.Unlock()
	return b.String()
}
