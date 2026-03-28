package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

type UserHandler struct {
	userStore *store.UserStore
	tvCache   *TokenVersionCache
	permCache *PermissionCache
	hub       *ws.Hub
}

func NewUserHandler(us *store.UserStore, tv *TokenVersionCache, pc *PermissionCache, hub *ws.Hub) *UserHandler {
	return &UserHandler{userStore: us, tvCache: tv, permCache: pc, hub: hub}
}

func (h *UserHandler) List(c *gin.Context) {
	users, err := h.userStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

func (h *UserHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	user, err := h.userStore.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

type createUserRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role" binding:"required"`
}

func (h *UserHandler) Create(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Role != "admin" && req.Role != "operator" && req.Role != "viewer" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be admin, operator, or viewer"})
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hash failed"})
		return
	}

	id, err := h.userStore.Create(req.Username, hash, req.DisplayName, req.Role)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}

	user, _ := h.userStore.GetByID(id)
	c.JSON(http.StatusCreated, user)
}

type updateUserRequest struct {
	DisplayName string `json:"display_name"`
	Role        string `json:"role" binding:"required"`
	Enabled     bool   `json:"enabled"`
}

func (h *UserHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Role != "admin" && req.Role != "operator" && req.Role != "viewer" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be admin, operator, or viewer"})
		return
	}

	currentUserID := c.GetInt64("user_id")
	oldUser, err := h.userStore.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// System invariants
	if id == currentUserID {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot modify yourself via this endpoint"})
		return
	}
	if oldUser.Role == "admin" && (req.Role != "admin" || !req.Enabled) {
		count, _ := h.userStore.CountEnabledAdmins()
		if count <= 1 {
			c.JSON(http.StatusConflict, gin.H{"error": "must keep at least one enabled admin"})
			return
		}
	}

	if err := h.userStore.Update(id, req.DisplayName, req.Role, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Determine if token/permission invalidation needed
	roleChanged := oldUser.Role != req.Role
	enabledChanged := oldUser.Enabled != req.Enabled

	if roleChanged || enabledChanged {
		h.userStore.IncrementTokenVersion(id)
		h.tvCache.Invalidate(id)
		h.permCache.Invalidate(id)
	}

	user, _ := h.userStore.GetByID(id)
	c.JSON(http.StatusOK, user)
}

func (h *UserHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	currentUserID := c.GetInt64("user_id")
	if id == currentUserID {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot delete yourself"})
		return
	}

	user, err := h.userStore.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if user.Role == "admin" {
		count, _ := h.userStore.CountEnabledAdmins()
		if count <= 1 {
			c.JSON(http.StatusConflict, gin.H{"error": "must keep at least one enabled admin"})
			return
		}
	}

	h.userStore.IncrementTokenVersion(id)
	h.tvCache.Invalidate(id)
	h.permCache.Invalidate(id)

	if err := h.userStore.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type resetPwdRequest struct {
	Password string `json:"password" binding:"required"`
}

func (h *UserHandler) ResetPassword(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req resetPwdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hash failed"})
		return
	}

	if err := h.userStore.ResetPassword(id, hash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.userStore.IncrementTokenVersion(id)
	h.tvCache.Invalidate(id)

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *UserHandler) GetPermissions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	perms, err := h.userStore.GetPermissions(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if perms == nil {
		perms = []store.Permission{}
	}
	c.JSON(http.StatusOK, gin.H{"permissions": perms})
}

type setPermissionsRequest struct {
	Permissions []store.Permission `json:"permissions"`
}

func (h *UserHandler) SetPermissions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req setPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.userStore.SetPermissions(id, req.Permissions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.permCache.Invalidate(id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
