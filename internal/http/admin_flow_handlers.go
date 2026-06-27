package httphandler

// Handlers da API REST de fluxos de execução — painel de administração.
// Todos os endpoints requerem autenticação via SessionMiddleware.
// Ref: docs/specs/face-flow/plan.md §5, tasks.md §4.1.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jotjunior/face-attendance/internal/flow"
	"github.com/jotjunior/face-attendance/internal/logging"
	"github.com/jotjunior/face-attendance/internal/repository"
	"github.com/jotjunior/face-attendance/internal/secrets"
)

const (
	// secretInputPrefix é o prefixo que indica um header secreto no payload de entrada.
	// O valor APÓS o prefixo é o plaintext a ser cifrado (tasks.md §3.8.1).
	secretInputPrefix = "__secret__:"

	// flowSealedSentinel é o sentinela armazenado no config do nó em vez do valor real.
	// Deve coincidir com sealedSentinel em flowengine/node_https.go.
	flowSealedSentinel = "__sealed__"

	// secretMasked é o valor exibido na resposta da API para headers selados (tasks.md §3.8.2).
	secretMasked = "__secret__:***"
)

// --- Interfaces dos repositórios ---

// adminFlowRepo define os métodos de FlowRepository usados pelos handlers de fluxo.
type adminFlowRepo interface {
	Create(ctx context.Context, f *flow.Flow) (*flow.Flow, error)
	FindByID(ctx context.Context, id int64) (*flow.Flow, error)
	FindAll(ctx context.Context) ([]*flow.Flow, error)
	Update(ctx context.Context, f *flow.Flow) (*flow.Flow, error)
	SetStatus(ctx context.Context, id int64, status string) error
	SetDeviceID(ctx context.Context, id int64, deviceID *int64) error
	Delete(ctx context.Context, id int64) error
}

// adminFlowLogRepo define os métodos de FlowExecutionLogRepository usados pelos handlers.
type adminFlowLogRepo interface {
	FindByFlowID(ctx context.Context, flowID int64, limit, offset int) ([]*repository.FlowExecutionLog, error)
}

// AdminFlowsConfig agrupa as dependências para os handlers de fluxo admin.
type AdminFlowsConfig struct {
	FlowRepo   adminFlowRepo
	DeviceRepo deviceAdminRepo  // para validar existência do device (tasks.md §4.1.3)
	LogRepo    adminFlowLogRepo
	Cipher     *secrets.Cipher  // nil → headers secretos não podem ser cifrados
	Logger     *logging.Logger
}

// --- Payload types ---

// flowCreateRequest é o payload para POST /admin/api/flows.
type flowCreateRequest struct {
	Name string `json:"name"`
}

// flowUpdateRequest é o payload para PUT /admin/api/flows/{id}.
type flowUpdateRequest struct {
	Name  string          `json:"name"`
	Nodes []flow.FlowNode `json:"nodes"`
	Edges []flow.FlowEdge `json:"edges"`
}

// flowSetDeviceRequest é o payload para PUT /admin/api/flows/{id}/device.
type flowSetDeviceRequest struct {
	DeviceID int64 `json:"device_id"`
}

// validationErrorResponse é o payload 422 retornado quando flow.Validate() detecta erros.
type validationErrorResponse struct {
	Errors []flowValidationError `json:"errors"`
}

type flowValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	NodeID  string `json:"node_id,omitempty"`
}

// flowAPIResponse é o payload JSON de resposta de um fluxo.
// Não inclui sealed_config — headers selados são mascarados como "__secret__:***" (tasks.md §3.8.2).
type flowAPIResponse struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Status    string          `json:"status"`
	DeviceID  *int64          `json:"device_id,omitempty"`
	Nodes     json.RawMessage `json:"nodes"`
	Edges     []flow.FlowEdge `json:"edges"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// flowLogResponse é o payload de resposta de um log de execução.
type flowLogResponse struct {
	ID           int64      `json:"id"`
	FlowID       int64      `json:"flow_id"`
	DeviceID     int64      `json:"device_id"`
	EventKey     string     `json:"event_key"`
	Status       string     `json:"status"`
	FailedNodeID *string    `json:"failed_node_id,omitempty"`
	Error        *string    `json:"error,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   time.Time  `json:"finished_at"`
}

// --- Roteadores ---

// AdminFlowsRootHandler serve GET /admin/api/flows (listar) e POST /admin/api/flows (criar).
func AdminFlowsRootHandler(cfg AdminFlowsConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listFlowsHandler(cfg, w, r)
		case http.MethodPost:
			createFlowHandler(cfg, w, r)
		default:
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
		}
	})
}

// adminFlowsSubRouter roteia /admin/api/flows/{id}[/sub] para os handlers corretos.
// Routing table (todos requerem SessionMiddleware — aplicado pelo chamador):
//
//	/admin/api/flows/{id}              → GET (detalhe) | PUT (atualizar) | DELETE (remover)
//	/admin/api/flows/{id}/activate     → PUT (ativar)
//	/admin/api/flows/{id}/deactivate   → PUT (desativar)
//	/admin/api/flows/{id}/device       → PUT (vincular) | DELETE (desvincular)
//	/admin/api/flows/{id}/logs         → GET (logs paginados)
func adminFlowsSubRouter(cfg AdminFlowsConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flowID, segs, ok := flowPathSegments(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de fluxo inválido")
			return
		}

		if len(segs) == 0 || (len(segs) == 1 && segs[0] == "") {
			// /admin/api/flows/{id}
			switch r.Method {
			case http.MethodGet:
				getFlowHandler(cfg, w, r, flowID)
			case http.MethodPut:
				updateFlowHandler(cfg, w, r, flowID)
			case http.MethodDelete:
				deleteFlowHandler(cfg, w, r, flowID)
			default:
				adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			}
			return
		}

		switch segs[0] {
		case "activate":
			if r.Method != http.MethodPut {
				adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
				return
			}
			activateFlowHandler(cfg, w, r, flowID)
		case "deactivate":
			if r.Method != http.MethodPut {
				adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
				return
			}
			deactivateFlowHandler(cfg, w, r, flowID)
		case "device":
			switch r.Method {
			case http.MethodPut:
				setFlowDeviceHandler(cfg, w, r, flowID)
			case http.MethodDelete:
				unsetFlowDeviceHandler(cfg, w, r, flowID)
			default:
				adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			}
		case "logs":
			if r.Method != http.MethodGet {
				adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
				return
			}
			listFlowLogsHandler(cfg, w, r, flowID)
		default:
			adminJSONError(w, http.StatusNotFound, "endpoint de fluxo não encontrado")
		}
	})
}

// --- Handlers individuais ---

func listFlowsHandler(cfg AdminFlowsConfig, w http.ResponseWriter, r *http.Request) {
	flows, err := cfg.FlowRepo.FindAll(r.Context())
	if err != nil {
		cfg.Logger.Error("admin_flow_list", "", "", "lista de fluxos falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}
	if flows == nil {
		flows = []*flow.Flow{}
	}
	respList := make([]flowAPIResponse, 0, len(flows))
	for _, f := range flows {
		resp, err := buildFlowAPIResponse(f)
		if err != nil {
			cfg.Logger.Error("admin_flow_list", "", "", "erro ao serializar fluxo", err)
			adminJSONError(w, http.StatusInternalServerError, "erro interno")
			return
		}
		respList = append(respList, resp)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"flows": respList}) //nolint:errcheck
}

func createFlowHandler(cfg AdminFlowsConfig, w http.ResponseWriter, r *http.Request) {
	var req flowCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		adminJSONError(w, http.StatusBadRequest, "body JSON inválido")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		adminJSONError(w, http.StatusBadRequest, "nome é obrigatório")
		return
	}

	f := &flow.Flow{
		Name:   req.Name,
		Status: "inactive",
		Nodes:  []flow.FlowNode{},
		Edges:  []flow.FlowEdge{},
	}
	created, err := cfg.FlowRepo.Create(r.Context(), f)
	if err != nil {
		cfg.Logger.Error("admin_flow_create", "", "", "criar fluxo falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}
	resp, _ := buildFlowAPIResponse(created)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func getFlowHandler(cfg AdminFlowsConfig, w http.ResponseWriter, r *http.Request, flowID int64) {
	f, err := cfg.FlowRepo.FindByID(r.Context(), flowID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			adminJSONError(w, http.StatusNotFound, "fluxo não encontrado")
			return
		}
		cfg.Logger.Error("admin_flow_get", "", "", "buscar fluxo falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}
	resp, err := buildFlowAPIResponse(f)
	if err != nil {
		cfg.Logger.Error("admin_flow_get", "", "", "serializar fluxo falhou", err)
		adminJSONError(w, http.StatusInternalServerError, "erro interno")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func updateFlowHandler(cfg AdminFlowsConfig, w http.ResponseWriter, r *http.Request, flowID int64) {
	var req flowUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		adminJSONError(w, http.StatusBadRequest, "body JSON inválido")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		adminJSONError(w, http.StatusBadRequest, "nome é obrigatório")
		return
	}
	if req.Nodes == nil {
		req.Nodes = []flow.FlowNode{}
	}
	if req.Edges == nil {
		req.Edges = []flow.FlowEdge{}
	}

	// Buscar fluxo existente para obter SealedConfig atual (para reter segredos não re-submetidos).
	existing, err := cfg.FlowRepo.FindByID(r.Context(), flowID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			adminJSONError(w, http.StatusNotFound, "fluxo não encontrado")
			return
		}
		cfg.Logger.Error("admin_flow_update", "", "", "buscar fluxo falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}

	// Processar segredos selados antes de validar (tasks.md §3.8.1).
	processedNodes, newSealedConfig, sealErr := processFlowSealedConfig(
		req.Nodes, existing.SealedConfig, cfg.Cipher,
	)
	if sealErr != nil {
		adminJSONError(w, http.StatusBadRequest, sealErr.Error())
		return
	}

	// Validar fluxo (tasks.md §4.1.2).
	draft := &flow.Flow{Nodes: processedNodes, Edges: req.Edges}
	if validErrs := flow.Validate(draft); len(validErrs) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(validationErrorResponse{ //nolint:errcheck
			Errors: convertValidationErrors(validErrs),
		})
		return
	}

	updated, err := cfg.FlowRepo.Update(r.Context(), &flow.Flow{
		ID:           flowID,
		Name:         req.Name,
		Nodes:        processedNodes,
		Edges:        req.Edges,
		SealedConfig: newSealedConfig,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			adminJSONError(w, http.StatusNotFound, "fluxo não encontrado")
			return
		}
		cfg.Logger.Error("admin_flow_update", "", "", "atualizar fluxo falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}
	resp, _ := buildFlowAPIResponse(updated)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func deleteFlowHandler(cfg AdminFlowsConfig, w http.ResponseWriter, r *http.Request, flowID int64) {
	if err := cfg.FlowRepo.Delete(r.Context(), flowID); err != nil {
		cfg.Logger.Error("admin_flow_delete", "", "", "excluir fluxo falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func activateFlowHandler(cfg AdminFlowsConfig, w http.ResponseWriter, r *http.Request, flowID int64) {
	// Buscar fluxo atual para validar antes de ativar.
	f, err := cfg.FlowRepo.FindByID(r.Context(), flowID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			adminJSONError(w, http.StatusNotFound, "fluxo não encontrado")
			return
		}
		cfg.Logger.Error("admin_flow_activate", "", "", "buscar fluxo falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}

	// Validar antes de ativar (tasks.md §4.1.2).
	if validErrs := flow.Validate(f); len(validErrs) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(validationErrorResponse{ //nolint:errcheck
			Errors: convertValidationErrors(validErrs),
		})
		return
	}

	if err := cfg.FlowRepo.SetStatus(r.Context(), flowID, "active"); err != nil {
		cfg.Logger.Error("admin_flow_activate", "", "", "ativar fluxo falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "active"}) //nolint:errcheck
}

func deactivateFlowHandler(cfg AdminFlowsConfig, w http.ResponseWriter, r *http.Request, flowID int64) {
	if err := cfg.FlowRepo.SetStatus(r.Context(), flowID, "inactive"); err != nil {
		cfg.Logger.Error("admin_flow_deactivate", "", "", "desativar fluxo falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "inactive"}) //nolint:errcheck
}

func setFlowDeviceHandler(cfg AdminFlowsConfig, w http.ResponseWriter, r *http.Request, flowID int64) {
	var req flowSetDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		adminJSONError(w, http.StatusBadRequest, "body JSON inválido")
		return
	}
	if req.DeviceID <= 0 {
		adminJSONError(w, http.StatusBadRequest, "device_id inválido")
		return
	}

	// Verificar que o device existe (tasks.md §4.1.3).
	if _, err := cfg.DeviceRepo.GetDeviceByID(r.Context(), req.DeviceID); err != nil {
		adminJSONError(w, http.StatusNotFound, "dispositivo não encontrado")
		return
	}

	// Vincular device ao fluxo (unique constraint → ErrFlowDeviceConflict se ocupado).
	if err := cfg.FlowRepo.SetDeviceID(r.Context(), flowID, &req.DeviceID); err != nil {
		if errors.Is(err, repository.ErrFlowDeviceConflict) {
			adminJSONError(w, http.StatusConflict, "dispositivo já vinculado a outro fluxo ativo")
			return
		}
		cfg.Logger.Error("admin_flow_set_device", "", "", "vincular device falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"flow_id": flowID, "device_id": req.DeviceID}) //nolint:errcheck
}

func unsetFlowDeviceHandler(cfg AdminFlowsConfig, w http.ResponseWriter, r *http.Request, flowID int64) {
	if err := cfg.FlowRepo.SetDeviceID(r.Context(), flowID, nil); err != nil {
		cfg.Logger.Error("admin_flow_unset_device", "", "", "desvincular device falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func listFlowLogsHandler(cfg AdminFlowsConfig, w http.ResponseWriter, r *http.Request, flowID int64) {
	const defaultLimit = 20
	const maxLimit = 100

	q := r.URL.Query()
	limit := defaultLimit
	offset := 0

	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > maxLimit {
				n = maxLimit
			}
			limit = n
		}
	}
	if o := q.Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	logs, err := cfg.LogRepo.FindByFlowID(r.Context(), flowID, limit, offset)
	if err != nil {
		cfg.Logger.Error("admin_flow_logs", "", "", "buscar logs falhou", err)
		adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
		return
	}
	if logs == nil {
		logs = []*repository.FlowExecutionLog{}
	}
	respList := make([]flowLogResponse, 0, len(logs))
	for _, l := range logs {
		respList = append(respList, flowLogResponse{
			ID:           l.ID,
			FlowID:       l.FlowID,
			DeviceID:     l.DeviceID,
			EventKey:     l.EventKey,
			Status:       l.Status,
			FailedNodeID: l.FailedNodeID,
			Error:        l.Error,
			StartedAt:    l.StartedAt,
			FinishedAt:   l.FinishedAt,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
		"logs":   respList,
		"limit":  limit,
		"offset": offset,
	})
}

// --- Helpers ---

// flowPathSegments extrai o flowID e os segmentos restantes de /admin/api/flows/{id}[/sub].
func flowPathSegments(path string) (flowID int64, segs []string, ok bool) {
	const prefix = "/admin/api/flows/"
	if !strings.HasPrefix(path, prefix) {
		return 0, nil, false
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" {
		return 0, nil, false
	}
	parts := strings.SplitN(rest, "/", 2)
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, nil, false
	}
	if len(parts) == 1 {
		return id, nil, true
	}
	subPath := parts[1]
	if subPath == "" {
		return id, nil, true
	}
	return id, strings.Split(subPath, "/"), true
}

// buildFlowAPIResponse converte *flow.Flow em flowAPIResponse, mascarando headers selados.
// Não inclui sealed_config na resposta (segurança — tasks.md §3.8.2).
func buildFlowAPIResponse(f *flow.Flow) (flowAPIResponse, error) {
	maskedNodesJSON, err := maskSealedNodesJSON(f.Nodes)
	if err != nil {
		return flowAPIResponse{}, fmt.Errorf("buildFlowAPIResponse: %w", err)
	}
	edges := f.Edges
	if edges == nil {
		edges = []flow.FlowEdge{}
	}
	return flowAPIResponse{
		ID:        f.ID,
		Name:      f.Name,
		Status:    f.Status,
		DeviceID:  f.DeviceID,
		Nodes:     maskedNodesJSON,
		Edges:     edges,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}, nil
}

// maskSealedNodesJSON serializa os nós do fluxo como JSON, substituindo sentinela
// "__sealed__" por "__secret__:***" nos headers dos nós https_call (tasks.md §3.8.2).
func maskSealedNodesJSON(nodes []flow.FlowNode) (json.RawMessage, error) {
	if nodes == nil {
		nodes = []flow.FlowNode{}
	}
	out := make([]json.RawMessage, 0, len(nodes))
	for _, n := range nodes {
		if n.Type != flow.NodeTypeHTTPSCall || len(n.Config) == 0 {
			raw, err := json.Marshal(n)
			if err != nil {
				return nil, err
			}
			out = append(out, raw)
			continue
		}
		// Para nós https_call: mascarar headers com valor "__sealed__".
		var cfgMap map[string]json.RawMessage
		if err := json.Unmarshal(n.Config, &cfgMap); err != nil {
			raw, _ := json.Marshal(n)
			out = append(out, raw)
			continue
		}
		if headersRaw, ok := cfgMap["headers"]; ok {
			var headers map[string]string
			if err := json.Unmarshal(headersRaw, &headers); err == nil {
				changed := false
				for k, v := range headers {
					if v == flowSealedSentinel {
						headers[k] = secretMasked
						changed = true
					}
				}
				if changed {
					maskedJSON, err := json.Marshal(headers)
					if err == nil {
						cfgMap["headers"] = maskedJSON
					}
				}
			}
		}
		maskedConfig, err := json.Marshal(cfgMap)
		if err != nil {
			raw, _ := json.Marshal(n)
			out = append(out, raw)
			continue
		}
		maskedNode := flow.FlowNode{
			ID:     n.ID,
			Type:   n.Type,
			Config: maskedConfig,
			X:      n.X,
			Y:      n.Y,
		}
		raw, err := json.Marshal(maskedNode)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return json.Marshal(out)
}

// processFlowSealedConfig processa os nós do fluxo cifrando headers com prefixo "__secret__:".
// Retorna os nós modificados (sentinela "__sealed__" no lugar do valor) e o novo SealedConfig.
// Mantém entradas existentes do sealedConfig para headers não re-submetidos (reter segredos).
// Ref: tasks.md §3.8.1.
func processFlowSealedConfig(
	nodes []flow.FlowNode,
	existingSealedConfig map[string]string,
	cipher *secrets.Cipher,
) (processedNodes []flow.FlowNode, newSealedConfig map[string]string, err error) {
	// Começar com o sealed_config existente; apenas sobrescrever entradas re-cifradas.
	newSealedConfig = make(map[string]string, len(existingSealedConfig))
	for k, v := range existingSealedConfig {
		newSealedConfig[k] = v
	}

	processedNodes = make([]flow.FlowNode, len(nodes))
	for i, n := range nodes {
		if n.Type != flow.NodeTypeHTTPSCall || len(n.Config) == 0 {
			processedNodes[i] = n
			continue
		}

		var cfgMap map[string]json.RawMessage
		if err := json.Unmarshal(n.Config, &cfgMap); err != nil {
			processedNodes[i] = n
			continue
		}

		headersRaw, ok := cfgMap["headers"]
		if !ok {
			processedNodes[i] = n
			continue
		}

		var headers map[string]string
		if err := json.Unmarshal(headersRaw, &headers); err != nil {
			processedNodes[i] = n
			continue
		}

		changed := false
		for headerName, headerValue := range headers {
			if !strings.HasPrefix(headerValue, secretInputPrefix) {
				continue
			}
			if cipher == nil {
				return nil, nil, fmt.Errorf(
					"cifrador não configurado (ISAPI_CRED_KEY ausente); não é possível cifrar header secreto %q",
					headerName,
				)
			}
			plaintext := strings.TrimPrefix(headerValue, secretInputPrefix)
			encrypted, encErr := cipher.Encrypt(plaintext)
			if encErr != nil {
				return nil, nil, fmt.Errorf("falha ao cifrar header %q: %w", headerName, encErr)
			}
			sealedKey := n.ID + "." + headerName
			newSealedConfig[sealedKey] = base64.StdEncoding.EncodeToString(encrypted)
			headers[headerName] = flowSealedSentinel
			changed = true
		}

		if !changed {
			processedNodes[i] = n
			continue
		}

		maskedHeaders, err := json.Marshal(headers)
		if err != nil {
			return nil, nil, fmt.Errorf("falha ao serializar headers mascarados: %w", err)
		}
		cfgMap["headers"] = maskedHeaders
		newConfig, err := json.Marshal(cfgMap)
		if err != nil {
			return nil, nil, fmt.Errorf("falha ao serializar config do nó: %w", err)
		}
		processedNodes[i] = flow.FlowNode{
			ID:     n.ID,
			Type:   n.Type,
			Config: newConfig,
			X:      n.X,
			Y:      n.Y,
		}
	}
	return processedNodes, newSealedConfig, nil
}

// convertValidationErrors converte []flow.ValidationError para o payload de resposta.
func convertValidationErrors(errs []flow.ValidationError) []flowValidationError {
	out := make([]flowValidationError, 0, len(errs))
	for _, e := range errs {
		out = append(out, flowValidationError{
			Code:    e.Code,
			Message: e.Message,
			NodeID:  e.NodeID,
		})
	}
	return out
}
