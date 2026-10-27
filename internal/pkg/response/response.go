package response

import (
	"math"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/oekit/logx"
	"github.com/oekit/pkg/errors"

	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// logger 是本包的日志出口(经 logx 门面)。
//
// **用 Delegating 而不是 Default()**:包级 var 在 init 时求值,
// 直接捕获 Default() 会永远绑在 init 那一刻的默认 logger 上,
// 消费方随后的 SetDefault 对它无效;Delegating 是**调用时**解析,
// 自动跟随 main() 里装好的后端。
var logger = logx.Delegating("response")

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
		logger.Error("internal server error", "detail", message)
	}
	Error(c, consts.StatusInternalServerError, "internal server error")
}

// NewBizError 造一个**带 HTTP 码**的业务错误(oekit errors.BizError)。
//
// **为什么要有这个**:错误在 service 层产生、在 handler 层渲染 ——
// 「这个错误该落成 403 还是 400」如果由渲染方现场挑,同一个业务错误
// 在不同 handler 里可能落成不同状态码。BizError 把「码 + 消息」绑在
// 错误自己身上 ⇒ 产生错误的地方定码,渲染的地方只负责回。
//
// 本仓响应信封里 `code` 字段就是 HTTP 状态码 ⇒ 业务码与 HTTP 码合一,
// 免得信封里没地方放业务码。
func NewBizError(statusCode int, message string) *errors.BizError {
	return errors.NewBizErrorWithStatus(statusCode, statusCode, message)
}

// Biz 按业务错误回响应:错误自带码就用自己的,没带就走 FromError 的分类契约。
func Biz(c *app.RequestContext, err error) {
	be, ok := errors.AsBizError(err)
	if !ok {
		FromError(c, err)
		return
	}
	code := be.HTTPCode
	if code == 0 {
		code = consts.StatusBadRequest
	}
	Error(c, code, be.Message)
}

// FromError 按 core 的错误分类契约(corebridge.Classify)选状态码并回响应。
//   - 业务错误(BizError) → 用它自带的码(错误在产生处定的码,渲染处不猜)
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
	if be, ok := errors.AsBizError(err); ok {
		code := be.HTTPCode
		if code == 0 {
			code = consts.StatusBadRequest
		}
		Error(c, code, be.Message)
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
