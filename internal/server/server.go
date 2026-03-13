package server

import (
	"database/sql"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// New creates a Gin engine that serves the API and the embedded Angular SPA.
func New(db *sql.DB, uiFS embed.FS) *gin.Engine {
	r := gin.Default()

	registerRoutes(r, db)

	// Serve the embedded Angular SPA.
	// The embed path is "ui/dist/browser", so we strip that prefix.
	stripped, err := fs.Sub(uiFS, "ui/dist/browser")
	if err != nil {
		panic("failed to create sub filesystem for UI: " + err.Error())
	}

	// Read index.html once for SPA fallback.
	indexHTML, err := fs.ReadFile(stripped, "index.html")
	if err != nil {
		panic("failed to read index.html from embedded UI: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(stripped))

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path[1:] // strip leading "/"

		// Try to serve a static file if it exists.
		if path != "" {
			if f, err := stripped.Open(path); err == nil {
				f.Close()
				setCacheHeaders(c, c.Request.URL.Path)
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}

		// Fall back to index.html for Angular client-side routing.
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})

	return r
}

// longCacheExts lists file extensions for assets that are fingerprinted by the
// Angular build and can be cached indefinitely.
var longCacheExts = map[string]bool{
	".js": true, ".css": true,
	".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".svg": true, ".ico": true, ".webp": true, ".avif": true,
	".map":  true,
	".json": false, // JSON files may change more frequently, so we don't cache them long-term. (especially lang files)
}

// setCacheHeaders sets Cache-Control headers based on the served file's extension.
func setCacheHeaders(c *gin.Context, urlPath string) {
	ext := strings.ToLower(path.Ext(urlPath))
	if longCacheExts[ext] {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Header("Cache-Control", "no-cache")
	}
}
