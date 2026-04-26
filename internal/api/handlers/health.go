package handlers

import (
	"net/http"

	"github.com/Niutaq/Gix/internal/infrastructure"
	"github.com/gin-gonic/gin"
)

// HandleHealthCheck godoc
// @Summary      Health Check
// @Description  Checks if the server, database, and Redis are running correctly.
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /healthz [get]
func HandleHealthCheck(app *infrastructure.AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := app.DB.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "service": "database"})
			return
		}
		if _, err := app.Cache.Ping(c.Request.Context()).Result(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "service": "cache"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "Gix is alive."})
	}
}
