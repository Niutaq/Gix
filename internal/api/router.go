package api

import (
	"time"

	"github.com/Niutaq/Gix/internal/api/handlers"
	"github.com/Niutaq/Gix/internal/api/middleware"
	"github.com/Niutaq/Gix/internal/infrastructure"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	gintrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/gin-gonic/gin"
)

// SetupRouter configures and returns a gin router with all API endpoints.
func SetupRouter(app *infrastructure.AppState) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.Use(gintrace.Middleware("gix-server"))
	r.Use(middleware.RestrictCORS())

	r.GET("/healthz", handlers.HandleHealthCheck(app))

	v1 := r.Group("/api/v1")
	{
		v1.GET("/cantors", handlers.HandleCantorsList(app))
		v1.GET("/rates", handlers.HandleGetRates(app))
		v1.GET("/history", handlers.HandleGetHistory(app))
		v1.GET("/finops", handlers.HandleFinOps(app))
		v1.GET("/finops/flow", handlers.HandleFinOpsFlow(app))

		// Mutating / cost-heavy: API key (when GIX_API_KEY set) + rate limit (NIS2 access + abuse).
		mutate := v1.Group("/")
		mutate.Use(middleware.RequireAPIKey(), middleware.RateLimit(20, time.Minute))
		{
			mutate.DELETE("/cantors", handlers.HandleDeleteAllCantors(app))
			mutate.DELETE("/cantors/:id", handlers.HandleDeleteCantor(app))
			mutate.POST("/discover", handlers.HandleDiscover(app))
		}
	}
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}
