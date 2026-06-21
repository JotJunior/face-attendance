package httphandler

// Reenvio individual de um membro para reprocessamento (enrollment).
// POST /admin/api/members/{id}/resync — republica UMA ProcessingMessage na fila
// member.processing; o worker reprocessa apenas aquele membro. Opera por id
// (não por CPF) porque o frontend só expõe o CPF mascarado.
// Ref: dec-006/enrollment pipeline, FR-007 (sync), pedido do operador (reenvio seletivo).

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/logging"
)

// memberByIDFinder busca um membro pela chave primária.
type memberByIDFinder interface {
	FindByID(ctx context.Context, id int64) (*domain.Member, error)
}

// processingPublisher publica uma mensagem de processamento (enrollment) na fila.
type processingPublisher interface {
	Publish(ctx context.Context, msg domain.ProcessingMessage) error
}

// AdminResyncConfig agrupa as dependências do handler de reenvio individual.
type AdminResyncConfig struct {
	MemberFinder memberByIDFinder
	Publisher    processingPublisher
	Logger       *logging.Logger
}

// AdminMemberResyncHandler processa POST /admin/api/members/{id}/resync.
func AdminMemberResyncHandler(cfg AdminResyncConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		id, ok := resyncIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusNotFound, "rota inválida")
			return
		}

		member, err := cfg.MemberFinder.FindByID(r.Context(), id)
		if err != nil {
			cfg.Logger.Error("admin_resync", "", "", "falha ao buscar membro", err, "member_id", id)
			adminJSONError(w, http.StatusInternalServerError, "erro ao buscar membro")
			return
		}
		if member == nil {
			adminJSONError(w, http.StatusNotFound, "membro não encontrado")
			return
		}
		if !member.HasSelfie() {
			// Sem selfie não há o que enrolar (o scheduler também pula esses).
			adminJSONError(w, http.StatusUnprocessableEntity, "membro sem selfie — nada a reenviar")
			return
		}

		msg := domain.ProcessingMessage{
			FederalDocument: member.FederalDocument,
			Name:            member.Name,
			URLSelfie:       *member.URLSelfie,
			GobID:           member.GobID,
		}
		if err := cfg.Publisher.Publish(r.Context(), msg); err != nil {
			cfg.Logger.Error("admin_resync", "", member.FederalDocument, "falha ao publicar reprocessamento", err, "member_id", id)
			adminJSONError(w, http.StatusBadGateway, "falha ao enfileirar reprocessamento")
			return
		}

		cfg.Logger.Info("admin_resync", "", member.FederalDocument, "membro reenfileirado para reprocessamento", "member_id", id)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"queued"}`)) //nolint:errcheck
	})
}

// resyncIDFromPath extrai o {id} de /admin/api/members/{id}/resync.
// Retorna (0, false) se o padrão não casar ou o id não for inteiro.
func resyncIDFromPath(p string) (int64, bool) {
	p = strings.TrimSuffix(p, "/")
	parts := strings.Split(p, "/")
	// esperado: ["", "admin", "api", "members", "<id>", "resync"]
	if len(parts) < 2 || parts[len(parts)-1] != "resync" {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
