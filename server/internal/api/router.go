package api

import (
	"github.com/gin-gonic/gin"
	"opsboard/server/internal/store"
	"opsboard/server/internal/ws"
)

// RouterDeps holds all dependencies for setting up the HTTP router.
type RouterDeps struct {
	ServerStore     *store.ServerStore
	GroupStore      *store.GroupStore
	Hub             *ws.Hub
	MetricsProvider MetricsProvider
	StaticDir       string
	ProbeHandler    *ProbeHandler
	AssetHandler    *AssetHandler
	AuthHandler     *AuthHandler
	DatabaseHandler *DatabaseHandler
	BillingHandler  *BillingHandler
	AlertHandler    *AlertHandler
	GroupHandler    *GroupHandler
	// Will be extended later with:
	// CredentialHandler    *CredentialHandler
	// ManagedServerHandler *ManagedServerHandler
	// CloudHandler         *CloudHandler
}

func SetupRouter(deps RouterDeps) *gin.Engine {
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
	r.POST("/api/v1/auth/login", deps.AuthHandler.Login)

	// Protected API group
	v1 := r.Group("/api/v1")
	v1.Use(deps.AuthHandler.JWTMiddleware())
	{
		v1.GET("/auth/me", deps.AuthHandler.Me)

		dash := &DashboardHandler{serverStore: deps.ServerStore, metricsProvider: deps.MetricsProvider, groupStore: deps.GroupStore}
		v1.GET("/dashboard", dash.Overview)

		srv := &ServerHandler{store: deps.ServerStore}
		v1.GET("/servers", srv.List)
		v1.GET("/servers/:id", srv.Get)
		v1.PUT("/servers/:id/name", srv.UpdateDisplayName)
		v1.PUT("/servers/:id/group", deps.GroupHandler.SetServerGroup)

		// Groups
		v1.GET("/groups", deps.GroupHandler.List)
		v1.POST("/groups", deps.GroupHandler.Create)
		v1.PUT("/groups/:id", deps.GroupHandler.Update)
		v1.DELETE("/groups/:id", deps.GroupHandler.Delete)

		// Probes
		v1.GET("/probes", deps.ProbeHandler.List)
		v1.POST("/probes", deps.ProbeHandler.Create)
		v1.PUT("/probes/:id", deps.ProbeHandler.Update)
		v1.DELETE("/probes/:id", deps.ProbeHandler.Delete)
		v1.GET("/probes/status", deps.ProbeHandler.Status)

		// Assets
		v1.GET("/assets", deps.AssetHandler.List)
		v1.POST("/assets", deps.AssetHandler.Create)
		v1.PUT("/assets/:id", deps.AssetHandler.Update)
		v1.DELETE("/assets/:id", deps.AssetHandler.Delete)

		// Databases (RDS)
		v1.GET("/databases", deps.DatabaseHandler.List)
		v1.GET("/databases/:id", deps.DatabaseHandler.Get)

		// Billing
		v1.GET("/billing", deps.BillingHandler.List)

		// Alerts
		v1.GET("/alerts/rules", deps.AlertHandler.ListRules)
		v1.POST("/alerts/rules", deps.AlertHandler.CreateRule)
		v1.PUT("/alerts/rules/:id", deps.AlertHandler.UpdateRule)
		v1.DELETE("/alerts/rules/:id", deps.AlertHandler.DeleteRule)
		v1.GET("/alerts/events", deps.AlertHandler.ListEvents)
		v1.GET("/alerts/stats", deps.AlertHandler.GetStats)
		v1.PUT("/alerts/events/:id/ack", deps.AlertHandler.AckEvent)
		v1.GET("/alerts/events/:id/notifications", deps.AlertHandler.GetEventNotifications)
		v1.GET("/alerts/channels", deps.AlertHandler.ListChannels)
		v1.POST("/alerts/channels", deps.AlertHandler.CreateChannel)
		v1.PUT("/alerts/channels/:id", deps.AlertHandler.UpdateChannel)
		v1.DELETE("/alerts/channels/:id", deps.AlertHandler.DeleteChannel)
		v1.POST("/alerts/channels/:id/test", deps.AlertHandler.TestChannel)
	}

	r.GET("/ws", func(c *gin.Context) {
		deps.Hub.HandleWS(c.Writer, c.Request)
	})

	// 静态文件服务（前端 SPA）
	if deps.StaticDir != "" {
		r.Static("/assets", deps.StaticDir+"/assets")
		r.StaticFile("/favicon.svg", deps.StaticDir+"/favicon.svg")
		r.NoRoute(func(c *gin.Context) {
			c.File(deps.StaticDir + "/index.html")
		})
	}

	return r
}
