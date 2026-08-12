package handlers

import (
	"net/http"
	"strconv"

	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/gin-gonic/gin"
)

// bindJSON 解析 JSON 请求体；失败时响应 400 并返回 false。
func bindJSON(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return false
	}
	return true
}

// pathID 解析路径参数为 int；失败时响应 400 并返回 false。
func pathID(c *gin.Context, name string) (int, bool) {
	id, err := strconv.Atoi(c.Param(name))
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidParam)
		return 0, false
	}
	return id, true
}

// serverError 统一 500 内部错误响应；非调试模式不向客户端泄漏内部错误细节。
func serverError(c *gin.Context, err error) {
	if err == nil {
		resp.Error(c, http.StatusInternalServerError, resp.ErrInternalServer)
		return
	}
	if !conf.IsDebug() {
		log.Errorf("internal server error: %v", err)
		resp.Error(c, http.StatusInternalServerError, resp.ErrInternalServer)
		return
	}
	resp.Error(c, http.StatusInternalServerError, err.Error())
}

// handleWriteError 统一写操作错误映射：唯一键冲突 → 409，其余 → 500。
func handleWriteError(c *gin.Context, err error) {
	if db.IsDuplicateError(err) {
		resp.Error(c, http.StatusConflict, resp.ErrDuplicateResource)
		return
	}
	serverError(c, err)
}
