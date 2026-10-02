package response

import (
	"log/slog"
	"math"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// Error sends an error response with a custom status code (kept for backward compatibility)
func Error(c *app.RequestContext, statusCode int, message string) {
	c.JSON(statusCode, Response{
		Code:      statusCode,
		Message:   message,
		Data:      nil,
		Timestamp: nowTimestamp(),
	})
}

// Success sends a 200 OK response
func Success(c *app.RequestContext, data interface{}) {
	c.JSON(consts.StatusOK, Response{
		Code:      consts.StatusOK,
		Message:   "success",
		Data:      data,
		Timestamp: nowTimestamp(),
	})
}

// Created sends a 201 Created response
func Created(c *app.RequestContext, data interface{}) {
	c.JSON(consts.StatusCreated, Response{
		Code:      consts.StatusCreated,
		Message:   "created",
		Data:      data,
		Timestamp: nowTimestamp(),
	})
}

// BadRequest sends a 400 Bad Request response
func BadRequest(c *app.RequestContext, message string) {
	Error(c, consts.StatusBadRequest, message)
}

// NotFound sends a 404 Not Found response
func NotFound(c *app.RequestContext, message string) {
	Error(c, consts.StatusNotFound, message)
}

// InternalError sends a 500 Internal Server Error response.
// 出于安全:原始 detail 只写入服务端日志,不回传客户端(避免泄露 SQL/DSN/表名/文件路径等内部信息)。
func InternalError(c *app.RequestContext, message string) {
	if message != "" {
		slog.Error("internal server error", "detail", message)
	}
	Error(c, consts.StatusInternalServerError, "internal server error")
}

// FromError 按 core 的错误分类契约(corebridge.Classify)选状态码并回响应。
//   - not_found      → 404,回传业务消息(与既往 errors.Is(ErrNotFound) 分支一致)
//   - validation     → 400
//   - conflict       → 409(任务运行中/并发上限/合规冻结)
//   - auth           → 401
//   - rate_limited   → 429
//   - unavailable    → 503,消息回传(平台侧 5xx,客户端可重试)
//   - internal/未分类 → 500,详情只进日志(沿用 InternalError 的安全策略)
func FromError(c *app.RequestContext, err error) {
	if err == nil {
		InternalError(c, "")
		return
	}
	switch corebridge.Classify(err) {
	case corebridge.ErrorClassNotFound:
		NotFound(c, err.Error())
	case corebridge.ErrorClassValidation:
		BadRequest(c, err.Error())
	case corebridge.ErrorClassConflict:
		Error(c, consts.StatusConflict, err.Error())
	case corebridge.ErrorClassAuth:
		Error(c, consts.StatusUnauthorized, err.Error())
	case corebridge.ErrorClassRateLimited:
		Error(c, consts.StatusTooManyRequests, err.Error())
	case corebridge.ErrorClassUnavailable:
		Error(c, consts.StatusServiceUnavailable, err.Error())
	default:
		InternalError(c, err.Error())
	}
}

// Paginated sends a 200 OK response with pagination data
func Paginated(c *app.RequestContext, list interface{}, total int64, page, pageSize int) {
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	c.JSON(consts.StatusOK, Response{
		Code:    consts.StatusOK,
		Message: "success",
		Data: PaginatedData{
			List: list,
			Pagination: Pagination{
				Page:       page,
				PageSize:   pageSize,
				Total:      total,
				TotalPages: totalPages,
			},
		},
		Timestamp: nowTimestamp(),
	})
}

// NoContent sends a 204 No Content response
func NoContent(c *app.RequestContext) {
	c.Status(consts.StatusNoContent)
}
