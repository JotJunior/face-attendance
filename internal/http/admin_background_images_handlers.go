package httphandler

// Handlers da API REST de imagens de fundo do editor de fluxos.
// Todos os endpoints requerem autenticação via SessionMiddleware.
// Ref: docs/specs/face-flow/plan.md §5, tasks.md §4.2.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/jotjunior/face-attendance/internal/logging"
	"github.com/jotjunior/face-attendance/internal/repository"
)

const (
	// maxUploadSize é o tamanho máximo para upload de imagens de fundo (tasks.md §4.2.1).
	maxUploadSize = 5 << 20 // 5 MB
)

// adminBgImageRepo define os métodos de BackgroundImageRepository usados pelos handlers.
type adminBgImageRepo interface {
	Create(ctx context.Context, name, filePath string) (*repository.BackgroundImage, error)
	FindByID(ctx context.Context, id int64) (*repository.BackgroundImage, error)
	FindAll(ctx context.Context) ([]*repository.BackgroundImage, error)
	Delete(ctx context.Context, id int64) error
}

// AdminBackgroundImagesConfig agrupa as dependências para os handlers de imagens.
type AdminBackgroundImagesConfig struct {
	Repo       adminBgImageRepo
	ImagesDir  string // diretório base onde as imagens são armazenadas
	Logger     *logging.Logger
}

// backgroundImageAPIResponse é o payload JSON de resposta de uma imagem de fundo.
type backgroundImageAPIResponse struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FilePath string `json:"file_path"`
}

// AdminBackgroundImagesRootHandler serve GET /admin/api/background-images (listar)
// e POST /admin/api/background-images (upload).
func AdminBackgroundImagesRootHandler(cfg AdminBackgroundImagesConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listBackgroundImagesHandler(cfg, w, r)
		case http.MethodPost:
			uploadBackgroundImageHandler(cfg, w, r)
		default:
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
		}
	})
}

// adminBgImagesSubRouter roteia /admin/api/background-images/{id} para os handlers corretos.
func adminBgImagesSubRouter(cfg AdminBackgroundImagesConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imageID, ok := bgImagePathID(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de imagem inválido")
			return
		}
		switch r.Method {
		case http.MethodDelete:
			deleteBackgroundImageHandler(cfg, w, r, imageID)
		default:
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
		}
	})
}

func listBackgroundImagesHandler(cfg AdminBackgroundImagesConfig, w http.ResponseWriter, r *http.Request) {
	images, err := cfg.Repo.FindAll(r.Context())
	if err != nil {
		cfg.Logger.Error("admin_bg_image_list", "", "", "listar imagens falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}
	if images == nil {
		images = []*repository.BackgroundImage{}
	}
	respList := make([]backgroundImageAPIResponse, 0, len(images))
	for _, img := range images {
		respList = append(respList, backgroundImageAPIResponse{
			ID:       img.ID,
			Name:     img.Name,
			FilePath: img.FilePath,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"images": respList}) //nolint:errcheck
}

func uploadBackgroundImageHandler(cfg AdminBackgroundImagesConfig, w http.ResponseWriter, r *http.Request) {
	// Limitar tamanho do upload (tasks.md §4.2.1).
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		adminJSONError(w, http.StatusRequestEntityTooLarge, "arquivo excede o limite de 5 MB")
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		adminJSONError(w, http.StatusBadRequest, "campo 'image' ausente no form multipart")
		return
	}
	defer file.Close()

	// Ler conteúdo para detectar tipo MIME por sniff (não confiar apenas na extensão).
	data, err := io.ReadAll(io.LimitReader(file, maxUploadSize))
	if err != nil {
		adminJSONError(w, http.StatusBadRequest, "falha ao ler arquivo enviado")
		return
	}

	ext, ok := detectImageMIME(data)
	if !ok {
		// Validar extensão MIME (JPEG/PNG apenas — tasks.md §4.2.2).
		adminJSONError(w, http.StatusUnsupportedMediaType, "tipo de arquivo não suportado; use JPEG ou PNG")
		return
	}

	// Gerar nome de arquivo único.
	filename, err := generateUniqueFilename(ext)
	if err != nil {
		cfg.Logger.Error("admin_bg_image_upload", "", "", "gerar filename falhou", err)
		adminJSONError(w, http.StatusInternalServerError, "erro interno ao gerar nome do arquivo")
		return
	}

	// Garantir que o diretório existe.
	if err := os.MkdirAll(cfg.ImagesDir, 0o755); err != nil {
		cfg.Logger.Error("admin_bg_image_upload", "", "", "criar diretório de imagens falhou", err)
		adminJSONError(w, http.StatusInternalServerError, "erro interno ao preparar diretório")
		return
	}

	destPath := filepath.Join(cfg.ImagesDir, filename)
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		cfg.Logger.Error("admin_bg_image_upload", "", "", "salvar arquivo falhou", err)
		adminJSONError(w, http.StatusInternalServerError, "erro interno ao salvar arquivo")
		return
	}

	// Nome amigável: usar nome original do arquivo sem extensão.
	friendlyName := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	if friendlyName == "" {
		friendlyName = strings.TrimSuffix(filename, ext)
	}

	created, err := cfg.Repo.Create(r.Context(), friendlyName, filename)
	if err != nil {
		// Compensar: remover arquivo já salvo em caso de falha no DB.
		_ = os.Remove(destPath)
		cfg.Logger.Error("admin_bg_image_upload", "", "", "registrar imagem no DB falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}

	resp := backgroundImageAPIResponse{
		ID:       created.ID,
		Name:     created.Name,
		FilePath: created.FilePath,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func deleteBackgroundImageHandler(cfg AdminBackgroundImagesConfig, w http.ResponseWriter, r *http.Request, imageID int64) {
	// Buscar registro para obter o file_path antes de excluir do DB.
	img, err := cfg.Repo.FindByID(r.Context(), imageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			adminJSONError(w, http.StatusNotFound, "imagem não encontrada")
			return
		}
		cfg.Logger.Error("admin_bg_image_delete", "", "", "buscar imagem falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}

	// Excluir registro do DB (tasks.md §2.2.3: handler HTTP remove o arquivo em disco).
	if err := cfg.Repo.Delete(r.Context(), imageID); err != nil {
		cfg.Logger.Error("admin_bg_image_delete", "", "", "excluir imagem do DB falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}

	// Remover arquivo em disco (best-effort; falha não reverte o delete do DB).
	destPath := filepath.Join(cfg.ImagesDir, img.FilePath)
	if removeErr := os.Remove(destPath); removeErr != nil && !os.IsNotExist(removeErr) {
		cfg.Logger.Error("admin_bg_image_delete", "", "", "remover arquivo em disco falhou", fmt.Errorf("%s: %w", img.FilePath, removeErr))
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers ---

// bgImagePathID extrai o ID de imagem de /admin/api/background-images/{id}.
func bgImagePathID(path string) (int64, bool) {
	const prefix = "/admin/api/background-images/"
	if !strings.HasPrefix(path, prefix) {
		return 0, false
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || strings.Contains(rest, "/") {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// detectImageMIME detecta se os dados são JPEG ou PNG por sniff do conteúdo.
// Retorna a extensão de arquivo normalizada (.jpg ou .png) e true se suportado.
func detectImageMIME(data []byte) (ext string, ok bool) {
	if len(data) < 4 {
		return "", false
	}
	// Detectar por magic bytes: PNG = \x89PNG, JPEG = \xFF\xD8.
	switch {
	case len(data) >= 8 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47:
		return ".png", true
	case data[0] == 0xFF && data[1] == 0xD8:
		return ".jpg", true
	default:
		// Fallback: http.DetectContentType (usa primeiros 512 bytes).
		sniffed := http.DetectContentType(data)
		switch {
		case strings.HasPrefix(sniffed, "image/png"):
			return ".png", true
		case strings.HasPrefix(sniffed, "image/jpeg"):
			return ".jpg", true
		}
		return "", false
	}
}

// generateUniqueFilename gera um nome de arquivo único com a extensão fornecida.
func generateUniqueFilename(ext string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("gerar nome único: %w", err)
	}
	return hex.EncodeToString(b) + ext, nil
}
