package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/relay/keymanager"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/pipeline"
	"github.com/looplj/axonhub/llm/pipeline/stream"
	"github.com/looplj/axonhub/llm/streams"
	"github.com/looplj/axonhub/llm/transformer"
)

// Handler 返回处理入站请求并转发到上游服务的 Gin handler。
func Handler(inboundType llm.APIFormat) gin.HandlerFunc {
	inAdapter := newInbound(inboundType)
	return func(c *gin.Context) {
		run, err := newRelayRun(c, inboundType, inAdapter)
		if err != nil {
			return
		}
		run.run()
	}
}

func newRelayRun(c *gin.Context, inboundType llm.APIFormat, inAdapter transformer.Inbound) (*relayRun, error) {
	internalRequest, err := parseRequest(c, inboundType, inAdapter)
	if err != nil {
		return nil, err
	}

	if supportedModels := c.GetString("supported_models"); supportedModels != "" {
		if !slices.Contains(strings.Split(supportedModels, ","), internalRequest.Model) {
			err := errors.New("model not supported")
			resp.Error(c, http.StatusBadRequest, err.Error())
			return nil, err
		}
	}

	group, err := op.GroupGetEnabledMap(internalRequest.Model, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "model not found")
		return nil, err
	}

	apiKeyID := c.GetInt("api_key_id")
	iter, stickyKeyID := balancer.NewIterator(group, apiKeyID, internalRequest.Model)
	if iter.Len() == 0 {
		err := errors.New("no available channel")
		resp.Error(c, http.StatusServiceUnavailable, err.Error())
		return nil, err
	}

	return &relayRun{
		c:               c,
		inAdapter:       inAdapter,
		internalRequest: internalRequest,
		metrics: &RelayMetrics{
			APIKeyID:        apiKeyID,
			RequestModel:    internalRequest.Model,
			ActualModel:     internalRequest.Model,
			StartTime:       time.Now(),
			InternalRequest: internalRequest,
		},
		iter:        iter,
		group:       group,
		stickyKeyID: stickyKeyID,
	}, nil
}

// run 外层按分组候选渠道迭代；渠道内 Key 故障转移由 tryChannel 完成。
// 只有 ok（完整成功）或 written（已向客户端写出，不可再试）才结束；否则继续下一渠道。
func (r *relayRun) run() {
	ctx := r.c.Request.Context()
	var lastErr error

	for r.iter.Next() {
		select {
		case <-ctx.Done():
			log.Infof("request context canceled, stopping retry")
			r.metrics.Save(ctx, false, context.Canceled, r.iter.Attempts())
			return
		default:
		}

		ok, written, err := r.tryChannel()
		if ok {
			r.metrics.Save(ctx, true, nil, r.iter.Attempts())
			return
		}
		if written {
			// 流式已写出部分内容：不能换渠道，但 err 仍记失败（如客户端中途断开）。
			r.metrics.Save(ctx, false, err, r.iter.Attempts())
			return
		}
		if err != nil {
			lastErr = err
		}
	}

	if lastErr == nil {
		lastErr = errors.New("all channels failed")
	}
	r.metrics.Save(ctx, false, lastErr, r.iter.Attempts())
	resp.Error(r.c, http.StatusBadGateway, lastErr.Error())
}

// tryChannel 处理当前分组候选渠道，并在渠道内做 Key 快路径 / 多 Key 故障转移。
//
// 返回值语义：
//   - ok=true：本次转发成功（可结束整个请求）
//   - written=true：响应已开始写给客户端（流式首 token 已发出），不可再换渠道/Key
//   - err：失败原因（written=false 时可被外层继续重试）
//
// 渠道内 Key 选择分层：
//  1. allow_empty_key：不走 Key 管理器，直接空 Key 转发
//  2. 0 可用 Key：跳过本渠道
//  3. 1 可用 Key：单次尝试（仍走 OnResult / 熔断）
//  4. ≥2 可用 Key：按 KeyMode 选择并在同渠道内故障转移，耗尽后再换渠道
//
// 429 冷却恢复说明：
//
//	Key 被 429 后不会从列表物理删除，只是 IsAvailable 在冷却期内返回 false。
//	冷却结束后（now - LastUseTimeStamp >= cooldown），下次 ListAvailable/Select
//	会自动再次选中该 Key，无需后台任务“补回列表”。
func (r *relayRun) tryChannel() (ok, written bool, err error) {
	item := r.iter.Item()
	channel, err := op.ChannelGet(item.ChannelID, r.c.Request.Context())
	if err != nil {
		log.Warnf("failed to get channel %d: %v", item.ChannelID, err)
		r.iter.Skip(item.ChannelID, 0, fmt.Sprintf("channel_%d", item.ChannelID), fmt.Sprintf("channel not found: %v", err))
		return false, false, err
	}
	if !channel.Enabled {
		r.iter.Skip(channel.ID, 0, channel.Name, "channel disabled")
		return false, false, nil
	}

	// 无需 API Key 的上游：整次请求只用空 Key，失败后直接换下一渠道。
	if channel.AllowEmptyKey {
		return r.tryOneKey(channel, dbmodel.ChannelKey{}, item.ModelName)
	}

	// 粘性会话：仅当当前候选就是粘性渠道时生效。
	// - sticky key 仍可用：优先试它
	// - sticky key 临时不可用（如 429 冷却）或本次失败：同渠道换其他 Key，保留渠道亲和
	// - sticky key 已删除/永久禁用：清除会话粘性
	exclude := map[int]struct{}{}
	if r.iter.IsSticky() && r.stickyKeyID > 0 {
		var stickyKey *dbmodel.ChannelKey
		for i := range channel.Keys {
			if channel.Keys[i].ID == r.stickyKeyID {
				stickyKey = &channel.Keys[i]
				break
			}
		}
		if stickyKey == nil || !stickyKey.Enabled || stickyKey.ChannelKey == "" {
			balancer.ClearSticky(r.metrics.APIKeyID, r.metrics.RequestModel)
			r.stickyKeyID = 0
		} else if keymanager.IsAvailable(channel, *stickyKey, time.Now().Unix()) {
			success, written, err := r.tryOneKey(channel, *stickyKey, item.ModelName)
			if success || written {
				return success, written, err
			}
			// 本次 sticky key 失败：排除后继续同渠道其他 Key。
			exclude[stickyKey.ID] = struct{}{}
		} else {
			// 冷却中：跳过 sticky key，但渠道亲和仍保留。
			exclude[stickyKey.ID] = struct{}{}
		}
	}

	available := keymanager.ListAvailable(channel, exclude)
	switch len(available) {
	case 0:
		r.iter.Skip(channel.ID, 0, channel.Name, "no available key")
		return false, false, nil
	case 1:
		// 单 Key：没有“换 Key”空间，只做一次真实转发。
		return r.tryOneKey(channel, available[0], item.ModelName)
	default:
		// 多 Key：在同渠道内按策略选择并故障转移，直到无可用 Key。
		var lastErr error
		for {
			// 请求端已断开/取消（如客户端超时）：不再浪费性重试剩余 Key，
			// 直接以 context.Canceled 结束，避免把客户端取消记成渠道/Key 失败。
			if r.c.Request.Context().Err() != nil {
				return false, false, context.Canceled
			}
			// 每次循环重新取缓存快照，使上一 Key 的 429/禁用状态对本循环可见。
			if refreshed, getErr := op.ChannelGet(channel.ID, r.c.Request.Context()); getErr == nil {
				channel = refreshed
			}
			key, okSel := keymanager.Select(channel, exclude)
			if !okSel {
				break
			}
			success, written, err := r.tryOneKey(channel, key, item.ModelName)
			if success || written {
				return success, written, err
			}
			if err != nil {
				lastErr = err
			}
			// 本请求内不再重试该 Key（即使冷却逻辑上仍“可用”）。
			exclude[key.ID] = struct{}{}
		}
		return false, false, lastErr
	}
}

// tryOneKey builds outbound adapter, checks circuit breaker, and runs one forward attempt.
func (r *relayRun) tryOneKey(channel *dbmodel.Channel, usedKey dbmodel.ChannelKey, modelName string) (ok, written bool, err error) {
	skipped, isProbe := r.iter.SkipCircuitBreak(channel.ID, usedKey.ID, channel.Name)
	if skipped {
		return false, false, nil
	}
	// Any exit before RecordSuccess/Failure on a HalfOpen probe must abort the probe.
	probeDone := false
	defer func() {
		if isProbe && !probeDone {
			balancer.RecordProbeAborted(channel.ID, usedKey.ID, modelName)
		}
	}()

	outAdapter, err := newOutbound(channel.Type, r.internalRequest, channel.GetBaseUrl(), usedKey.ChannelKey)
	if err != nil {
		r.iter.Skip(channel.ID, usedKey.ID, channel.Name, err.Error())
		return false, false, nil
	}

	r.internalRequest.Model = modelName
	r.metrics.ActualModel = modelName
	r.metrics.ParamOverride = ""
	log.Infof("request model %s, mode: %d, forwarding to channel: %s model: %s key: %d (attempt %d/%d, sticky=%t)",
		r.metrics.RequestModel, r.group.Mode, channel.Name, modelName, usedKey.ID,
		r.iter.Index()+1, r.iter.Len(), r.iter.IsSticky())

	ra := &relayAttempt{
		relayRun:   r,
		outAdapter: outAdapter,
		channel:    channel,
		usedKey:    usedKey,
	}
	ok, written, err = ra.run()
	// run() always calls RecordSuccess or RecordFailure for real forwards.
	probeDone = true
	return ok, written, err
}

// run 统一管理一次通道+Key 尝试的完整生命周期。
// 返回 (success, written, err)：success 表示转发成功；written 表示响应已写出不可再试。
func (ra *relayAttempt) run() (bool, bool, error) {
	span := ra.iter.StartAttempt(ra.channel.ID, ra.usedKey.ID, ra.channel.Name)

	upstreamStatusCode, fwdErr := ra.forward()
	if fwdErr == nil && upstreamStatusCode == 0 {
		upstreamStatusCode = http.StatusOK
	}

	if fwdErr == nil {
		costDelta := ra.metrics.Stats.InputCost + ra.metrics.Stats.OutputCost
		keymanager.OnResult(ra.channel, ra.usedKey, upstreamStatusCode, costDelta)

		span.End(dbmodel.AttemptSuccess, "")
		op.StatsChannelUpdate(ra.channel.ID, dbmodel.StatsMetrics{
			WaitTime:       span.Duration().Milliseconds(),
			RequestSuccess: 1,
		})
		balancer.RecordSuccess(ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)
		balancer.SetSticky(ra.metrics.APIKeyID, ra.metrics.RequestModel, ra.channel.ID, ra.usedKey.ID)
		return true, false, nil
	}

	keymanager.OnResult(ra.channel, ra.usedKey, upstreamStatusCode, 0)
	span.End(dbmodel.AttemptFailed, fwdErr.Error())
	op.StatsChannelUpdate(ra.channel.ID, dbmodel.StatsMetrics{
		WaitTime:      span.Duration().Milliseconds(),
		RequestFailed: 1,
	})
	balancer.RecordFailure(ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)

	written := ra.c.Writer.Written()
	return false, written, fmt.Errorf("channel %s failed: %v", ra.channel.Name, fwdErr)
}

// parseRequest 解析并验证入站请求
func parseRequest(c *gin.Context, inboundType llm.APIFormat, inAdapter transformer.Inbound) (*llm.Request, error) {
	if inAdapter == nil {
		err := fmt.Errorf("unsupported inbound type: %s", inboundType)
		resp.Error(c, http.StatusBadRequest, err.Error())
		return nil, err
	}

	httpRequest, err := httpclient.ReadHTTPRequest(c.Request)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return nil, err
	}

	internalRequest, err := inAdapter.TransformRequest(c.Request.Context(), httpRequest)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if errors.Is(err, transformer.ErrInvalidRequest) {
			statusCode = http.StatusBadRequest
		}
		resp.Error(c, statusCode, err.Error())
		return nil, err
	}
	if internalRequest.RawRequest == nil {
		internalRequest.RawRequest = httpRequest
	}

	return internalRequest, nil
}

// forward 转发请求到上游服务。
// 返回的 statusCode 会进入 keymanager.OnResult / 熔断记录，必须尽量反映真实上游结果：
//   - 上游 HTTP 错误：middleware 捕获的 StatusCode
//   - 流式成功：200
//   - 流式失败但已写出首 token：200（客户端已看到部分内容）
//   - 流式失败且未写出：502（避免把失败记成成功冷却）
func (ra *relayAttempt) forward() (int, error) {
	ctx := ra.c.Request.Context()
	if ra.internalRequest.RawRequest == nil {
		return 0, fmt.Errorf("missing raw request")
	}

	httpClient, err := helper.ChannelHttpClient(ra.channel)
	if err != nil {
		log.Warnf("failed to get http client: %v", err)
		return 0, err
	}

	relayMiddleware := &relayPipelineMiddleware{attempt: ra}
	// 每次 attempt 新建 pipeline：出站适配器/渠道参数不同，且请求体可能被 ParamOverride 修改，
	// 复用同一 pipeline 实例跨渠道不安全。Factory 本身很轻，主要成本在上游 RTT。
	isStream := ra.internalRequest.Stream != nil && *ra.internalRequest.Stream

	// 首字超时：仅流式请求启用。计时起点前移到“发出请求之前”，覆盖
	// “等待上游响应头（TTFB）”+“收到响应头后等首个 token”两个阶段；
	// 收到首个有效 token 即停止。到点 cancel() 取消请求上下文，中断上游请求，
	// 再由上层循环故障转移切到下一个渠道。若不前移，上游在返回响应头前
	// 长时间思考/排队时，计时器（原在 writeStream 内）根本不会创建，超时不触发。
	firstTokenTimeoutSec := ra.group.FirstTokenTimeOut
	reqCtx := ctx
	var firstTokenTimer *time.Timer
	var firstTokenTimedOut atomic.Bool
	if isStream && firstTokenTimeoutSec > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithCancel(ctx)
		defer cancel()
		firstTokenTimer = time.AfterFunc(time.Duration(firstTokenTimeoutSec)*time.Second, func() {
			firstTokenTimedOut.Store(true)
			cancel()
		})
		defer firstTokenTimer.Stop()
	}

	result, err := pipeline.NewFactory(httpclient.NewHttpClientWithClient(httpClient)).
		Pipeline(
			&parsedRequestInbound{Inbound: ra.inAdapter, request: ra.internalRequest},
			ra.outAdapter,
			pipeline.WithMiddlewares(stream.EnsureUsage(), relayMiddleware),
			pipeline.WithEmptyResponseDetection(),
		).
		Process(reqCtx, ra.internalRequest.RawRequest)
	if err != nil {
		// 首字超时通过取消请求上下文体现为 context canceled；这里转成明确的
		// first token timeout 错误，触发故障转移，而不是把渠道记成上游失败。
		if firstTokenTimedOut.Load() {
			log.Warnf("first token timeout (%ds) while waiting for response headers, switching channel", firstTokenTimeoutSec)
			return relayMiddleware.upstreamStatusCode, fmt.Errorf("first token timeout (%ds)", firstTokenTimeoutSec)
		}
		return relayMiddleware.upstreamStatusCode, err
	}
	if result == nil {
		return 0, fmt.Errorf("empty pipeline result")
	}
	if result.Stream {
		if err := ra.writeStream(reqCtx, result.EventStream, firstTokenTimer, &firstTokenTimedOut); err != nil {
			// 已向客户端写出 SSE 后不可再换渠道；状态仍按 200 记录（部分成功语义）。
			// 尚未写出时返回 502，避免 OnResult 把失败当成功。
			if ra.c.Writer.Written() {
				return http.StatusOK, err
			}
			return http.StatusBadGateway, err
		}
		return http.StatusOK, nil
	}
	if result.Response == nil {
		return 0, fmt.Errorf("empty pipeline response")
	}
	ra.metrics.InternalResponse = result.Response.Body
	statusCode := result.Response.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	contentType := "application/json"
	if result.Response.Headers != nil {
		for key, values := range result.Response.Headers {
			for _, value := range values {
				ra.c.Header(key, value)
			}
		}
		if result.Response.Headers.Get("Content-Type") != "" {
			contentType = result.Response.Headers.Get("Content-Type")
		}
	}
	ra.c.Data(statusCode, contentType, result.Response.Body)
	return statusCode, nil
}

func (ra *relayAttempt) applyChannelRequestOptions(outboundRequest *httpclient.Request) {
	// ParamOverride 只覆盖 JSON 请求体；multipart 图片编辑等请求不能按 map 合并。
	if ra.channel.ParamOverride != nil && *ra.channel.ParamOverride != "" && strings.Contains(strings.ToLower(outboundRequest.Headers.Get("Content-Type")+" "+outboundRequest.ContentType), "application/json") {
		var bodyMap map[string]any
		if err := json.Unmarshal(outboundRequest.Body, &bodyMap); err != nil {
			log.Warnf("failed to unmarshal request body: %v, skipping param_override", err)
		} else {
			var override map[string]any
			if err := json.Unmarshal([]byte(*ra.channel.ParamOverride), &override); err != nil {
				log.Warnf("failed to unmarshal param_override: %v, skipping", err)
			} else {
				maps.Copy(bodyMap, override)
				modifiedBody, err := json.Marshal(bodyMap)
				if err != nil {
					log.Warnf("failed to marshal modified body: %v, skipping param_override", err)
				} else {
					outboundRequest.Body = modifiedBody
					ra.metrics.ParamOverride = *ra.channel.ParamOverride
				}
			}
		}
	}
	for _, header := range ra.channel.CustomHeader {
		// pipeline 在 raw request middleware 前已经写入 Auth；同名敏感头保持认证配置优先，延续旧 BuildHttpRequest 的覆盖顺序。
		if outboundRequest.Headers.Get(header.HeaderKey) != "" && httpclient.IsSensitiveHeader(header.HeaderKey) {
			continue
		}
		outboundRequest.Headers.Set(header.HeaderKey, header.HeaderValue)
	}
}

// streamEventBufferMax 限制内存中缓存的 SSE 事件数量，仅用于最终日志聚合。
// 超限后停止追加，仍继续转发；日志侧可能丢失后半段响应体。
const streamEventBufferMax = 4096

// writeStream 把 pipeline 输出的客户端格式流写回请求方。
//
// 设计要点：
//  1. 读上游 stream 放在独立 goroutine，避免 Next() 阻塞导致首 token 超时/客户端断开无法处理。
//  2. done 关闭后 reader 退出，主循环不再 Close stream（由 reader 的 defer 统一 Close，避免双关）。
//  3. 客户端断开返回 error（不是 nil），避免外层记成成功并错误累加费用。
//  4. 首 token 超时返回 error 且 Written()=false，外层可换渠道/Key。
//  5. responseEvents 只服务日志聚合，有上限，避免超长流 OOM。
func (ra *relayAttempt) writeStream(ctx context.Context, clientStream streams.Stream[*httpclient.StreamEvent], firstTokenTimer *time.Timer, firstTokenTimedOut *atomic.Bool) error {
	if clientStream == nil {
		return fmt.Errorf("empty pipeline stream")
	}

	// SSE 响应头：在真正写出首个事件前设置；gin 会在首次 Write 时发送。
	ra.c.Header("Content-Type", "text/event-stream")
	ra.c.Header("Cache-Control", "no-cache")
	ra.c.Header("Connection", "keep-alive")
	ra.c.Header("X-Accel-Buffering", "no")

	firstToken := true
	responseEvents := make([]*httpclient.StreamEvent, 0, 8)
	eventsTruncated := false
	type sseReadResult struct {
		event *httpclient.StreamEvent
		err   error
	}
	results := make(chan sseReadResult, 1)
	done := make(chan struct{})
	defer close(done)

	go func() {
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
	}()

	firstTokenTimeoutSec := ra.group.FirstTokenTimeOut

	for {
		select {
		case <-ctx.Done():
			// 先判首字超时再当客户端断开：首字超时也是通过取消 reqCtx 触发 ctx.Done()。
			if firstTokenTimedOut.Load() {
				log.Warnf("first token timeout (%ds) while reading stream, switching channel", firstTokenTimeoutSec)
				return fmt.Errorf("first token timeout (%ds)", firstTokenTimeoutSec)
			}
			// 客户端断开：返回 error，由 run()/Save 记失败；stream Close 交给 reader defer。
			log.Infof("client disconnected, stopping stream")
			return context.Canceled
		case r, ok := <-results:
			if !ok {
				log.Infof("stream end")
				if len(responseEvents) == 0 {
					return nil
				}
				// 流式路径不会自动生成完整响应体；聚合已缓存事件仅用于日志与 usage。
				responseBody, meta, err := ra.inAdapter.AggregateStreamChunks(context.WithoutCancel(ctx), responseEvents)
				if err != nil {
					log.Warnf("failed to aggregate stream response for log: %v", err)
					return nil
				}
				if eventsTruncated {
					log.Warnf("stream event buffer truncated at %d events; log body may be incomplete", streamEventBufferMax)
				}
				ra.metrics.InternalResponse = responseBody
				ra.metrics.RecordUsage(meta.Usage)
				return nil
			}
			if r.err != nil {
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

			if r.event == nil || len(r.event.Data) == 0 {
				continue
			}
			// 事件缓存仅服务最终日志；超限后仍转发，只是不再追加。
			if len(responseEvents) < streamEventBufferMax {
				responseEvents = append(responseEvents, r.event)
			} else if !eventsTruncated {
				eventsTruncated = true
			}
			if firstToken {
				ra.metrics.FirstTokenTime = time.Now()
				firstToken = false
				// 收到首个有效 token，停止首字计时；余下流不再受其约束。
				if firstTokenTimer != nil {
					firstTokenTimer.Stop()
				}
			}

			ra.c.SSEvent(r.event.Type, r.event.Data)
			ra.c.Writer.Flush()
		}
	}
}

// relayPipelineMiddleware 承接 octopus 自己的通道级副作用：
// 1. 在 pipeline 发出上游请求前应用渠道参数覆盖和自定义 header；
// 2. 在上游失败时保存 HTTP 状态码，供 key 冷却、熔断和后续选路使用；
// 3. 在非流式响应转成 llm.Response 后记录 usage。
// axonhub/llm 只提供了部分函数式 middleware 构造器，错误状态码和 llm 响应 usage 这两个回调没有公开构造器，
// 所以这里保留一个很薄的结构体实现完整接口，而不是在 relay 主流程里重复 pipeline 的执行逻辑。
type relayPipelineMiddleware struct {
	pipeline.DummyMiddleware
	attempt            *relayAttempt
	upstreamStatusCode int
}

func (m *relayPipelineMiddleware) Name() string {
	return "octopus_relay"
}

func (m *relayPipelineMiddleware) OnOutboundRawRequest(ctx context.Context, request *httpclient.Request) (*httpclient.Request, error) {
	if request.Headers == nil {
		request.Headers = make(http.Header)
	}
	m.attempt.applyChannelRequestOptions(request)
	return request, nil
}

func (m *relayPipelineMiddleware) OnOutboundRawError(ctx context.Context, err error) {
	var upstreamErr *httpclient.Error
	if errors.As(err, &upstreamErr) {
		// pipeline 会把上游错误转换成统一错误返回；这里在转换前记录原始 HTTP 状态码，用于渠道 key 的后续调度决策。
		m.upstreamStatusCode = upstreamErr.StatusCode
	}
}

func (m *relayPipelineMiddleware) OnOutboundLlmResponse(ctx context.Context, response *llm.Response) (*llm.Response, error) {
	if response != nil {
		// 非流式 usage 已由 outbound transformer 标准化到 llm.Response；流式 usage 在最终聚合时记录，避免重复计数。
		m.attempt.metrics.RecordUsage(response.Usage)
	}
	return response, nil
}

// parsedRequestInbound 让 pipeline 复用 relay 在选路前已经解析好的 llm.Request。
// 这样每次候选通道尝试只重新执行 outbound transform 和 HTTP 请求，不会重复读取或解析客户端 body。
type parsedRequestInbound struct {
	transformer.Inbound
	request *llm.Request
}

func (in *parsedRequestInbound) TransformRequest(ctx context.Context, request *httpclient.Request) (*llm.Request, error) {
	if in.request == nil {
		return nil, fmt.Errorf("missing parsed request")
	}
	// relay 已经为选路解析过请求；pipeline 入口复用该结果，避免每次通道尝试再次解析同一份 body。
	in.request.RawRequest = request
	return in.request, nil
}
