package handler

import (
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

// AddKBMemberRequest is the body of POST /knowledge-bases/:id/members.
type AddKBMemberRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Role   string `json:"role"    binding:"required"`
}

// requireKBAdmin reports whether the caller may manage this KB's membership:
// workspace admins/owners, the KB owner/creator, or a kb_members admin.
func (h *KnowledgeBaseHandler) requireKBAdmin(c *gin.Context, kb *types.KnowledgeBase) bool {
	caller := types.CallerFromContext(c.Request.Context())
	if caller.Role == types.TenantRoleOwner || caller.Role == types.TenantRoleAdmin {
		return true
	}
	if caller.UserID == "" {
		return false
	}
	if kb.OwnerUserID == caller.UserID || kb.CreatorID == caller.UserID {
		return true
	}
	return h.service.KBMemberRole(c.Request.Context(), kb.ID, caller.UserID) == types.KBMemberAdmin
}

// ListKBMembers godoc
// @Summary      知识库成员列表
// @Description  返回知识库的显式成员授权（viewer/contributor/admin）
// @Tags         知识库
// @Produce      json
// @Param        id   path  string  true  "知识库ID"
// @Success      200  {object}  map[string]interface{}
// @Router       /knowledge-bases/{id}/members [get]
func (h *KnowledgeBaseHandler) ListKBMembers(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		c.Error(err)
		return
	}
	members, err := h.service.ListKBMembers(c.Request.Context(), kb.ID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": members})
}

// AddKBMember godoc
// @Summary      添加或更新知识库成员
// @Description  仅库管理员/创建者/空间管理员可操作
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Param        id    path  string               true  "知识库ID"
// @Param        body  body  AddKBMemberRequest   true  "成员与角色"
// @Success      200   {object}  map[string]interface{}
// @Router       /knowledge-bases/{id}/members [post]
func (h *KnowledgeBaseHandler) AddKBMember(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		c.Error(err)
		return
	}
	if !h.requireKBAdmin(c, kb) {
		c.Error(apperrors.NewForbiddenError("only knowledge base admins can manage members"))
		return
	}
	var req AddKBMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}
	member, err := h.service.UpsertKBMember(
		c.Request.Context(), kb.ID, req.UserID, types.KBMemberRole(req.Role))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": member})
}

// RemoveKBMember godoc
// @Summary      移除知识库成员
// @Tags         知识库
// @Produce      json
// @Param        id       path  string  true  "知识库ID"
// @Param        user_id  path  string  true  "用户ID"
// @Success      200      {object}  map[string]interface{}
// @Router       /knowledge-bases/{id}/members/{user_id} [delete]
func (h *KnowledgeBaseHandler) RemoveKBMember(c *gin.Context) {
	kb, _, _, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		c.Error(err)
		return
	}
	if !h.requireKBAdmin(c, kb) {
		c.Error(apperrors.NewForbiddenError("only knowledge base admins can manage members"))
		return
	}
	userID := c.Param("user_id")
	if userID == "" {
		c.Error(apperrors.NewBadRequestError("user_id is required"))
		return
	}
	if err := h.service.RemoveKBMember(c.Request.Context(), kb.ID, userID); err != nil {
		c.Error(err)
		return
	}
	c.JSON(200, gin.H{"success": true})
}
