package main

import (
	"crypto/rsa"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"wisdomHouse-backend/internal/apperror"
	"wisdomHouse-backend/internal/authutil"
	"wisdomHouse-backend/internal/cache"
	"wisdomHouse-backend/internal/config"
	"wisdomHouse-backend/internal/email"
	"wisdomHouse-backend/internal/handlers"
	"wisdomHouse-backend/internal/metrics"
	"wisdomHouse-backend/internal/middleware"
	"wisdomHouse-backend/internal/repository"
)

func setupRouter(
	cfg *config.Config,
	userRepo repository.UserRepository,
	userCache *cache.UserCache,
	rsaPublicKey *rsa.PublicKey,
	tokenBlocklist *authutil.TokenBlocklist,
	healthHandler *handlers.HealthHandler,
	testimonialHandler *handlers.TestimonialHandler,
	authHandler *handlers.AuthHandler,
	adminHandler *handlers.AdminHandler,
	adminEmailHandler *handlers.AdminEmailHandler,
	uploadHandler *handlers.UploadHandler,
	assetHandler *handlers.AssetHandler,
	eventHandler *handlers.EventHandler,
	adminNotificationHandler *handlers.AdminNotificationHandler,
	approvalRequestHandler *handlers.ApprovalRequestHandler,
	reelHandler *handlers.ReelHandler,
	analyticsHandler *handlers.AnalyticsHandler,
	formHandler *handlers.FormHandler,
	notificationHandler *handlers.NotificationHandler,
	otpHandler *handlers.OTPHandler,
	workforceHandler *handlers.WorkforceHandler,
	leadershipHandler *handlers.LeadershipHandler,
	memberHandler *handlers.MemberHandler,
	emailTemplateHandler *handlers.EmailTemplateHandler,
	emailTemplateRegistryHandler *handlers.EmailTemplateRegistryHandler,
	sermonHandler *handlers.SermonHandler,
	storeHandler *handlers.StoreHandler,
	givingHandler *handlers.GivingHandler,
	siteContentHandler *handlers.SiteContentHandler,
	engagementHandler *handlers.EngagementHandler,
	givingPaymentsHandler *handlers.GivingPaymentsHandler,
	attendanceHandler *handlers.AttendanceHandler,
	cellGroupHandler *handlers.CellGroupHandler,
	prayerRequestHandler *handlers.PrayerRequestHandler,
	ministryHandler *handlers.MinistryHandler,
	sseHandler *handlers.SSEHandler,
	auditLogRepo repository.AuditLogRepository,
) *gin.Engine {
	router := gin.New()
	secure := strings.TrimSpace(cfg.App.Environment) == "production"

	router.Use(otelgin.Middleware("wisdomhouse-backend"))
	router.Use(metrics.GinMiddleware())
	router.Use(gin.CustomRecovery(apperror.PanicRecoveryHandler))
	router.Use(apperror.Handler())
	// CORS must run before any middleware capable of aborting the request
	// (body-size limit, rate limiter, etc.), otherwise an aborted request's
	// error response goes out without CORS headers and the browser reports
	// a misleading "CORS policy" error instead of the real HTTP status.
	router.Use(middleware.CORS(&cfg.CORS))
	router.Use(middleware.RequestID())
	router.Use(middleware.CampusContext())
	router.Use(middleware.Logger(cfg.App.LogLevel))
	router.Use(middleware.SecurityHeaders(secure))
	router.Use(middleware.RequestBodyLimit(cfg.Server.RequestBodyMax))
	router.Use(middleware.RateLimiter(middleware.RateLimiterOptions{
		RequestsPerMinute: cfg.RateLimit.Global.RequestsPerMinute,
		Burst:             cfg.RateLimit.Global.Burst,
		Window:            cfg.RateLimit.Global.Window,
		RedisURL:          cfg.Redis.URL,
		Prefix:            "rl",
		Message:           "Too many requests. Please wait a moment and try again.",
		SkipPathPrefixes:  []string{"/api/v1/auth/login", "/api/v1/auth/otp/", "/api/v1/auth/me", "/api/v1/auth/mfa"},
	}))

	// Health
	router.GET("/healthz", healthHandler.Liveness)
	router.GET("/readyz", healthHandler.Readiness)

	// Brand assets embedded in the binary (e.g. the logo referenced by email
	// templates) — public, unauthenticated, since email clients fetch images
	// with no session context.
	router.GET(email.LogoAssetPath, func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=86400")
		c.Data(http.StatusOK, email.LogoContentType, email.LogoBytes)
	})
	router.GET("/forms/:slug", formHandler.ViewPublicFormPage)
	router.GET("/form/:slug", formHandler.RedirectLegacyPublicFormPage)
	router.GET("/reports/forms/:slug", formHandler.ViewPublicFormReport)
	router.GET("/reports/forms/:slug/data", formHandler.GetPublicFormReportData)
	router.GET("/reports/forms/:slug/export.pdf", formHandler.ExportPublicFormReportPDF)

	api := router.Group("/api/v1")

	authGuard := middleware.AuthMiddleware(middleware.AuthMiddlewareOptions{
		JWTSecret:    cfg.JWT.Secret,
		RSAPublicKey: rsaPublicKey,
		Blocklist:    tokenBlocklist,
	})
	sessionFreshnessGuard := middleware.SessionFreshnessMiddleware(userRepo, userCache)
	sessionGuard := middleware.SessionTimeout(cfg.Auth.SessionIdleTimeout, cfg.Auth.RememberedSessionIdleTimeout, secure)
	csrfProtector := middleware.NewCSRFProtector(middleware.CSRFOptions{
		SecretKey:  cfg.Auth.SecretKey,
		Secure:     secure,
		CookieName: cfg.Auth.CSRFCookieName,
		HeaderName: cfg.Auth.CSRFHeaderName,
		CookieTTL:  cfg.Auth.CSRFCookieTTL,
	})

	// AUTH
	auth := api.Group("/auth")
	auth.Use(middleware.DeviceFingerprint(secure))
	auth.Use(middleware.NoStore())
	loginRateLimiter := middleware.RateLimiter(middleware.RateLimiterOptions{
		RequestsPerMinute: cfg.RateLimit.Auth.RequestsPerMinute,
		Burst:             cfg.RateLimit.Auth.Burst,
		Window:            cfg.RateLimit.Auth.Window,
		RedisURL:          cfg.Redis.URL,
		Prefix:            "rl:login",
		Message:           "Too many authentication attempts. Please wait a moment and try again.",
	})

	auth.POST("/login", loginRateLimiter, authHandler.Login)
	auth.POST("/login/verify-otp", loginRateLimiter, authHandler.VerifyLoginOTP)
	auth.POST("/login/resend-otp", loginRateLimiter, authHandler.ResendLoginOTP)
	auth.POST("/register", loginRateLimiter, authHandler.Register)
	auth.POST("/password-reset/request", loginRateLimiter, authHandler.RequestPasswordReset)
	auth.POST("/password-reset/confirm", loginRateLimiter, authHandler.ConfirmPasswordReset)
	auth.POST("/otp/verify", loginRateLimiter, authHandler.VerifyLoginOTP)
	auth.POST("/otp/resend", loginRateLimiter, authHandler.ResendLoginOTP)
	auth.GET("/oauth/google/start", authHandler.StartGoogleOAuth)
	auth.GET("/oauth/google/callback", authHandler.HandleGoogleOAuthCallback)
	auth.GET("/me", middleware.OptionalAuthMiddleware(middleware.AuthMiddlewareOptions{
		JWTSecret:    cfg.JWT.Secret,
		RSAPublicKey: rsaPublicKey,
		Blocklist:    tokenBlocklist,
	}), authHandler.GetCurrentUser)
	auth.POST("/token/refresh", authHandler.RotateRefreshToken)

	authProtected := auth.Group("")
	authProtected.Use(authGuard, sessionFreshnessGuard, sessionGuard, csrfProtector.Middleware(), middleware.AuditLogger("auth", auditLogRepo))
	authProtected.GET("/csrf-token", authHandler.GetCSRFToken)
	authProtected.PATCH("/profile", authHandler.UpdateProfile)
	authProtected.POST("/change-password", authHandler.ChangePassword)
	authProtected.DELETE("/account", authHandler.DeleteAccount)
	authProtected.POST("/clear-data", authHandler.ClearData)
	authProtected.POST("/refresh", authHandler.RefreshToken)
	authProtected.POST("/logout", authHandler.Logout)
	authProtected.GET("/mfa", authHandler.GetMFASecurityProfile)
	authProtected.POST("/mfa/totp/setup", authHandler.BeginTOTPSetup)
	authProtected.POST("/mfa/totp/enable", authHandler.EnableTOTP)
	authProtected.POST("/mfa/totp/disable", authHandler.DisableTOTP)
	authProtected.PATCH("/mfa/method", authHandler.SetPreferredMFAMethod)

	// OTP
	otpRateLimiter := middleware.RateLimiter(middleware.RateLimiterOptions{
		RequestsPerMinute: cfg.RateLimit.Auth.RequestsPerMinute,
		Burst:             cfg.RateLimit.Auth.Burst,
		Window:            cfg.RateLimit.Auth.Window,
		RedisURL:          cfg.Redis.URL,
		Prefix:            "rl:otp",
		Message:           "Too many OTP requests. Please wait before trying again.",
	})
	api.POST("/otp/send", middleware.NoStore(), otpRateLimiter, otpHandler.SendOTP)
	api.POST("/otp/verify", middleware.NoStore(), otpRateLimiter, otpHandler.VerifyOTP)

	// Public testimonials
	api.GET("/testimonials", testimonialHandler.GetPaginatedTestimonials)
	api.GET("/testimonials/all", testimonialHandler.GetAllTestimonials)
	api.GET("/testimonials/:id", testimonialHandler.GetTestimonialByID)
	api.POST("/testimonials", testimonialHandler.CreateTestimonial)

	// Public events/reels
	api.GET("/events", eventHandler.List)
	api.GET("/events/:id", eventHandler.Get)
	api.GET("/reels", reelHandler.List)
	api.GET("/sermons", sermonHandler.List)
	api.POST("/analytics/events", analyticsHandler.IngestEvents)

	// Notifications (newsletter-style)
	api.POST("/notifications/subscribe", notificationHandler.Subscribe)
	api.GET("/notifications/subscribe", notificationHandler.SubscribeByLink)
	api.POST("/notifications/unsubscribe", notificationHandler.Unsubscribe)
	api.GET("/notifications/unsubscribe", notificationHandler.UnsubscribeByLink)

	// Public store APIs (backend-driven)
	api.GET("/store/products", storeHandler.ListProducts)
	api.POST("/store/orders", storeHandler.CreateOrder)
	api.GET("/store/orders/:orderId", storeHandler.GetOrder)
	// PRAYER REQUESTS — public submission (aggressive rate limit applied at infra level)
	api.POST("/prayer-requests", prayerRequestHandler.Submit)

	api.GET("/giving/options", givingHandler.ListOptions)
	api.GET("/giving/categories", givingPaymentsHandler.ListCategories)
	api.POST("/giving/initiate/:provider", givingPaymentsHandler.Initiate)
	api.GET("/giving/verify/:provider/:reference", givingPaymentsHandler.Verify)
	api.POST("/giving/webhook/:provider", givingPaymentsHandler.Webhook)
	api.POST("/giving/intents", engagementHandler.CreateGivingIntent)
	api.POST("/pastoral-care/requests", engagementHandler.CreatePastoralCareRequest)
	api.POST("/contact/messages", engagementHandler.CreateContactMessage)
	api.GET("/content/homepage-ad", siteContentHandler.GetHomepageAd)
	api.GET("/content/confession-popup", siteContentHandler.GetConfessionPopup)

	// Public forms
	api.GET("/forms/:slug", formHandler.GetPublicForm)
	api.POST("/forms/:slug/submissions", formHandler.SubmitPublicForm)
	api.GET("/forms/:slug/report", formHandler.ViewPublicFormReport)
	api.GET("/forms/:slug/report/data", formHandler.GetPublicFormReportData)
	api.GET("/forms/:slug/report/export.pdf", formHandler.ExportPublicFormReportPDF)
	api.GET("/forms/:slug/calendar/confirm", formHandler.ConfirmCalendarOptIn)
	api.GET("/forms/:slug/calendar.ics", formHandler.DownloadCalendarICS)

	// Workforce public apply
	workforceLookupRateLimiter := middleware.RateLimiter(middleware.RateLimiterOptions{
		RequestsPerMinute: cfg.RateLimit.Auth.RequestsPerMinute,
		Burst:             cfg.RateLimit.Auth.Burst,
		Window:            cfg.RateLimit.Auth.Window,
		RedisURL:          cfg.Redis.URL,
		Prefix:            "rl:workforce-lookup",
		Message:           "Too many lookup requests. Please wait a moment and try again.",
	})
	api.GET("/workforce/member/lookup", workforceLookupRateLimiter, workforceHandler.LookupByEmail)
	api.POST("/workforce/apply", workforceHandler.Apply)
	api.POST("/workforce/serving/register", workforceHandler.ApplyServing)

	// Leadership public
	api.GET("/leadership", leadershipHandler.ListPublic)
	api.POST("/leadership/apply", leadershipHandler.Apply)
	api.POST("/leadership/upload-image", leadershipHandler.UploadImage)
	api.POST("/leadership/upload", leadershipHandler.UploadImage)

	// Universal public uploads
	api.POST("/uploads", uploadHandler.UploadFile)
	api.POST("/uploads/files", uploadHandler.UploadFile)
	api.POST("/uploads/images", uploadHandler.UploadImage)

	// ADMIN
	admin := api.Group("/admin")
	admin.Use(
		authGuard,
		sessionFreshnessGuard,
		sessionGuard,
		middleware.RoleMiddleware("admin"),
		middleware.RequireAdminMFA(userRepo),
		middleware.RequirePermission(middleware.PermissionAdminAccess),
		csrfProtector.Middleware(),
		middleware.AuditLogger("admin", auditLogRepo),
	)

	auditLogsHandler := func(c *gin.Context) {
		page, err := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("page", "1")))
		if err != nil || page < 1 {
			page = 1
		}
		limit, err := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "50")))
		if err != nil || limit < 1 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}
		scope := strings.TrimSpace(c.Query("scope"))

		if auditLogRepo == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Audit log storage is not configured"})
			return
		}

		logs, total, err := auditLogRepo.List(c.Request.Context(), page, limit, scope)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to load audit logs"})
			return
		}

		totalPages := 0
		if limit > 0 {
			totalPages = int((total + int64(limit) - 1) / int64(limit))
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "Audit logs loaded",
			"data":    logs,
			"total":   total,
			"page":    page,
			"limit":   limit,
			"meta": gin.H{
				"page": page, "limit": limit,
				"total_items": total, "total_pages": totalPages,
				"has_next": page < totalPages, "has_prev": page > 1,
			},
		})
	}

	admin.GET("/dashboard", middleware.RequirePermission(middleware.PermissionAdminRead), adminHandler.GetDashboardStats)
	admin.GET("/security/overview", middleware.RequirePermission(middleware.PermissionSecurityRead), adminHandler.GetSecurityOverview)
	admin.GET("/testimonials/pending", middleware.RequirePermission(middleware.PermissionAdminRead), adminHandler.GetPendingTestimonials)
	admin.PUT("/testimonials/:id", middleware.RequirePermission(middleware.PermissionAdminWrite), testimonialHandler.UpdateTestimonial)
	admin.GET("/audit-logs", middleware.RequirePermission(middleware.PermissionAdminRead), auditLogsHandler)
	admin.GET("/audit", middleware.RequirePermission(middleware.PermissionAdminRead), auditLogsHandler)
	admin.GET("/activity", middleware.RequirePermission(middleware.PermissionAdminRead), auditLogsHandler)

	admin.GET("/users", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.ListUsers)
	admin.GET("/users/:id", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.GetUserByID)
	admin.POST("/users", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.CreateUser)
	admin.PATCH("/users/:id", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.UpdateUser)
	admin.DELETE("/users/:id", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.DeleteUser)

	admin.GET("/analytics", middleware.RequirePermission(middleware.PermissionAnalyticsRead), analyticsHandler.GetAdminAnalytics)
	admin.GET("/analytics/insights", middleware.RequirePermission(middleware.PermissionAnalyticsRead), analyticsHandler.GetDecisionInsights)
	admin.GET("/content/homepage-ad", middleware.RequirePermission(middleware.PermissionContentManage), siteContentHandler.GetAdminHomepageAd)
	admin.PUT("/content/homepage-ad", middleware.RequirePermission(middleware.PermissionContentManage), siteContentHandler.UpdateAdminHomepageAd)
	admin.GET("/content/confession-popup", middleware.RequirePermission(middleware.PermissionContentManage), siteContentHandler.GetAdminConfessionPopup)
	admin.PUT("/content/confession-popup", middleware.RequirePermission(middleware.PermissionContentManage), siteContentHandler.UpdateAdminConfessionPopup)
	admin.GET("/pastoral-care/requests", middleware.RequirePermission(middleware.PermissionEngagementRead), engagementHandler.ListPastoralCareRequests)
	admin.GET("/giving/intents", middleware.RequirePermission(middleware.PermissionEngagementRead), engagementHandler.ListGivingIntents)
	admin.GET("/giving", middleware.RequirePermission(middleware.PermissionEngagementRead), givingPaymentsHandler.List)
	admin.GET("/giving/summary", middleware.RequirePermission(middleware.PermissionEngagementRead), givingPaymentsHandler.MonthlySummary)

	// ATTENDANCE
	admin.GET("/attendance/service-types", attendanceHandler.ListServiceTypes)
	admin.POST("/attendance/service-types", middleware.RequirePermission(middleware.PermissionAdminWrite), attendanceHandler.CreateServiceType)
	admin.POST("/attendance/sessions", attendanceHandler.CreateSession)
	admin.GET("/attendance/sessions", attendanceHandler.ListSessions)
	admin.GET("/attendance/sessions/:id", attendanceHandler.GetSession)
	admin.PATCH("/attendance/sessions/:id", attendanceHandler.UpdateSession)
	admin.GET("/attendance/sessions/:id/records", attendanceHandler.ListRecords)
	admin.POST("/attendance/checkin", attendanceHandler.CheckIn)
	admin.GET("/attendance/members/:member_id/history", attendanceHandler.MemberHistory)

	// CELL GROUPS
	admin.POST("/cell-groups", cellGroupHandler.Create)
	admin.GET("/cell-groups", cellGroupHandler.List)
	admin.GET("/cell-groups/:id", cellGroupHandler.Get)
	admin.PATCH("/cell-groups/:id", cellGroupHandler.Update)
	admin.DELETE("/cell-groups/:id", middleware.RequirePermission(middleware.PermissionAdminWrite), cellGroupHandler.Delete)
	admin.POST("/cell-groups/:id/members", cellGroupHandler.AddMember)
	admin.GET("/cell-groups/:id/members", cellGroupHandler.ListMembers)
	admin.DELETE("/cell-groups/:id/members/:member_id", cellGroupHandler.RemoveMember)
	admin.POST("/cell-groups/:id/meetings", cellGroupHandler.CreateMeeting)
	admin.GET("/cell-groups/:id/meetings", cellGroupHandler.ListMeetings)
	admin.GET("/members/:member_id/cell-groups", cellGroupHandler.MemberGroups)

	// PRAYER REQUESTS — admin management (every read is sensitive pastoral data)
	admin.GET("/prayer-requests", prayerRequestHandler.List)
	admin.GET("/prayer-requests/:id", prayerRequestHandler.Get)
	admin.PATCH("/prayer-requests/:id/status", prayerRequestHandler.UpdateStatus)
	admin.PATCH("/prayer-requests/:id/assign", prayerRequestHandler.Assign)
	admin.POST("/prayer-requests/:id/notes", prayerRequestHandler.AddNotes)
	admin.DELETE("/prayer-requests/:id", middleware.RequirePermission(middleware.PermissionAdminWrite), prayerRequestHandler.Delete)

	// MINISTRIES
	admin.POST("/ministries", ministryHandler.Create)
	admin.GET("/ministries", ministryHandler.List)
	admin.GET("/ministries/:id", ministryHandler.Get)
	admin.PATCH("/ministries/:id", ministryHandler.Update)
	admin.DELETE("/ministries/:id", middleware.RequirePermission(middleware.PermissionAdminWrite), ministryHandler.Delete)
	admin.POST("/ministries/:id/members", ministryHandler.AddMember)
	admin.GET("/ministries/:id/members", ministryHandler.ListMembers)
	admin.DELETE("/ministries/:id/members/:member_id", ministryHandler.RemoveMember)
	admin.GET("/members/:member_id/ministries", ministryHandler.MemberMinistries)

	// SSE — real-time event stream (requires auth, no CSRF needed for GET)
	admin.GET("/events/stream", sseHandler.Stream)
	admin.GET("/contact/messages", middleware.RequirePermission(middleware.PermissionEngagementRead), engagementHandler.ListContactMessages)

	// Forms
	admin.GET("/forms", middleware.RequirePermission(middleware.PermissionFormsRead), formHandler.ListAdminForms)
	admin.GET("/forms/:id", middleware.RequirePermission(middleware.PermissionFormsRead), formHandler.GetAdminForm)
	admin.POST("/forms", middleware.RequirePermission(middleware.PermissionFormsManage), formHandler.CreateAdminForm)
	admin.PUT("/forms/:id", middleware.RequirePermission(middleware.PermissionFormsManage), formHandler.UpdateAdminForm)
	admin.DELETE("/forms/:id", middleware.RequirePermission(middleware.PermissionFormsManage), formHandler.DeleteAdminForm)
	admin.POST("/forms/:id/banner", middleware.RequirePermission(middleware.PermissionFormsManage), middleware.RequirePermission(middleware.PermissionUploadsManage), formHandler.UploadAdminFormBanner)
	admin.POST("/forms/:id/publish", middleware.RequirePermission(middleware.PermissionFormsManage), formHandler.PublishAdminForm)
	admin.GET("/forms/:id/report-link", middleware.RequirePermission(middleware.PermissionFormsExport), formHandler.GetAdminFormReportLink)
	admin.POST("/forms/:id/report-link", middleware.RequirePermission(middleware.PermissionFormsExport), formHandler.GetAdminFormReportLink)
	admin.GET("/forms/:id/campaigns/history", middleware.RequirePermission(middleware.PermissionFormsRead), formHandler.ListAdminFormCampaignHistory)
	admin.POST("/forms/:id/campaigns/send", middleware.RequirePermission(middleware.PermissionFormsCampaign), formHandler.SendAdminFormCampaign)
	admin.GET("/forms/:id/submissions", middleware.RequirePermission(middleware.PermissionFormsRead), formHandler.ListAdminSubmissions)
	admin.GET("/forms/:id/submissions/export.pdf", middleware.RequirePermission(middleware.PermissionFormsExport), formHandler.ExportAdminSubmissionsPDF)
	admin.GET("/forms/:id/submissions/stats", middleware.RequirePermission(middleware.PermissionFormsRead), formHandler.GetFormSubmissionStats)
	admin.GET("/forms/stats", middleware.RequirePermission(middleware.PermissionFormsRead), formHandler.GetFormStats)
	admin.DELETE("/form-submissions/:id", middleware.RequirePermission(middleware.PermissionFormsManage), formHandler.DeleteFormSubmission)

	// Notifications (admin)
	admin.GET("/notifications/subscribers", middleware.RequirePermission(middleware.PermissionNotificationsManage), notificationHandler.ListSubscribers)
	admin.GET("/notifications/subscribers/summary", middleware.RequirePermission(middleware.PermissionNotificationsManage), notificationHandler.GetSubscriberSummary)
	admin.POST("/notifications/send", middleware.RequirePermission(middleware.PermissionNotificationsManage), notificationHandler.SendNotification)
	admin.GET("/notifications/inbox", middleware.RequirePermission(middleware.PermissionNotificationsManage), adminNotificationHandler.List)
	admin.PATCH("/notifications/:id/read", middleware.RequirePermission(middleware.PermissionNotificationsManage), adminNotificationHandler.MarkRead)
	admin.POST("/notifications/read-all", middleware.RequirePermission(middleware.PermissionNotificationsManage), adminNotificationHandler.MarkAllRead)

	// Approval requests
	admin.GET("/requests", middleware.RequirePermission(middleware.PermissionApprovalsRead), approvalRequestHandler.List)
	admin.GET("/requests/timeline", middleware.RequirePermission(middleware.PermissionApprovalsRead), approvalRequestHandler.Timeline)
	admin.POST("/requests/:id/approve", middleware.RequirePermission(middleware.PermissionApprovalsRead), approvalRequestHandler.Approve)
	admin.POST("/requests/:id/reject", middleware.RequirePermission(middleware.PermissionApprovalsRead), approvalRequestHandler.Reject)

	// Email templates
	admin.POST("/email/templates/send", middleware.RequirePermission(middleware.PermissionEmailManage), emailTemplateHandler.SendTemplate)
	admin.GET("/email/templates", middleware.RequirePermission(middleware.PermissionEmailManage), emailTemplateRegistryHandler.List)
	admin.POST("/email/templates", middleware.RequirePermission(middleware.PermissionEmailManage), emailTemplateRegistryHandler.Create)
	admin.GET("/email/templates/:id", middleware.RequirePermission(middleware.PermissionEmailManage), emailTemplateRegistryHandler.Get)
	admin.PUT("/email/templates/:id", middleware.RequirePermission(middleware.PermissionEmailManage), emailTemplateRegistryHandler.Update)
	admin.POST("/email/templates/:id/activate", middleware.RequirePermission(middleware.PermissionEmailManage), emailTemplateRegistryHandler.Activate)
	admin.GET("/email/marketing/summary", middleware.RequirePermission(middleware.PermissionEmailManage), adminEmailHandler.GetMarketingSummary)
	admin.GET("/email/marketing/forms", middleware.RequirePermission(middleware.PermissionEmailManage), adminEmailHandler.ListAudienceForms)
	admin.GET("/email/marketing/audience/preview", middleware.RequirePermission(middleware.PermissionEmailManage), adminEmailHandler.PreviewAudience)
	admin.GET("/email/compose/history", middleware.RequirePermission(middleware.PermissionEmailManage), adminEmailHandler.ListComposeHistory)
	admin.POST("/email/compose/send", middleware.RequirePermission(middleware.PermissionEmailManage), adminEmailHandler.SendComposeEmail)
	admin.GET("/campaigns", middleware.RequirePermission(middleware.PermissionAdminRead), adminEmailHandler.ListComposeHistory)

	// Uploads
	admin.POST("/uploads/images", middleware.RequirePermission(middleware.PermissionUploadsManage), uploadHandler.UploadImage)
	admin.POST("/uploads/files", middleware.RequirePermission(middleware.PermissionUploadsManage), uploadHandler.UploadFile)
	admin.POST("/uploads", middleware.RequirePermission(middleware.PermissionUploadsManage), uploadHandler.UploadFile)
	admin.POST("/uploads/presign", middleware.RequirePermission(middleware.PermissionUploadsManage), assetHandler.Presign)
	admin.POST("/uploads/:id/complete", middleware.RequirePermission(middleware.PermissionUploadsManage), assetHandler.Complete)
	admin.GET("/uploads/:id", middleware.RequirePermission(middleware.PermissionUploadsManage), assetHandler.Get)
	admin.GET("/uploads", middleware.RequirePermission(middleware.PermissionUploadsManage), assetHandler.List)

	// Events
	admin.GET("/events", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.List)
	admin.GET("/events/:id", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.Get)
	admin.POST("/events", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.Create)
	admin.PUT("/events/:id", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.Update)
	admin.DELETE("/events/:id", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.Delete)
	admin.POST("/events/:id/image", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.UploadImage)
	admin.POST("/events/:id/banner", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.UploadBanner)

	// Reels
	admin.GET("/reels", middleware.RequirePermission(middleware.PermissionReelsManage), reelHandler.List)
	admin.POST("/reels", middleware.RequirePermission(middleware.PermissionReelsManage), reelHandler.Create)
	admin.DELETE("/reels/:id", middleware.RequirePermission(middleware.PermissionReelsManage), reelHandler.Delete)

	// Workforce admin
	admin.GET("/workforce", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.List)
	admin.POST("/workforce", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.Create)
	admin.PUT("/workforce/:id", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.Update)
	admin.DELETE("/workforce/:id", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.Delete)
	admin.GET("/workforce/stats", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.Stats)
	admin.GET("/workforce/birthdays/stats", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.BirthdayStats)
	admin.GET("/workforce/birthdays/month/:month", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.BirthdaysByMonth)
	admin.GET("/workforce/birthdays/today", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.BirthdaysToday)
	admin.POST("/workforce/birthdays/send-today", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.SendBirthdaysToday)

	// Members admin
	admin.GET("/members", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.List)
	admin.GET("/members/stats", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.Stats)
	admin.POST("/members", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.Create)
	admin.PUT("/members/:id", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.Update)
	admin.DELETE("/members/:id", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.Delete)
	admin.GET("/new-members/dashboard", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.NewMemberDashboard)
	admin.GET("/new-members/submissions", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.ListNewMemberSubmissions)
	admin.GET("/members/birthdays/stats", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.BirthdayStats)
	admin.GET("/members/birthdays/month/:month", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.BirthdaysByMonth)
	admin.GET("/members/birthdays/today", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.BirthdaysToday)
	admin.POST("/members/birthdays/send-today", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.SendBirthdaysToday)

	// Store admin
	admin.GET("/store/products", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.ListProductsAdmin)
	admin.POST("/store/products", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.CreateProduct)
	admin.PUT("/store/products/:id", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.UpdateProduct)
	admin.PATCH("/store/products/:id/stock", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.UpdateProductStock)
	admin.PATCH("/store/products/:id/active", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.UpdateProductActive)
	admin.GET("/store/orders", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.ListOrdersAdmin)
	admin.PATCH("/store/orders/:orderId/status", middleware.RequirePermission(middleware.PermissionStoreManage), storeHandler.UpdateOrderStatus)
	admin.POST("/members/notify", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.SendAnnouncement)
	admin.POST("/members/import", middleware.RequirePermission(middleware.PermissionMembersManage), memberHandler.ImportCSV)

	// Leadership admin
	admin.GET("/leadership", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.List)
	admin.POST("/leadership", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.Create)
	admin.PUT("/leadership/:id", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.Update)
	admin.DELETE("/leadership/:id", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.Delete)
	admin.GET("/leadership/birthdays/stats", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.BirthdayStats)
	admin.GET("/leadership/birthdays/month/:month", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.BirthdaysByMonth)
	admin.GET("/leadership/birthdays/today", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.BirthdaysToday)
	admin.POST("/leadership/birthdays/send-today", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.SendBirthdaysToday)
	admin.GET("/leadership/anniversaries/stats", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.AnniversaryStats)
	admin.GET("/leadership/anniversaries/month/:month", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.AnniversariesByMonth)
	admin.GET("/leadership/anniversaries/today", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.AnniversariesToday)
	admin.POST("/leadership/anniversaries/send-today", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.SendAnniversariesToday)

	// Super-admin
	superAdmin := admin.Group("")
	superAdmin.Use(middleware.RoleMiddleware("super_admin"))
	superAdmin.POST("/users/:id/approve", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.ApproveUser)
	superAdmin.POST("/users/:id/reject", middleware.RequirePermission(middleware.PermissionUsersManage), adminHandler.RejectUser)
	superAdmin.POST("/workforce/:id/approve", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.Approve)
	superAdmin.POST("/workforce/:id/delete/approve", middleware.RequirePermission(middleware.PermissionWorkforceManage), workforceHandler.ApproveDelete)
	superAdmin.POST("/leadership/:id/approve", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.Approve)
	superAdmin.POST("/leadership/:id/decline", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.Decline)
	superAdmin.POST("/leadership/:id/delete/approve", middleware.RequirePermission(middleware.PermissionLeadershipManage), leadershipHandler.ApproveDelete)
	superAdmin.PATCH("/testimonials/:id/approve", middleware.RequirePermission(middleware.PermissionAdminWrite), testimonialHandler.ApproveTestimonial)
	superAdmin.DELETE("/testimonials/:id", middleware.RequirePermission(middleware.PermissionAdminWrite), testimonialHandler.DeleteTestimonial)
	superAdmin.PATCH("/events/:id/approve", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.Approve)
	superAdmin.POST("/events/:id/delete/approve", middleware.RequirePermission(middleware.PermissionEventsManage), eventHandler.ApproveDeleteEvent)

	return router
}
