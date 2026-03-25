package main

import (
	"net/http"

	docs "booker/docs"

	"github.com/go-chi/chi/v5"
	"github.com/swaggo/swag"
)

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Room Booking Service API</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
  <style>
    html {
      box-sizing: border-box;
      overflow-y: scroll;
    }

    *,
    *::before,
    *::after {
      box-sizing: inherit;
    }

    body {
      margin: 0;
      background: #fafafa;
    }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-standalone-preset.js" crossorigin></script>
  <script>
    window.onload = function() {
      window.ui = SwaggerUIBundle({
        url: '/swagger/doc.json',
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
        layout: 'BaseLayout'
      });
    };
  </script>
</body>
</html>
`

func init() {
	docs.SwaggerInfo.Title = "Room Booking Service API"
	docs.SwaggerInfo.Version = "1.0"
	docs.SwaggerInfo.Description = "API for booking meeting rooms."
	docs.SwaggerInfo.BasePath = "/"
	docs.SwaggerInfo.Schemes = []string{"http"}
}

func registerSwaggerRoutes(r chi.Router) {
	r.Get("/swagger", redirectSwaggerIndex)
	r.Get("/swagger/", redirectSwaggerIndex)
	r.Get("/swagger/index.html", serveSwaggerUI)
	r.Get("/swagger/doc.json", serveSwaggerDoc)
}

func redirectSwaggerIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/swagger/index.html", http.StatusMovedPermanently)
}

func serveSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerUIHTML))
}

func serveSwaggerDoc(w http.ResponseWriter, _ *http.Request) {
	doc, err := swag.ReadDoc(docs.SwaggerInfo.InstanceName())
	if err != nil || doc == "" {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(doc))
}
