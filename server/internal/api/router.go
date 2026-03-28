package api

import (
	"github.com/gin-gonic/gin"
	"mantisops/server/internal/logging"
	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
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
	AlertHandler      *AlertHandler
	GroupHandler      *GroupHandler
	CredentialHandler *CredentialHandler
	CloudHandler         *CloudHandler
	ManagedServerHandler *ManagedServerHandler
	NasHandler           *NasHandler
	LogHandler           *LogHandler
	LogManager           *logging.LogManager
	SettingsHandler      *SettingsHandler
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

	// Audit middleware (after CORS, before routes)
	if deps.LogManager != nil {
		r.Use(logging.AuditMiddleware(deps.LogManager))
	}

	// Public endpoints
	r.POST("/api/v1/auth/login", deps.AuthHandler.Login)
	if deps.SettingsHandler != nil {
		r.GET("/api/v1/settings", deps.SettingsHandler.Get)
	}

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
		v1.PUT("/servers/:id/config", srv.UpdateConfig)

		// Groups
		v1.GET("/groups", deps.GroupHandler.List)
		v1.POST("/groups", deps.GroupHandler.Create)
		v1.PUT("/groups/:id", deps.GroupHandler.Update)
		v1.DELETE("/groups/:id", deps.GroupHandler.Delete)
		v1.PUT("/groups-sort", deps.GroupHandler.BatchSortGroups)
		v1.PUT("/servers-sort", deps.GroupHandler.BatchSortServers)

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

		// Credentials
		if deps.CredentialHandler != nil {
			v1.GET("/credentials", deps.CredentialHandler.List)
			v1.POST("/credentials", deps.CredentialHandler.Create)
			v1.PUT("/credentials/:id", deps.CredentialHandler.Update)
			v1.DELETE("/credentials/:id", deps.CredentialHandler.Delete)
		}

		// Managed servers
		if deps.ManagedServerHandler != nil {
			v1.GET("/managed-servers", deps.ManagedServerHandler.List)
			v1.POST("/managed-servers", deps.ManagedServerHandler.Create)
			v1.POST("/managed-servers/test-connection", deps.ManagedServerHandler.TestConnection)
			v1.POST("/managed-servers/:id/deploy", deps.ManagedServerHandler.Deploy)
			v1.POST("/managed-servers/:id/retry", deps.ManagedServerHandler.Retry)
			v1.DELETE("/managed-servers/:id", deps.ManagedServerHandler.Delete)
			v1.POST("/managed-servers/:id/uninstall", deps.ManagedServerHandler.Uninstall)
		}

		// Platform settings (GET is public, PUT requires auth)
		if deps.SettingsHandler != nil {
			v1.PUT("/settings", deps.SettingsHandler.Update)
		}

		// Logs
		if deps.LogHandler != nil {
			v1.GET("/logs/audit", deps.LogHandler.ListAudit)
			v1.GET("/logs/runtime", deps.LogHandler.ListRuntime)
			v1.GET("/logs/export", deps.LogHandler.Export)
			v1.GET("/logs/sources", deps.LogHandler.Sources)
			v1.GET("/logs/stats", deps.LogHandler.Stats)
		}

		// NAS devices
		if deps.NasHandler != nil {
			v1.POST("/nas-devices/test", deps.NasHandler.TestConnection)
			v1.GET("/nas-devices", deps.NasHandler.List)
			v1.POST("/nas-devices", deps.NasHandler.Create)
			v1.PUT("/nas-devices/:id", deps.NasHandler.Update)
			v1.DELETE("/nas-devices/:id", deps.NasHandler.Delete)
			v1.GET("/nas-devices/:id/metrics", deps.NasHandler.GetMetrics)
		}

		// Cloud accounts & instances
		if deps.CloudHandler != nil {
			v1.GET("/cloud-accounts", deps.CloudHandler.ListAccounts)
			v1.POST("/cloud-accounts", deps.CloudHandler.CreateAccount)
			v1.POST("/cloud-accounts/verify", deps.CloudHandler.Verify)
			v1.PUT("/cloud-accounts/:id", deps.CloudHandler.UpdateAccount)
			v1.DELETE("/cloud-accounts/:id", deps.CloudHandler.DeleteAccount)
			v1.POST("/cloud-accounts/:id/sync", deps.CloudHandler.SyncAccount)
			v1.GET("/cloud-accounts/:id/instances", deps.CloudHandler.ListInstances)
			v1.POST("/cloud-instances/confirm", deps.CloudHandler.ConfirmInstances)
			v1.PUT("/cloud-instances/:id", deps.CloudHandler.UpdateInstance)
			v1.POST("/cloud-instances", deps.CloudHandler.AddInstance)
			v1.DELETE("/cloud-instances/:id", deps.CloudHandler.DeleteInstance)
		}
	}

	// WebSocket（通过 query param token 鉴权）
	r.GET("/ws", func(c *gin.Context) {
		token := c.Query("token")
		if token == "" {
			c.JSON(401, gin.H{"error": "missing token"})
			return
		}
		if _, err := deps.AuthHandler.ValidateToken(token); err != nil {
			c.JSON(401, gin.H{"error": "invalid token"})
			return
		}
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
