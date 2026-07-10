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
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
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
	iter := balancer.NewIterator(group, apiKeyID, internalRequest.Model)
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
		iter:     iter,
		group:    group,
		selector: newChannelSelector(c.Request.Context()),
	}, nil
}

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

		attempt, err := r.prepareAttempt()
		if err != nil {
			lastErr = err
			continue
		}
		if attempt == nil {
			continue
		}

		written, err := attempt.run()
		if err == nil {
			r.metrics.Save(ctx, true, nil, r.iter.Attempts())
			return
		}
		if written {
			log.Warnf("relay attempt failed (already written, model=%s): %v", r.metrics.RequestModel, err)
			r.metrics.Save(ctx, false, err, r.iter.Attempts())
			return
		}
		// 记录每次渠道尝试失败的具体原因，便于排查“所有模型调用都失败”的根因（DNS/超时/上游断连等）。
		log.Warnf("relay attempt failed (model=%s, attempt=%d/%d): %v",
			r.metrics.RequestModel, r.iter.Index()+1, r.iter.Len(), err)
		lastErr = err
	}

	if lastErr == nil {
		lastErr = errors.New("all channels failed")
	}

	// 兜底：组内候选全部失败且尚未向客户端写入响应时，在“同组同渠道”上
	// 用渠道真实支持的其他高能力模型（按上下文长度降序）重试，避免整组不可用。
	if lastErr != nil && !r.c.Writer.Written() {
		if fallbackErr := r.runFallback(ctx); fallbackErr == nil {
			r.metrics.Save(ctx, true, nil, r.iter.Attempts())
			return
		} else {
			lastErr = fallbackErr
		}
	}

	r.metrics.Save(ctx, false, lastErr, r.iter.Attempts())
	resp.Error(r.c, http.StatusBadGateway, lastErr.Error())
}

func (r *relayRun) prepareAttempt() (*relayAttempt, error) {
	item := r.iter.Item()
	if r.selector.skipIfUnavailable(r.iter, item) {
		return nil, nil
	}

	channel, err := r.selector.channelFor(item.ChannelID)
	if err != nil {
		log.Warnf("failed to get channel %d: %v", item.ChannelID, err)
		reason := fmt.Sprintf("channel not found: %v", err)
		r.selector.markChannelUnavailable(item.ChannelID, reason)
		r.iter.Skip(item.ChannelID, 0, r.selector.channelName(item.ChannelID), reason)
		return nil, err
	}
	if !channel.Enabled {
		r.selector.markChannelUnavailable(channel.ID, "channel disabled")
		r.iter.Skip(channel.ID, 0, channel.Name, "channel disabled")
		return nil, nil
	}

	keys := r.selector.keysFor(channel)
	if len(keys) == 0 {
		r.iter.Skip(channel.ID, 0, channel.Name, "no available key")
		return nil, nil
	}

	availableKeys := r.selector.availableKeys(channel, r.iter)
	if len(availableKeys) == 0 {
		r.selector.markModelUnavailable(channel.ID, item.ModelName, "all keys circuit-brokered")
		r.iter.Skip(channel.ID, 0, channel.Name, "all keys circuit-brokered")
		return nil, nil
	}

	var (
		usedKey    dbmodel.ChannelKey
		outAdapter transformer.Outbound
	)
	for _, candidateKey := range availableKeys {
		outAdapter, err = newOutbound(channel.Type, r.internalRequest, channel.GetBaseUrl(), candidateKey.ChannelKey)
		if err != nil {
			log.Warnf("failed to build outbound for channel %s key %d: %v", channel.Name, candidateKey.ID, err)
			continue
		}
		usedKey = candidateKey
		break
	}
	if usedKey.ChannelKey == "" && !channel.NoKey {
		r.selector.markModelUnavailable(channel.ID, item.ModelName, "all outbound build failed")
		r.iter.Skip(channel.ID, 0, channel.Name, "all outbound build failed")
		return nil, nil
	}

	// 每次尝试都把客户端模型改成本次候选的实际上游模型；重试时会被下一候选覆盖。
	r.internalRequest.Model = item.ModelName
	r.metrics.ActualModel = item.ModelName
	r.metrics.ParamOverride = ""
	log.Infof("request model %s, mode: %d, forwarding to channel: %s model: %s (attempt %d/%d, sticky=%t)",
		r.metrics.RequestModel, r.group.Mode, channel.Name, item.ModelName,
		r.iter.Index()+1, r.iter.Len(), r.iter.IsSticky())

	return &relayAttempt{
		relayRun:   r,
		outAdapter: outAdapter,
		channel:    channel,
		usedKey:    usedKey,
	}, nil
}

// buildFallbackAttempt 为兜底场景构造一次转发尝试。
// 与 prepareAttempt 的区别：
//   - 候选模型不来自 iter.Item，而是由调用方指定的 fallbackModel（渠道真实支持的模型）；
//   - 不依赖 iter 的 Skip/SkipCircuitBreak（兜底阶段自行控制跳过与熔断检查）。
//
// channel 必须已校验为 enabled 且存在可用 key；调用方需在调用前完成这些检查。
func (r *relayRun) buildFallbackAttempt(channel *dbmodel.Channel, usedKey dbmodel.ChannelKey, fallbackModel string) (*relayAttempt, error) {
	outAdapter, err := newOutbound(channel.Type, r.internalRequest, channel.GetBaseUrl(), usedKey.ChannelKey)
	if err != nil {
		return nil, err
	}

	r.internalRequest.Model = fallbackModel
	r.metrics.ActualModel = fallbackModel
	r.metrics.ParamOverride = ""
	return &relayAttempt{
		relayRun:      r,
		outAdapter:    outAdapter,
		channel:       channel,
		usedKey:       usedKey,
		fallbackModel: fallbackModel,
	}, nil
}

// runFallback 在组内候选全部失败后，于“同组同渠道”上用渠道真实支持的其他高能力模型兜底重试。
// 仅在尚未向客户端写入响应时调用。成功返回 nil。
func (r *relayRun) runFallback(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// 收集本轮已尝试过的模型名（含组内已配置候选），用于兜底去重
	attemptModelNames := make([]string, 0, len(r.iter.Attempts()))
	for _, a := range r.iter.Attempts() {
		attemptModelNames = append(attemptModelNames, a.ModelName)
	}
	groupItemModels := make([]string, 0, len(r.group.Items))
	// 去重收集本组涉及的渠道，保证同渠道只查一次可用模型
	channelSeen := make(map[int]struct{})
	channelIDs := make([]int, 0, len(r.group.Items))
	for _, item := range r.group.Items {
		groupItemModels = append(groupItemModels, item.ModelName)
		if _, ok := channelSeen[item.ChannelID]; !ok {
			channelSeen[item.ChannelID] = struct{}{}
			channelIDs = append(channelIDs, item.ChannelID)
		}
	}
	usedModels := collectUsedModels(attemptModelNames, groupItemModels)

	var lastErr error
	for _, channelID := range channelIDs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		channel, err := r.selector.channelFor(channelID)
		if err != nil || !channel.Enabled {
			continue
		}
		keys := r.selector.keysFor(channel)
		if len(keys) == 0 {
			continue
		}
		channelModels := r.selector.channelModels(channel)

		for _, usedKey := range keys {
			fallbackModels := selectFallbackModels(
				usedModels, channel.ID, usedKey.ID, channelModels...,
			)
			if len(fallbackModels) == 0 {
				continue
			}

			for _, fm := range fallbackModels {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				attempt, bErr := r.buildFallbackAttempt(channel, usedKey, fm)
				if bErr != nil || attempt == nil {
					continue
				}
				log.Infof("relay fallback (model=%s, channel=%s, key=%d, fallback_model=%s): no group candidate succeeded, trying channel's strongest available model",
					r.metrics.RequestModel, channel.Name, usedKey.ID, fm)

				written, runErr := attempt.run()
				if runErr == nil {
					log.Infof("relay fallback succeeded (model=%s, channel=%s, key=%d, fallback_model=%s)",
						r.metrics.RequestModel, channel.Name, usedKey.ID, fm)
					return nil
				}
				// 已写入响应（流式已开始）则不能再切换，直接返回
				if written {
					log.Warnf("relay fallback failed (already written, model=%s, key=%d, fallback_model=%s): %v",
						r.metrics.RequestModel, usedKey.ID, fm, runErr)
					return runErr
				}
				log.Warnf("relay fallback failed (model=%s, channel=%s, key=%d, fallback_model=%s): %v",
					r.metrics.RequestModel, channel.Name, usedKey.ID, fm, runErr)
				lastErr = runErr
				usedModels[fm] = true
			}
		}
	}

	if lastErr == nil {
		lastErr = errors.New("fallback exhausted, no available model")
	}
	return lastErr
}

// run 统一管理一次通道尝试的完整生命周期。
func (ra *relayAttempt) run() (bool, error) {
	defer ra.selector.invalidateKeys(ra.channel.ID)

	var span *balancer.AttemptSpan
	if ra.fallbackModel != "" {
		// 兜底场景：模型名不由 iter 当前位置决定，避免越界且记录正确的模型
		span = ra.iter.StartAttemptWithModel(ra.channel.ID, ra.usedKey.ID, ra.channel.Name, ra.fallbackModel)
	} else {
		span = ra.iter.StartAttempt(ra.channel.ID, ra.usedKey.ID, ra.channel.Name)
	}

	upstreamStatusCode, fwdErr := ra.forward()
	if fwdErr == nil && upstreamStatusCode == 0 {
		upstreamStatusCode = http.StatusOK
	}
	if ra.usedKey.ID != 0 {
		ra.usedKey.StatusCode = upstreamStatusCode
		ra.usedKey.LastUseTimeStamp = time.Now().Unix()
	}

	if fwdErr == nil {
		if ra.usedKey.ID != 0 {
			ra.usedKey.TotalCost += ra.metrics.Stats.InputCost + ra.metrics.Stats.OutputCost
			op.ChannelKeyUpdate(ra.usedKey)
		}

		span.End(dbmodel.AttemptSuccess, "")
		op.StatsChannelUpdate(ra.channel.ID, dbmodel.StatsMetrics{
			WaitTime:       span.Duration().Milliseconds(),
			RequestSuccess: 1,
		})
		balancer.RecordSuccess(ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)
		balancer.SetSticky(ra.metrics.APIKeyID, ra.metrics.RequestModel, ra.channel.ID, ra.usedKey.ID)
		return false, nil
	}

	if ra.usedKey.ID != 0 {
		op.ChannelKeyUpdate(ra.usedKey)
	}
	span.End(dbmodel.AttemptFailed, fwdErr.Error())
	op.StatsChannelUpdate(ra.channel.ID, dbmodel.StatsMetrics{
		WaitTime:      span.Duration().Milliseconds(),
		RequestFailed: 1,
	})
	balancer.RecordFailure(ra.channel.ID, ra.usedKey.ID, ra.internalRequest.Model)

	return ra.c.Writer.Written(), fmt.Errorf("channel %s failed: %v", ra.channel.Name, fwdErr)
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

// forward 转发请求到上游服务
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
	result, err := pipeline.NewFactory(httpclient.NewHttpClientWithClient(httpClient)).
		Pipeline(
			&parsedRequestInbound{Inbound: ra.inAdapter, request: ra.internalRequest},
			ra.outAdapter,
			pipeline.WithMiddlewares(stream.EnsureUsage(), relayMiddleware),
			pipeline.WithEmptyResponseDetection(),
		).
		Process(ctx, ra.internalRequest.RawRequest)
	if err != nil {
		return relayMiddleware.upstreamStatusCode, err
	}
	if result == nil {
		return 0, fmt.Errorf("empty pipeline result")
	}
	if result.Stream {
		if err := ra.writeStream(ctx, result.EventStream); err != nil {
			return http.StatusOK, err
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
	if ra.channel.NoKey {
		outboundRequest.Headers.Del("Authorization")
		outboundRequest.Headers.Del("X-Api-Key")
		outboundRequest.Headers.Del("X-Goog-Api-Key")
	}

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

// writeStream 把 pipeline 输出的客户端格式流写回请求方，并保留首 token 超时切换通道的行为。
func (ra *relayAttempt) writeStream(ctx context.Context, clientStream streams.Stream[*httpclient.StreamEvent]) error {
	if clientStream == nil {
		return fmt.Errorf("empty pipeline stream")
	}

	// 设置 SSE 响应头
	ra.c.Header("Content-Type", "text/event-stream")
	ra.c.Header("Cache-Control", "no-cache")
	ra.c.Header("Connection", "keep-alive")
	ra.c.Header("X-Accel-Buffering", "no")

	firstToken := true
	responseEvents := make([]*httpclient.StreamEvent, 0, 8)
	type sseReadResult struct {
		event *httpclient.StreamEvent
		err   error
	}
	results := make(chan sseReadResult, 1)
	done := make(chan struct{})
	defer close(done)
	go func() {
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
		// Next 可能阻塞等待上游 token；放到协程里让首 token 超时和客户端断开都能及时打断本次通道尝试。
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
	var firstTokenTimer *time.Timer
	var firstTokenC <-chan time.Time
	if firstTokenTimeoutSec > 0 {
		firstTokenTimer = time.NewTimer(time.Duration(firstTokenTimeoutSec) * time.Second)
		firstTokenC = firstTokenTimer.C
		defer func() {
			if firstTokenTimer != nil {
				firstTokenTimer.Stop()
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			log.Infof("client disconnected, stopping stream")
			_ = clientStream.Close()
			return nil
		case <-firstTokenC:
			log.Warnf("first token timeout (%ds), switching channel", firstTokenTimeoutSec)
			_ = clientStream.Close()
			return fmt.Errorf("first token timeout (%ds)", firstTokenTimeoutSec)
		case r, ok := <-results:
			if !ok {
				log.Infof("stream end")
				if len(responseEvents) == 0 {
					return nil
				}
				// 客户端请求流式时，pipeline 只负责边转边写，不会自动生成完整响应体。
				// 这里复用同一个 inbound 聚合器把已经写给客户端的事件合成最终 body，日志只落一次最终响应。
				responseBody, meta, err := ra.inAdapter.AggregateStreamChunks(context.WithoutCancel(ctx), responseEvents)
				if err != nil {
					log.Warnf("failed to aggregate stream response for log: %v", err)
					return nil
				}
				ra.metrics.InternalResponse = responseBody
				ra.metrics.RecordUsage(meta.Usage)
				return nil
			}
			if r.err != nil {
				log.Warnf("failed to read event: %v", r.err)
				return fmt.Errorf("failed to read stream event: %w", r.err)
			}

			if r.event == nil || len(r.event.Data) == 0 {
				continue
			}
			// 这里只临时保存 pipeline 已经转换好的客户端格式事件，正常结束后聚合成最终响应体用于日志；不会把分片逐条落库。
			responseEvents = append(responseEvents, r.event)
			if firstToken {
				ra.metrics.FirstTokenTime = time.Now()
				firstToken = false
				if firstTokenTimer != nil {
					if !firstTokenTimer.Stop() {
						select {
						case <-firstTokenTimer.C:
						default:
						}
					}
					firstTokenTimer = nil
					firstTokenC = nil
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
