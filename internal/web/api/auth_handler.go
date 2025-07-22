package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"

	"github.com/gin-gonic/gin"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	*BaseHandler
	authService services.AuthService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(authService services.AuthService) *AuthHandler {
	return &AuthHandler{
		BaseHandler: &BaseHandler{},
		authService: authService,
	}
}

// Login 用户登录
// @Summary 用户登录
// @Description 用户登录接口
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body models.LoginRequest true "登录请求"
// @Success 200 {object} APIResponse{data=models.LoginResponse}
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if !h.BindJSON(c, &req) {
		return
	}

	// 调试：打印接收到的登录请求
	fmt.Printf("收到的登录请求: Username=[%s], Password=[%s]\n", req.Username, req.Password)

	response, err := h.authService.Login(&req)
	if err != nil {
		h.ErrorResponse(c, http.StatusUnauthorized, err.Error())
		return
	}

	h.SuccessResponse(c, response)
}

// Logout 用户登出
// @Summary 用户登出
// @Description 用户登出接口
// @Tags 认证
// @Security ApiKeyAuth
// @Success 200 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if token == "" {
		h.ErrorResponse(c, http.StatusBadRequest, "Missing authorization header")
		return
	}

	err := h.authService.Logout(token)
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, gin.H{"message": "Logged out successfully"})
}

// Refresh 刷新令牌
// @Summary 刷新访问令牌
// @Description 使用刷新令牌获取新的访问令牌
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body models.RefreshTokenRequest true "刷新令牌请求"
// @Success 200 {object} APIResponse{data=models.LoginResponse}
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Router /auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req models.RefreshTokenRequest
	if !h.BindJSON(c, &req) {
		return
	}

	response, err := h.authService.RefreshToken(&req)
	if err != nil {
		h.ErrorResponse(c, http.StatusUnauthorized, err.Error())
		return
	}

	h.SuccessResponse(c, response)
}

// GetProfile 获取用户档案
// @Summary 获取用户档案
// @Description 获取当前用户的档案信息
// @Tags 认证
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} APIResponse{data=models.UserInfo}
// @Failure 401 {object} APIResponse
// @Failure 404 {object} APIResponse
// @Router /auth/profile [get]
func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID := h.GetUserID(c)
	if userID == "" {
		h.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	// 将字符串ID转换为整数
	id, err := strconv.Atoi(userID)
	if err != nil {
		h.ErrorResponse(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	profile, err := h.authService.GetProfile(id)
	if err != nil {
		h.ErrorResponse(c, http.StatusNotFound, err.Error())
		return
	}

	h.SuccessResponse(c, profile)
}

// UpdateProfile 更新用户档案
// @Summary 更新用户档案
// @Description 更新当前用户的档案信息
// @Tags 认证
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param request body models.UpdateProfileRequest true "更新档案请求"
// @Success 200 {object} APIResponse
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Router /auth/profile [put]
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userID := h.GetUserID(c)
	if userID == "" {
		h.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	// 将字符串ID转换为整数
	id, err := strconv.Atoi(userID)
	if err != nil {
		h.ErrorResponse(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	var req models.UpdateProfileRequest
	if !h.BindJSON(c, &req) {
		return
	}

	err = h.authService.UpdateProfile(id, &req)
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	h.SuccessResponse(c, gin.H{"message": "Profile updated successfully"})
}

// ChangePassword 修改密码
// @Summary 修改密码
// @Description 修改当前用户的密码
// @Tags 认证
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param request body models.ChangePasswordRequest true "修改密码请求"
// @Success 200 {object} APIResponse
// @Failure 400 {object} APIResponse
// @Failure 401 {object} APIResponse
// @Router /auth/password [put]
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID := h.GetUserID(c)
	if userID == "" {
		h.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	// 将字符串ID转换为整数
	id, err := strconv.Atoi(userID)
	if err != nil {
		h.ErrorResponse(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	var req models.ChangePasswordRequest
	if !h.BindJSON(c, &req) {
		return
	}

	err = h.authService.ChangePassword(id, &req)
	if err != nil {
		h.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	h.SuccessResponse(c, gin.H{"message": "Password changed successfully"})
}
