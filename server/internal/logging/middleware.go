package logging

import (
	"strings"

	"github.com/gin-gonic/gin"
)

type auditRoute struct {
	Method       string
	PathPrefix   string
	PathSuffix   string // optional, for distinguishing same-prefix routes
	Action       string
	ResourceType string
}

var auditRoutes = []auditRoute{
	{"POST", "/api/v1/auth/login", "", "login", "auth"},
	{"PUT", "/api/v1/auth/password", "", "change_password", "auth"},
	{"POST", "/api/v1/alerts/rules", "", "create", "alert_rule"},
	{"PUT", "/api/v1/alerts/rules/", "", "update", "alert_rule"},
	{"DELETE", "/api/v1/alerts/rules/", "", "delete", "alert_rule"},
	{"PUT", "/api/v1/alerts/events/", "", "ack", "alert_event"},
	{"POST", "/api/v1/alerts/channels", "", "create", "channel"},
	{"PUT", "/api/v1/alerts/channels/", "", "update", "channel"},
	{"DELETE", "/api/v1/alerts/channels/", "", "delete", "channel"},
	{"POST", "/api/v1/alerts/channels/", "", "test", "channel"},
	{"POST", "/api/v1/probes", "", "create", "probe"},
	{"PUT", "/api/v1/probes/", "", "update", "probe"},
	{"DELETE", "/api/v1/probes/", "", "delete", "probe"},
	{"POST", "/api/v1/assets", "", "create", "asset"},
	{"PUT", "/api/v1/assets/", "", "update", "asset"},
	{"DELETE", "/api/v1/assets/", "", "delete", "asset"},
	{"POST", "/api/v1/cloud-accounts", "", "create", "cloud_account"},
	{"PUT", "/api/v1/cloud-accounts/", "", "update", "cloud_account"},
	{"DELETE", "/api/v1/cloud-accounts/", "", "delete", "cloud_account"},
	{"POST", "/api/v1/cloud-accounts/", "", "sync", "cloud_account"},
	{"POST", "/api/v1/managed-servers", "", "create", "managed_server"},
	{"DELETE", "/api/v1/managed-servers/", "", "delete", "managed_server"},
	{"POST", "/api/v1/managed-servers/", "", "deploy", "managed_server"},
	{"POST", "/api/v1/nas-devices/test", "", "test", "nas_device"},
	{"POST", "/api/v1/nas-devices", "", "create", "nas_device"},
	{"PUT", "/api/v1/nas-devices/", "", "update", "nas_device"},
	{"DELETE", "/api/v1/nas-devices/", "", "delete", "nas_device"},
	{"POST", "/api/v1/credentials", "", "create", "credential"},
	{"PUT", "/api/v1/credentials/", "", "update", "credential"},
	{"DELETE", "/api/v1/credentials/", "", "delete", "credential"},
	{"PUT", "/api/v1/servers/", "", "update", "server"},
	{"POST", "/api/v1/groups", "", "create", "group"},
	{"PUT", "/api/v1/groups/", "", "update", "group"},
	{"DELETE", "/api/v1/groups/", "", "delete", "group"},
	// AI analysis module
	{"POST", "/api/v1/ai/reports/generate", "", "generate", "ai_report"},
	{"DELETE", "/api/v1/ai/reports/", "", "delete", "ai_report"},
	{"POST", "/api/v1/ai/conversations", "", "create", "ai_conversation"},
	{"DELETE", "/api/v1/ai/conversations/", "", "delete", "ai_conversation"},
	{"PUT", "/api/v1/ai/settings", "", "update", "ai_settings"},
	{"PUT", "/api/v1/ai/schedules/", "", "update", "ai_schedule"},
	{"POST", "/api/v1/ai/providers/test", "", "test", "ai_provider"},
	// User management (suffix-disambiguated, more specific first)
	{"PUT", "/api/v1/users/", "/reset-pwd", "reset_pwd", "user"},
	{"PUT", "/api/v1/users/", "/permissions", "set_permissions", "user_permission"},
	{"POST", "/api/v1/users", "", "create", "user"},
	{"PUT", "/api/v1/users/", "", "update", "user"},
	{"DELETE", "/api/v1/users/", "", "delete", "user"},
}

func AuditMiddleware(mgr *LogManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next() // execute request first

		// Skip failed requests and GET
		if c.Writer.Status() >= 400 || c.Request.Method == "GET" || c.Request.Method == "OPTIONS" {
			return
		}

		path := c.Request.URL.Path
		method := c.Request.Method

		for _, r := range auditRoutes {
			if method != r.Method {
				continue
			}
			if !(r.PathPrefix == path || strings.HasPrefix(path, r.PathPrefix)) {
				continue
			}
			if r.PathSuffix != "" && !strings.HasSuffix(path, r.PathSuffix) {
				continue
			}

			username, _ := c.Get("username")
			usernameStr, _ := username.(string)

			// Special case: login sets audit_username from request body
			if r.ResourceType == "auth" && r.Action == "login" {
				if au, ok := c.Get("audit_username"); ok {
					usernameStr, _ = au.(string)
				}
			}

			resourceID := c.Param("id")

			mgr.Audit(
				usernameStr,
				r.Action,
				r.ResourceType,
				resourceID,
				"",
				"",
				c.ClientIP(),
				c.Request.UserAgent(),
			)
			return
		}
	}
}
