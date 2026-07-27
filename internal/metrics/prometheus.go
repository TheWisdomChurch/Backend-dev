package metrics

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "wisdomhouse",
		Name:      "http_request_duration_seconds",
		Help:      "Duration of HTTP requests in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "path", "status"})

	loginAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "wisdomhouse",
		Name:      "login_attempts_total",
		Help:      "Total login attempts, labelled by result.",
	}, []string{"result"}) // result: success | failed | locked | mfa_required

	analyticsBatches = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "wisdomhouse", Name: "analytics_batches_total",
		Help: "Analytics ingestion batches by result.",
	}, []string{"result"})
	analyticsEvents = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "wisdomhouse", Name: "analytics_events_accepted_total",
		Help: "Validated analytics events accepted for persistence.",
	})
	analyticsQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "wisdomhouse", Name: "analytics_query_duration_seconds",
		Help: "Duration of analytics endpoint computation.", Buckets: prometheus.DefBuckets,
	}, []string{"section", "result"})
	analyticsCache = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "wisdomhouse", Name: "analytics_cache_operations_total",
		Help: "Analytics cache operations by result.",
	}, []string{"result"})

	dbPoolMaxOpen = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "wisdomhouse",
		Name:      "db_pool_max_open",
		Help:      "Maximum number of open DB connections configured.",
	})
	dbPoolOpen = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "wisdomhouse",
		Name:      "db_pool_open",
		Help:      "Current number of open DB connections.",
	})
	dbPoolInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "wisdomhouse",
		Name:      "db_pool_in_use",
		Help:      "Current number of DB connections in use.",
	})
	dbPoolIdle = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "wisdomhouse",
		Name:      "db_pool_idle",
		Help:      "Current number of idle DB connections.",
	})
	dbPoolWaitCount = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "wisdomhouse",
		Name:      "db_pool_wait_count",
		Help:      "Cumulative number of times a goroutine waited for a DB connection.",
	})
)

// RecordLogin increments the login counter with the given result label.
// Canonical result values: "success", "failed", "locked", "mfa_required".
func RecordLogin(result string) {
	loginAttempts.WithLabelValues(result).Inc()
}

func RecordAnalyticsIngest(result string, eventCount int) {
	analyticsBatches.WithLabelValues(result).Inc()
	if result == "accepted" && eventCount > 0 {
		analyticsEvents.Add(float64(eventCount))
	}
}

func RecordAnalyticsQuery(section, result string, duration time.Duration) {
	analyticsQueryDuration.WithLabelValues(section, result).Observe(duration.Seconds())
}

func RecordAnalyticsCache(result string) {
	analyticsCache.WithLabelValues(result).Inc()
}

// GinMiddleware returns a Gin middleware that records HTTP request duration.
func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start).Seconds()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		httpRequestDuration.WithLabelValues(
			c.Request.Method,
			path,
			http.StatusText(c.Writer.Status()),
		).Observe(duration)
	}
}

// PollDBStats records sql.DBStats gauges. Call in a background goroutine every 15s.
func PollDBStats(db *sql.DB) {
	if db == nil {
		return
	}
	s := db.Stats()
	dbPoolMaxOpen.Set(float64(s.MaxOpenConnections))
	dbPoolOpen.Set(float64(s.OpenConnections))
	dbPoolInUse.Set(float64(s.InUse))
	dbPoolIdle.Set(float64(s.Idle))
	dbPoolWaitCount.Set(float64(s.WaitCount))
}

// Handler returns an http.Handler for the /metrics endpoint.
func Handler() http.Handler {
	return promhttp.Handler()
}
