package api

import (
	"github.com/gin-gonic/gin"
	"opsboard/server/internal/store"
	"opsboard/server/internal/ws"
)

func SetupRouter(serverStore *store.ServerStore, hub *ws.Hub) *gin.Engine {
	r := gin.Default()

	// CORS for dev
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	v1 := r.Group("/api/v1")
	{
		dash := &DashboardHandler{serverStore: serverStore}
		v1.GET("/dashboard", dash.Overview)

		srv := &ServerHandler{store: serverStore}
		v1.GET("/servers", srv.List)
		v1.GET("/servers/:id", srv.Get)
	}

	r.GET("/ws", func(c *gin.Context) {
		hub.HandleWS(c.Writer, c.Request)
	})

	return r
}
