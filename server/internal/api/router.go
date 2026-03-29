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
	ScanHandler          *ScanHandler
	UserHandler          *UserHandler
	PermissionCache      *PermissionCache
	AIHandler            *AIHandler
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

	// Protected API group (all authenticated users)
	v1 := r.Group("/api/v1")
	v1.Use(deps.AuthHandler.JWTMiddleware())
	{
		// Auth self-service (any role, including must_change_pwd)
		v1.GET("/auth/me", deps.AuthHandler.Me)
		v1.PUT("/auth/password", deps.AuthHandler.ChangePassword)

		// --- Viewer level (default: all GET queries) ---
		dash := &DashboardHandler{serverStore: deps.ServerStore, metricsProvider: deps.MetricsProvider, groupStore: deps.GroupStore, permCache: deps.PermissionCache}
		v1.GET("/dashboard", dash.Overview)

		srv := &ServerHandler{store: deps.ServerStore, permCache: deps.PermissionCache}
		v1.GET("/servers", srv.List)
		v1.GET("/servers/:id", srv.Get)

		v1.GET("/groups", deps.GroupHandler.List)

		v1.GET("/probes", deps.ProbeHandler.List)
		v1.GET("/probes/status", deps.ProbeHandler.Status)

		v1.GET("/assets", deps.AssetHandler.List)
		v1.GET("/assets/discovered", deps.AssetHandler.ListDiscovered)

		// Scan templates (read)
		if deps.ScanHandler != nil {
			v1.GET("/scan-templates", deps.ScanHandler.ListTemplates)
		}

		v1.GET("/databases", deps.DatabaseHandler.List)
		v1.GET("/databases/:id", deps.DatabaseHandler.Get)

		v1.GET("/billing", deps.BillingHandler.List)

		v1.GET("/alerts/rules", deps.AlertHandler.ListRules)
		v1.GET("/alerts/events", deps.AlertHandler.ListEvents)
		v1.GET("/alerts/stats", deps.AlertHandler.GetStats)
		v1.GET("/alerts/events/:id/notifications", deps.AlertHandler.GetEventNotifications)

		// Logs (viewer can see runtime, admin-only for audit)
		if deps.LogHandler != nil {
			v1.GET("/logs/runtime", deps.LogHandler.ListRuntime)
			v1.GET("/logs/export", deps.LogHandler.Export)
			v1.GET("/logs/sources", deps.LogHandler.Sources)
			v1.GET("/logs/stats", deps.LogHandler.Stats)
		}

		// NAS read
		if deps.NasHandler != nil {
			v1.GET("/nas-devices", deps.NasHandler.List)
			v1.GET("/nas-devices/:id/metrics", deps.NasHandler.GetMetrics)
		}

		// AI analysis module (read-only for all authenticated users)
		if deps.AIHandler != nil {
			v1.GET("/ai/reports", deps.AIHandler.ListReports)
			v1.GET("/ai/reports/latest", deps.AIHandler.LatestReport)
			v1.GET("/ai/reports/:id", deps.AIHandler.GetReport)
			v1.GET("/ai/reports/:id/download", deps.AIHandler.DownloadReport)
			v1.GET("/ai/conversations", deps.AIHandler.ListConversations)
			v1.GET("/ai/conversations/:id", deps.AIHandler.GetConversation)
			v1.GET("/ai/settings", deps.AIHandler.GetAISettings)
			v1.GET("/ai/providers", deps.AIHandler.ListProviders)
			v1.GET("/ai/schedules", deps.AIHandler.ListSchedules)
		}

		// --- Operator level ---
		op := v1.Group("")
		op.Use(RequireRole("operator"))
		{
			// Sensitive GET (contains Webhook URLs etc.)
			op.GET("/alerts/channels", deps.AlertHandler.ListChannels)

			// Probes CRUD
			op.POST("/probes", deps.ProbeHandler.Create)
			op.PUT("/probes/:id", deps.ProbeHandler.Update)
			op.DELETE("/probes/:id", deps.ProbeHandler.Delete)

			// Assets CRUD
			op.POST("/assets", deps.AssetHandler.Create)
			op.PUT("/assets/:id", deps.AssetHandler.Update)
			op.DELETE("/assets/:id", deps.AssetHandler.Delete)

			// Alert rules CRUD
			op.POST("/alerts/rules", deps.AlertHandler.CreateRule)
			op.PUT("/alerts/rules/:id", deps.AlertHandler.UpdateRule)
			op.DELETE("/alerts/rules/:id", deps.AlertHandler.DeleteRule)

			// Alert ack
			op.PUT("/alerts/events/:id/ack", deps.AlertHandler.AckEvent)

			// Notification channels CRUD
			op.POST("/alerts/channels", deps.AlertHandler.CreateChannel)
			op.PUT("/alerts/channels/:id", deps.AlertHandler.UpdateChannel)
			op.DELETE("/alerts/channels/:id", deps.AlertHandler.DeleteChannel)
			op.POST("/alerts/channels/:id/test", deps.AlertHandler.TestChannel)

			// AI write operations (operator+)
			if deps.AIHandler != nil {
				op.POST("/ai/reports/generate", deps.AIHandler.GenerateReport)
				op.DELETE("/ai/reports/:id", deps.AIHandler.DeleteReport)
				op.POST("/ai/conversations", deps.AIHandler.CreateConversation)
				op.DELETE("/ai/conversations/:id", deps.AIHandler.DeleteConversation)
				op.POST("/ai/conversations/:id/messages", deps.AIHandler.SendMessage)
			}
		}

		// --- Admin level ---
		adm := v1.Group("")
		adm.Use(RequireRole("admin"))
		{
			// Server management
			adm.PUT("/servers/:id/name", srv.UpdateDisplayName)
			adm.PUT("/servers/:id/group", deps.GroupHandler.SetServerGroup)
			adm.PUT("/servers/:id/config", srv.UpdateConfig)

			// Groups management
			adm.POST("/groups", deps.GroupHandler.Create)
			adm.PUT("/groups/:id", deps.GroupHandler.Update)
			adm.DELETE("/groups/:id", deps.GroupHandler.Delete)
			adm.PUT("/groups-sort", deps.GroupHandler.BatchSortGroups)
			adm.PUT("/servers-sort", deps.GroupHandler.BatchSortServers)

			// Platform settings
			if deps.SettingsHandler != nil {
				adm.PUT("/settings", deps.SettingsHandler.Update)
			}

			// AI admin operations (settings, schedules, provider test)
			if deps.AIHandler != nil {
				adm.PUT("/ai/settings", deps.AIHandler.UpdateAISettings)
				adm.PUT("/ai/schedules/:id", deps.AIHandler.UpdateSchedule)
				adm.POST("/ai/providers/test", deps.AIHandler.TestProvider)
				adm.GET("/ai/prompts", deps.AIHandler.GetPrompts)
				adm.PUT("/ai/prompts", deps.AIHandler.UpdatePrompts)
			}

			// Audit logs (admin only)
			if deps.LogHandler != nil {
				adm.GET("/logs/audit", deps.LogHandler.ListAudit)
			}

			// User management
			if deps.UserHandler != nil {
				adm.GET("/users", deps.UserHandler.List)
				adm.GET("/users/:id", deps.UserHandler.Get)
				adm.POST("/users", deps.UserHandler.Create)
				adm.PUT("/users/:id", deps.UserHandler.Update)
				adm.DELETE("/users/:id", deps.UserHandler.Delete)
				adm.PUT("/users/:id/reset-pwd", deps.UserHandler.ResetPassword)
				adm.GET("/users/:id/permissions", deps.UserHandler.GetPermissions)
				adm.PUT("/users/:id/permissions", deps.UserHandler.SetPermissions)
			}

			// Credentials
			if deps.CredentialHandler != nil {
				adm.GET("/credentials", deps.CredentialHandler.List)
				adm.POST("/credentials", deps.CredentialHandler.Create)
				adm.PUT("/credentials/:id", deps.CredentialHandler.Update)
				adm.DELETE("/credentials/:id", deps.CredentialHandler.Delete)
			}

			// Managed servers
			if deps.ManagedServerHandler != nil {
				adm.GET("/managed-servers", deps.ManagedServerHandler.List)
				adm.POST("/managed-servers", deps.ManagedServerHandler.Create)
				adm.POST("/managed-servers/test-connection", deps.ManagedServerHandler.TestConnection)
				adm.POST("/managed-servers/:id/deploy", deps.ManagedServerHandler.Deploy)
				adm.POST("/managed-servers/:id/retry", deps.ManagedServerHandler.Retry)
				adm.DELETE("/managed-servers/:id", deps.ManagedServerHandler.Delete)
				adm.POST("/managed-servers/:id/uninstall", deps.ManagedServerHandler.Uninstall)
			}

			// Scan templates + scan (admin only)
			if deps.ScanHandler != nil {
				adm.POST("/scan-templates", deps.ScanHandler.CreateTemplate)
				adm.PUT("/scan-templates/:id", deps.ScanHandler.UpdateTemplate)
				adm.DELETE("/scan-templates/:id", deps.ScanHandler.DeleteTemplate)
				adm.POST("/probes/scan", deps.ScanHandler.StartScan)
			}

			// NAS management
			if deps.NasHandler != nil {
				adm.POST("/nas-devices/test", deps.NasHandler.TestConnection)
				adm.POST("/nas-devices", deps.NasHandler.Create)
				adm.PUT("/nas-devices/:id", deps.NasHandler.Update)
				adm.DELETE("/nas-devices/:id", deps.NasHandler.Delete)
			}

			// Cloud accounts & instances
			if deps.CloudHandler != nil {
				adm.GET("/cloud-accounts", deps.CloudHandler.ListAccounts)
				adm.POST("/cloud-accounts", deps.CloudHandler.CreateAccount)
				adm.POST("/cloud-accounts/verify", deps.CloudHandler.Verify)
				adm.PUT("/cloud-accounts/:id", deps.CloudHandler.UpdateAccount)
				adm.DELETE("/cloud-accounts/:id", deps.CloudHandler.DeleteAccount)
				adm.POST("/cloud-accounts/:id/sync", deps.CloudHandler.SyncAccount)
				adm.GET("/cloud-accounts/:id/instances", deps.CloudHandler.ListInstances)
				adm.POST("/cloud-instances/confirm", deps.CloudHandler.ConfirmInstances)
				adm.PUT("/cloud-instances/:id", deps.CloudHandler.UpdateInstance)
				adm.POST("/cloud-instances", deps.CloudHandler.AddInstance)
				adm.DELETE("/cloud-instances/:id", deps.CloudHandler.DeleteInstance)
			}
		}
	}

	// WebSocket（通过 query param token 鉴权，携带角色和权限）
	r.GET("/ws", func(c *gin.Context) {
		token := c.Query("token")
		if token == "" {
			c.JSON(401, gin.H{"error": "missing token"})
			return
		}
		payload, err := deps.AuthHandler.ValidateToken(token)
		if err != nil {
			c.JSON(401, gin.H{"error": "invalid token"})
			return
		}
		// Check token version (same as JWTMiddleware)
		currentVersion, err := deps.AuthHandler.tvCache.Get(payload.UserID)
		if err != nil || payload.TokenVersion < currentVersion {
			c.JSON(401, gin.H{"error": "token revoked"})
			return
		}
		// Build permission checker for non-admin users
		var perm ws.PermChecker
		if payload.Role != "admin" && deps.PermissionCache != nil {
			ps, err := deps.PermissionCache.Get(payload.UserID)
			if err != nil {
				ps = &PermissionSet{} // empty = see nothing
			}
			perm = ps
		}
		deps.Hub.HandleWSWithAuth(c.Writer, c.Request, payload.UserID, payload.Role, perm)
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
