package api

import (
	"github.com/gin-gonic/gin"
	"opsboard/server/internal/store"
	"opsboard/server/internal/ws"
)

func SetupRouter(serverStore *store.ServerStore, hub *ws.Hub, probeHandler *ProbeHandler, assetHandler *AssetHandler, staticDir string) *gin.Engine {
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

		// Probes
		v1.GET("/probes", probeHandler.List)
		v1.POST("/probes", probeHandler.Create)
		v1.PUT("/probes/:id", probeHandler.Update)
		v1.DELETE("/probes/:id", probeHandler.Delete)
		v1.GET("/probes/status", probeHandler.Status)

		// Assets
		v1.GET("/assets", assetHandler.List)
		v1.POST("/assets", assetHandler.Create)
		v1.PUT("/assets/:id", assetHandler.Update)
		v1.DELETE("/assets/:id", assetHandler.Delete)
	}

	r.GET("/ws", func(c *gin.Context) {
		hub.HandleWS(c.Writer, c.Request)
	})

	// 静态文件服务（前端 SPA）
	if staticDir != "" {
		r.Static("/assets", staticDir+"/assets")
		r.StaticFile("/favicon.svg", staticDir+"/favicon.svg")
		r.NoRoute(func(c *gin.Context) {
			c.File(staticDir + "/index.html")
		})
	}

	return r
}
