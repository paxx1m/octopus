package relay

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/streams"
)

// sseReadResult 是 reader goroutine 向主循环传递的单次读取结果。
type sseReadResult struct {
	event *httpclient.StreamEvent
	err   error
}

// startStreamReader 在独立 goroutine 中读取上游 stream，通过 results 通道投递事件。
//
// 设计要点：
//   - done 关闭后 reader 退出，主循环不再 Close stream（由 reader 的 defer 统一 Close，避免双关）。
//   - Next() 可能阻塞等待上游 token；协程化后主循环才能同时监听首字超时与客户端断开。
//   - panic 恢复后仍尝试投递错误，让主循环按超时/断开语义分类处理。
func startStreamReader(
	ctx context.Context,
	clientStream streams.Stream[*httpclient.StreamEvent],
	results chan<- sseReadResult,
	done <-chan struct{},
) {
	// 唯一 Close 点：正常结束、主循环退出（done）、ctx 取消、panic 都走这里。
	defer close(results)
	defer clientStream.Close()
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("stream reader panic: %v", r)
			select {
			case results <- sseReadResult{err: fmt.Errorf("stream reader panic: %v", r)}:
			case <-done:
			case <-ctx.Done():
			}
		}
	}()
	// Next 可能阻塞等待上游 token；协程化后主循环才能同时监听超时与断开。
	for clientStream.Next() {
		select {
		case results <- sseReadResult{event: clientStream.Current()}:
		case <-done:
			return
		case <-ctx.Done():
			return
		}
	}
	if err := clientStream.Err(); err != nil {
		select {
		case results <- sseReadResult{err: err}:
		case <-done:
		case <-ctx.Done():
		}
	}
}

// aggregateStreamForLog 在流正常结束后，把缓存的事件聚合成完整响应体，记录 usage 与截断告警。
// 仅服务日志与计费，不影响已转发的客户端响应。
func (ra *relayAttempt) aggregateStreamForLog(
	ctx context.Context,
	responseEvents []*httpclient.StreamEvent,
	eventsTruncated bool,
) {
	// 流式路径不会自动生成完整响应体；聚合已缓存事件仅用于日志与 usage。
	responseBody, meta, err := ra.inAdapter.AggregateStreamChunks(context.WithoutCancel(ctx), responseEvents)
	if err != nil {
		log.Warnf("failed to aggregate stream response for log: %v", err)
		return
	}
	if eventsTruncated {
		log.Warnf("stream event buffer truncated at %d events; log body may be incomplete", streamEventBufferMax)
	}
	ra.metrics.InternalResponse = responseBody
	ra.metrics.RecordUsage(meta.Usage)
}

// appendEvent 将事件追加到日志缓存，超限后停止追加但仍继续转发。
// 返回最新的 (events, truncated) 状态。
func appendEvent(responseEvents []*httpclient.StreamEvent, eventsTruncated bool, event *httpclient.StreamEvent) ([]*httpclient.StreamEvent, bool) {
	if len(responseEvents) < streamEventBufferMax {
		return append(responseEvents, event), eventsTruncated
	}
	if !eventsTruncated {
		eventsTruncated = true
	}
	return responseEvents, eventsTruncated
}

// classifyReadError 将 reader goroutine 投递的错误分类为可操作的 relay 错误。
// 首字超时优先；其次客户端断开；最后才是真正的读取失败。
func classifyReadError(ctx context.Context, r sseReadResult, firstTokenTimedOut *atomic.Bool, firstTokenTimeoutSec int) error {
	// 首字超时通过取消请求上下文中断读取，这里把它转成明确的超时错误。
	if firstTokenTimedOut.Load() {
		return fmt.Errorf("first token timeout (%ds)", firstTokenTimeoutSec)
	}
	// 客户端断开：只停止，不触发故障转移（外层按 context.Canceled 记失败）。
	if ctx.Err() != nil {
		return context.Canceled
	}
	log.Warnf("failed to read event: %v", r.err)
	return fmt.Errorf("failed to read stream event: %w", r.err)
}
