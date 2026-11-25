package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/y001j/iot-gateway/internal/web/services"

	"github.com/gin-gonic/gin"
)

// BaseHandler 基础处理器
type BaseHandler struct {
	services *services.Services
}

// NewBaseHandler 创建基础处理器
func NewBaseHandler(services *services.Services) *BaseHandler {
	return &BaseHandler{
		services: services,
	}
}

// APIResponse 统一 API 响应格式
type APIResponse struct {
	Success   bool        `json:"success"`
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp int64       `json:"timestamp"`
	RequestID string      `json:"request_id,omitempty"`
}

// SuccessResponse 成功响应
func (h *BaseHandler) SuccessResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: "Success",
		Data:    data,
	})
}

func (h *BaseHandler) SuccessResponseWithCode(c *gin.Context, code int, data interface{}) {
	c.JSON(code, APIResponse{
		Success: true,
		Message: "Success",
		Data:    data,
	})
}

// ErrorResponse 错误响应
func (h *BaseHandler) ErrorResponse(c *gin.Context, code int, message string) {
	response := APIResponse{
		Success:   false,
		Code:      code,
		Message:   message,
		Timestamp: time.Now().Unix(),
	}
	c.JSON(code, response)
}

// ValidationErrorResponse 验证错误响应
func (h *BaseHandler) ValidationErrorResponse(c *gin.Context, err error) {
	h.ErrorResponse(c, http.StatusBadRequest, "Validation failed: "+err.Error())
}

// GetPaginationParams 获取分页参数
func (h *BaseHandler) GetPaginationParams(c *gin.Context) (page, pageSize int) {
	page = 1
	pageSize = 20

	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	return page, pageSize
}

// GetUserID 从上下文获取用户ID
func (h *BaseHandler) GetUserID(c *gin.Context) string {
	if userID, exists := c.Get("user_id"); exists {
		// 首先尝试作为字符串获取
		if uid, ok := userID.(string); ok {
			return uid
		}
		// 如果是float64类型（来自JWT claims），转换为字符串
		if uid, ok := userID.(float64); ok {
			return strconv.Itoa(int(uid))
		}
		// 如果是int类型，转换为字符串
		if uid, ok := userID.(int); ok {
			return strconv.Itoa(uid)
		}
	}
	return ""
}

// HasRole 检查用户是否有指定角色
func (h *BaseHandler) HasRole(c *gin.Context, role string) bool {
	// 从上下文获取用户角色
	if userRole, exists := c.Get("role"); exists {
		if roleStr, ok := userRole.(string); ok {
			// 如果是admin或administrator角色，拥有所有权限
			if roleStr == "admin" || roleStr == "administrator" {
				return true
			}
			// 检查是否匹配指定角色
			if roleStr == role {
				return true
			}
		}
	}
	return false
}

// BindJSON 绑定 JSON 数据
func (h *BaseHandler) BindJSON(c *gin.Context, obj interface{}) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		h.ValidationErrorResponse(c, err)
		return false
	}
	return true
}

// GetQueryParam 获取查询参数
func (h *BaseHandler) GetQueryParam(c *gin.Context, key string, defaultValue ...string) string {
	value := c.Query(key)
	if value == "" && len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return value
}

// Handler a REST API handler
type Handler struct {
	*AuthHandler
	*SystemHandler
	*PluginHandler
	*RuleHandler
}
