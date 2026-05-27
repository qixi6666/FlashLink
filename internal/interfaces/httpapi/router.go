package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jd/flashlink/internal/app/health"
	"github.com/jd/flashlink/internal/app/linkapp"
	"github.com/jd/flashlink/internal/app/statapp"
	"github.com/jd/flashlink/internal/domain/link"
)

func init() {
	gin.SetMode(gin.ReleaseMode)
}

type RouterOptions struct {
	Links    LinkService
	Stats    StatsService
	Recorder VisitRecorder
}

type LinkService interface {
	CreateShortLink(c context.Context, req linkapp.CreateRequest) (linkapp.CreateResponse, error)
	Resolve(c context.Context, code string) (link.ShortLink, error)
}

type StatsService interface {
	GetLinkStats(c context.Context, code string) (link.LinkStats, error)
}

type VisitRecorder interface {
	Record(c context.Context, event statapp.VisitEvent) error
}

func NewRouter(options RouterOptions) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery(), accessLog())

	router.Any("/healthz", gin.WrapH(health.Handler("gateway")))
	registerPprof(router)
	router.POST("/api/links", createShortLink(options.Links))
	router.GET("/api/links/:code/stats", getLinkStats(options.Stats))
	router.GET("/:code", redirect(options.Links, options.Recorder))

	return router
}

func accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		slog.InfoContext(
			c.Request.Context(),
			"http_request",
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}

func registerPprof(router *gin.Engine) {
	group := router.Group("/debug/pprof")
	group.GET("/", gin.WrapF(pprof.Index))
	group.GET("/cmdline", gin.WrapF(pprof.Cmdline))
	group.GET("/profile", gin.WrapF(pprof.Profile))
	group.GET("/symbol", gin.WrapF(pprof.Symbol))
	group.POST("/symbol", gin.WrapF(pprof.Symbol))
	group.GET("/trace", gin.WrapF(pprof.Trace))
	group.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
	group.GET("/block", gin.WrapH(pprof.Handler("block")))
	group.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
	group.GET("/heap", gin.WrapH(pprof.Handler("heap")))
	group.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
	group.GET("/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
}

type createShortLinkRequest struct {
	LongURL  string     `json:"long_url"`
	Domain   string     `json:"domain"`
	ExpireAt *time.Time `json:"expire_at"`
}

func createShortLink(service LinkService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createShortLinkRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
			return
		}

		resp, err := service.CreateShortLink(c.Request.Context(), linkapp.CreateRequest{
			LongURL:  req.LongURL,
			Domain:   req.Domain,
			ExpireAt: req.ExpireAt,
		})
		if err != nil {
			writeError(c, err)
			return
		}

		c.JSON(http.StatusCreated, resp)
	}
}

func getLinkStats(service StatsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats, err := service.GetLinkStats(c.Request.Context(), strings.TrimSpace(c.Param("code")))
		if err != nil {
			writeError(c, err)
			return
		}

		c.JSON(http.StatusOK, stats)
	}
}

func redirect(service LinkService, recorder VisitRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := strings.TrimSpace(c.Param("code"))
		item, err := service.Resolve(c.Request.Context(), code)
		if err != nil {
			writeError(c, err)
			return
		}

		if recorder != nil {
			_ = recorder.Record(c.Request.Context(), statapp.VisitEvent{
				Code:      code,
				IP:        c.ClientIP(),
				UserAgent: c.Request.UserAgent(),
				Referer:   c.Request.Referer(),
			})
		}
		c.Redirect(http.StatusFound, item.LongURL)
	}
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, link.ErrInvalidCode), errors.Is(err, link.ErrInvalidURL):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, link.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, link.ErrExpired):
		c.JSON(http.StatusGone, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
