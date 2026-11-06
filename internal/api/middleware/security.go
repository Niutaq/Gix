// Package middleware — NetSecOps controls for the HTTP API (NIS2-aligned access + abuse limits).
package middleware

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const headerAPIKey = "X-Gix-API-Key"

// RequireAPIKey gates mutating / costly routes when GIX_API_KEY is set.
// Empty key = local-dev open mode (loud warning once); production must set the env.
func RequireAPIKey() gin.HandlerFunc {
	expected := strings.TrimSpace(os.Getenv("GIX_API_KEY"))
	if expected == "" {
		var once sync.Once
		return func(c *gin.Context) {
			once.Do(func() {
				log.Printf("[NetSec] GIX_API_KEY unset — mutating routes are OPEN (local only; set key for NIS2 access control)")
			})
			c.Next()
		}
	}
	want := []byte(expected)
	return func(c *gin.Context) {
		got := strings.TrimSpace(c.GetHeader(headerAPIKey))
		if got == "" {
			got = strings.TrimSpace(c.Query("api_key")) // avoid; accepted only for emergency tooling
		}
		if subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

// RateLimit limits requests per client IP (sliding fixed window).
func RateLimit(maxPerWindow int, window time.Duration) gin.HandlerFunc {
	if maxPerWindow <= 0 {
		maxPerWindow = 10
	}
	if window <= 0 {
		window = time.Minute
	}
	type bucket struct {
		n    int
		from time.Time
	}
	var mu sync.Mutex
	buckets := make(map[string]*bucket)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()
		mu.Lock()
		b, ok := buckets[ip]
		if !ok || now.Sub(b.from) >= window {
			b = &bucket{n: 0, from: now}
			buckets[ip] = b
		}
		b.n++
		n := b.n
		mu.Unlock()
		if n > maxPerWindow {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

// RestrictCORS replaces wildcard CORS when GIX_CORS_ORIGINS is set (comma-separated).
func RestrictCORS() gin.HandlerFunc {
	raw := strings.TrimSpace(os.Getenv("GIX_CORS_ORIGINS"))
	allowed := map[string]bool{}
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			allowed[o] = true
		}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if len(allowed) == 0 {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		} else if allowed[origin] {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+headerAPIKey)
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
