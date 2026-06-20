// Package web exposes the embedded static assets for the admin UI.
// Assets are compiled into the binary via //go:embed (no runtime file access needed).
// Ref: spec.md §FR-013, tasks.md §2.6.1, plan.md §Project Structure.
package web

import (
	"embed"
	"io/fs"
)

// distFS holds the embedded static files INCLUDING the dist/ prefix.
//
// all: inclui arquivos ocultos (ex: .gitkeep) para evitar "no embeddable files"
// quando o diretório dist/ ainda está vazio (FASE 3 não executada).
//
//go:embed all:dist
var distFS embed.FS

// Assets is the dist/ subtree (prefixo removido) pronto para http.FileServer
// montado em /admin/ (wired em internal/http/server.go).
//
// O fs.Sub é obrigatório: sem ele, o FileServer enxerga a raiz do embed (que
// contém o diretório "dist/"), servindo uma LISTAGEM em /admin/ e devolvendo
// 404 em /admin/assets/* — bug detectado na validação do servidor real (a
// task 3.2.6). Com o Sub, "/" → dist/index.html e "/assets/app.css" resolve.
var Assets = mustSub(distFS, "dist")

func mustSub(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		// dist sempre existe em tempo de compilação (//go:embed garante).
		panic("web: fs.Sub(dist): " + err.Error())
	}
	return sub
}
