package api

import (
	"github.com/Niutaq/Gix/internal/api/handlers"
	"github.com/Niutaq/Gix/internal/infrastructure"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	gintrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/gin-gonic/gin"
)

func SetupRouter(app *infrastructure.AppState) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.Use(gintrace.Middleware("gix-server"))

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Next()
	})

	r.GET("/healthz", handlers.HandleHealthCheck(app))

	v1 := r.Group("/api/v1")
	{
		v1.GET("/cantors", handlers.HandleCantorsList(app))
		v1.DELETE("/cantors/:id", handlers.HandleDeleteCantor(app))
		v1.GET("/rates", handlers.HandleGetRates(app))
		v1.GET("/history", handlers.HandleGetHistory(app))
		v1.GET("/finops", handlers.HandleFinOps(app))
		v1.POST("/discover", handlers.HandleDiscover(app))
	}
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}
