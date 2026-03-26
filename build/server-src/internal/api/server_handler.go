package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"opsboard/server/internal/store"
)

type ServerHandler struct {
	store *store.ServerStore
}

func (h *ServerHandler) List(c *gin.Context) {
	servers, err := h.store.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, servers)
}

func (h *ServerHandler) Get(c *gin.Context) {
	id := c.Param("id")
	srv, err := h.store.GetByHostID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	c.JSON(http.StatusOK, srv)
}
