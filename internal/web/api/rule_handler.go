package api

import (
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"

	"github.com/gin-gonic/gin"
)

// RuleHandler 规则处理器
type RuleHandler struct {
	*BaseHandler
	ruleService services.RuleService
}

// NewRuleHandler 创建规则处理器
func NewRuleHandler(ruleService services.RuleService) *RuleHandler {
	return &RuleHandler{
		BaseHandler: &BaseHandler{},
		ruleService: ruleService,
	}
}

// GetRules 获取规则列表
// @Summary 获取规则列表
// @Description 获取所有规则
// @Tags 规则管理
// @Security ApiKeyAuth
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页大小" default(20)
// @Success 200 {object} APIResponse{data=models.PagedResponse{data=[]models.Rule}}
// @Router /rules [get]
func (h *RuleHandler) GetRules(c *gin.Context) {
	log.Info().Msg("API: GetRules 调用开始")
	page, pageSize := h.GetPaginationParams(c)
	req := &models.RuleListRequest{Page: page, PageSize: pageSize}
	log.Info().Int("page", page).Int("page_size", pageSize).Msg("API: 分页参数")
	
	rules, total, err := h.ruleService.GetRules(req)
	if err != nil {
		log.Error().Err(err).Msg("API: GetRules 失败")
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	
	log.Info().Int("rules_count", len(rules)).Int("total", total).Msg("API: GetRules 成功")
	h.SuccessResponse(c, &models.PagedResponse{
		Data:       rules,
		Pagination: models.Pagination{Total: total, Offset: (page - 1) * pageSize, Limit: pageSize},
	})
}

// GetRule 获取单个规则
// @Summary 获取单个规则
// @Description 获取指定ID的规则
// @Tags 规则管理
// @Security ApiKeyAuth
// @Produce json
// @Param id path string true "规则ID"
// @Success 200 {object} APIResponse{data=models.Rule}
// @Router /rules/{id} [get]
func (h *RuleHandler) GetRule(c *gin.Context) {
	id := c.Param("id")
	rule, err := h.ruleService.GetRule(id)
	if err != nil {
		h.ErrorResponse(c, http.StatusNotFound, err.Error())
		return
	}
	h.SuccessResponse(c, rule)
}

// CreateRule 创建规则
// @Summary 创建规则
// @Description 创建一个新规则
// @Tags 规则管理
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param rule body models.RuleCreateRequest true "规则内容"
// @Success 201 {object} APIResponse{data=models.Rule}
// @Router /rules [post]
func (h *RuleHandler) CreateRule(c *gin.Context) {
	var req models.RuleCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.ErrorResponse(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	rule, err := h.ruleService.CreateRule(&req)
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.SuccessResponseWithCode(c, http.StatusCreated, rule)
}

// UpdateRule 更新规则
// @Summary 更新规则
// @Description 更新指定ID的规则
// @Tags 规则管理
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param id path string true "规则ID"
// @Param rule body models.RuleUpdateRequest true "规则内容"
// @Success 200 {object} APIResponse{data=models.Rule}
// @Router /rules/{id} [put]
func (h *RuleHandler) UpdateRule(c *gin.Context) {
	id := c.Param("id")
	var req models.RuleUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.ErrorResponse(c, http.StatusBadRequest, "Invalid request body")
		return
	}
	rule, err := h.ruleService.UpdateRule(id, &req)
	if err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.SuccessResponse(c, rule)
}

// DeleteRule 删除规则
// @Summary 删除规则
// @Description 删除指定ID的规则
// @Tags 规则管理
// @Security ApiKeyAuth
// @Produce json
// @Param id path string true "规则ID"
// @Success 204
// @Router /rules/{id} [delete]
func (h *RuleHandler) DeleteRule(c *gin.Context) {
	id := c.Param("id")
	if err := h.ruleService.DeleteRule(id); err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// EnableRule 启用规则
// @Summary 启用规则
// @Description 启用指定ID的规则
// @Tags 规则管理
// @Security ApiKeyAuth
// @Produce json
// @Param id path string true "规则ID"
// @Success 200 {object} APIResponse
// @Router /rules/{id}/enable [post]
func (h *RuleHandler) EnableRule(c *gin.Context) {
	id := c.Param("id")
	if err := h.ruleService.EnableRule(id); err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.SuccessResponse(c, gin.H{"status": "enabled"})
}

// DisableRule 禁用规则
// @Summary 禁用规则
// @Description 禁用指定ID的规则
// @Tags 规则管理
// @Security ApiKeyAuth
// @Produce json
// @Param id path string true "规则ID"
// @Success 200 {object} APIResponse
// @Router /rules/{id}/disable [post]
func (h *RuleHandler) DisableRule(c *gin.Context) {
	id := c.Param("id")
	if err := h.ruleService.DisableRule(id); err != nil {
		h.ErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.SuccessResponse(c, gin.H{"status": "disabled"})
}
