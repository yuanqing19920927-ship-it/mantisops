package api

import (
	"github.com/gin-gonic/gin"
	"opsboard/server/internal/store"
	"opsboard/server/internal/ws"
)

func SetupRouter(serverStore *store.ServerStore, hub *ws.Hub, probeHandler *ProbeHandler, assetHandler *AssetHandler, authHandler *AuthHandler, dbHandler *DatabaseHandler, billingHandler *BillingHandler, alertHandler *AlertHandler, groupHandler *GroupHandler, groupStore *store.GroupStore, metricsProvider MetricsProvider, staticDir string) *gin.Engine {
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

	// Public auth endpoint
	r.POST("/api/v1/auth/login", authHandler.Login)

	// Protected API group
	v1 := r.Group("/api/v1")
	v1.Use(authHandler.JWTMiddleware())
	{
		v1.GET("/auth/me", authHandler.Me)

		dash := &DashboardHandler{serverStore: serverStore, metricsProvider: metricsProvider, groupStore: groupStore}
		v1.GET("/dashboard", dash.Overview)

		srv := &ServerHandler{store: serverStore}
		v1.GET("/servers", srv.List)
		v1.GET("/servers/:id", srv.Get)
		v1.PUT("/servers/:id/name", srv.UpdateDisplayName)
		v1.PUT("/servers/:id/group", groupHandler.SetServerGroup)

		// Groups
		v1.GET("/groups", groupHandler.List)
		v1.POST("/groups", groupHandler.Create)
		v1.PUT("/groups/:id", groupHandler.Update)
		v1.DELETE("/groups/:id", groupHandler.Delete)

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

		// Databases (RDS)
		v1.GET("/databases", dbHandler.List)
		v1.GET("/databases/:id", dbHandler.Get)

		// Billing
		v1.GET("/billing", billingHandler.List)

		// Alerts
		v1.GET("/alerts/rules", alertHandler.ListRules)
		v1.POST("/alerts/rules", alertHandler.CreateRule)
		v1.PUT("/alerts/rules/:id", alertHandler.UpdateRule)
		v1.DELETE("/alerts/rules/:id", alertHandler.DeleteRule)
		v1.GET("/alerts/events", alertHandler.ListEvents)
		v1.GET("/alerts/stats", alertHandler.GetStats)
		v1.PUT("/alerts/events/:id/ack", alertHandler.AckEvent)
		v1.GET("/alerts/events/:id/notifications", alertHandler.GetEventNotifications)
		v1.GET("/alerts/channels", alertHandler.ListChannels)
		v1.POST("/alerts/channels", alertHandler.CreateChannel)
		v1.PUT("/alerts/channels/:id", alertHandler.UpdateChannel)
		v1.DELETE("/alerts/channels/:id", alertHandler.DeleteChannel)
		v1.POST("/alerts/channels/:id/test", alertHandler.TestChannel)
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
