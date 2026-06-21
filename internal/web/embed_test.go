package web_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jotjunior/face-attendance/internal/web"
)

// TestAssets_ServesIndexAndAssets exercita o embed FS REAL (web.Assets) através
// de um http.FileServer, como em produção. Regressão do bug onde o prefixo
// "dist/" não era removido: o FileServer servia uma listagem de diretório em
// "/" e devolvia 404 nos assets. Sem o fs.Sub em embed.go, este teste falha.
// Ref: task 3.2.6 (validação de embed.FS), bug detectado na validação no Pi.
func TestAssets_ServesIndexAndAssets(t *testing.T) {
	srv := httptest.NewServer(http.FileServer(http.FS(web.Assets)))
	defer srv.Close()

	cases := []struct {
		path         string
		wantContains string // vazio = só checa status 200
	}{
		{"/", "Presença Facial"},          // index.html servido na raiz (não listagem)
		{"/index.html", "Presença Facial"}, // 301 → "/" → index.html
		{"/assets/app.css", ""},            // o asset que dava 404 antes do fix
		{"/assets/app.js", ""},
	}
	for _, tc := range cases {
		resp, err := http.Get(srv.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", tc.path, resp.StatusCode)
		}
		if tc.wantContains != "" && !strings.Contains(string(body), tc.wantContains) {
			t.Errorf("GET %s: corpo não contém %q (servindo listagem em vez do index?)", tc.path, tc.wantContains)
		}
	}
}
