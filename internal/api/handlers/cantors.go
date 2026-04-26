package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Niutaq/Gix/internal/infrastructure"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const internalServerError = "Internal server error"

// HandleCantorsList godoc
// @Summary      List Cantors
// @Description  Returns a list of all available cantors with their geolocations.
// @Tags         cantors
// @Produce      json
// @Success      200  {array}   infrastructure.CantorListResponse
// @Failure      500  {object}  map[string]string
// @Router       /cantors [get]
func HandleCantorsList(app *infrastructure.AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := app.DB.Query(c.Request.Context(), "SELECT id, display_name, name, latitude, longitude, strategy, COALESCE(address, '') FROM cantors")
		if err != nil {
			log.Printf("DB Error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": internalServerError})
			return
		}
		defer rows.Close()

		var cantors []infrastructure.CantorListResponse
		for rows.Next() {
			var cr infrastructure.CantorListResponse
			if err := rows.Scan(&cr.ID, &cr.DisplayName, &cr.Name, &cr.Latitude, &cr.Longitude, &cr.Strategy, &cr.Address); err != nil {
				log.Printf("Scan Error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": internalServerError})
				return
			}
			cantors = append(cantors, cr)
		}
		c.JSON(http.StatusOK, cantors)
	}
}

// HandleDeleteCantor godoc
// @Summary      Delete Cantor
// @Description  Deletes a discovered cantor from the database by ID.
// @Tags         cantors
// @Param        id   path      int  true  "Cantor ID"
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /cantors/{id} [delete]
func HandleDeleteCantor(app *infrastructure.AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cantor ID"})
			return
		}

		var strategy string
		err = app.DB.QueryRow(c.Request.Context(), "SELECT strategy FROM cantors WHERE id = $1", id).Scan(&strategy)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "cantor not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}

		if !strings.HasPrefix(strategy, "H") && strategy != "HEURISTIC" {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete default system cantors"})
			return
		}

		_, _ = app.DB.Exec(c.Request.Context(), "DELETE FROM rates WHERE cantor_id = $1", id)
		res, err := app.DB.Exec(c.Request.Context(), "DELETE FROM cantors WHERE id = $1", id)
		if err != nil {
			log.Printf("DB Delete Cantor Error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete cantor"})
			return
		}

		if res.RowsAffected() == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "cantor not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "deleted"})
	}
}
