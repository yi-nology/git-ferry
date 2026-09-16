package response

import (
	"log/slog"
	"math"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
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
