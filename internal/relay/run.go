package relay

import (
	"context"
	"errors"
	"net/http"

	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/gin-gonic/gin"
)

// runRecorder 在重试结束后落点统计与审计日志（主 relay 与 rerank 共用）。
type runRecorder interface {
	Save(ctx context.Context, success bool, err error, attempts []dbmodel.ChannelAttempt)
}

// runState 承载候选渠道迭代重试与最终指标落点，消除主 relay 与 rerank 的重复转发链路。
type runState struct {
	c            *gin.Context
	iter         *balancer.Iterator
	apiKeyID     int
	requestModel string
	stickyKeyID  *int
	tryOne       tryOneKeyFn
	record       runRecorder
}

// run 外层按分组候选渠道迭代；渠道内 Key 故障转移由 tryChannelWithKeys 完成。
// 只有 ok（完整成功）或 written（已向客户端写出，不可再试）才结束；否则继续下一渠道。
func (rs *runState) run() {
	ctx := rs.c.Request.Context()
	var lastErr error

	for rs.iter.Next() {
		select {
		case <-ctx.Done():
			log.Infof("request context canceled, stopping retry")
			rs.record.Save(ctx, false, context.Canceled, rs.iter.Attempts())
			return
		default:
		}

		ok, written, err := tryChannelWithKeys(rs.c, rs.iter, rs.stickyKeyID, rs.apiKeyID, rs.requestModel, rs.tryOne)
		if ok {
			rs.record.Save(ctx, true, nil, rs.iter.Attempts())
			return
		}
		if written {
			// 流式已写出部分内容：不能换渠道，但 err 仍记失败（如客户端中途断开）。
			if err == nil {
				err = errors.New("response partially written")
			}
			rs.record.Save(ctx, false, err, rs.iter.Attempts())
			return
		}
		if err != nil {
			lastErr = err
		}
	}

	if lastErr == nil {
		lastErr = errors.New("all channels failed")
	}
	rs.record.Save(ctx, false, lastErr, rs.iter.Attempts())
	if !rs.c.Writer.Written() {
		resp.Error(rs.c, http.StatusBadGateway, lastErr.Error())
	}
}
