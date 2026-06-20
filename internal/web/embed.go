// Package web exposes the embedded static assets for the admin UI.
// Assets are compiled into the binary via //go:embed (no runtime file access needed).
// Ref: spec.md §FR-013, tasks.md §2.6.1, plan.md §Project Structure.
package web

import "embed"

// Assets holds the embedded static files from the dist/ directory.
// Served by http.FileServer at /admin/ (wired in internal/http/server.go).
// dist/ is populated by the frontend build step (FASE 3 — frontend-design skill).
//
// all: inclui arquivos ocultos (ex: .gitkeep) para evitar "no embeddable files"
// quando o diretório dist/ ainda está vazio (FASE 3 não executada).
//
//go:embed all:dist
var Assets embed.FS
