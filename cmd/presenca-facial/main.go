package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/jotjunior/face-attendance/internal/config"
	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/flowengine"
	"github.com/jotjunior/face-attendance/internal/gob"
	"github.com/jotjunior/face-attendance/internal/hikvision"
	httphandler "github.com/jotjunior/face-attendance/internal/http"
	"github.com/jotjunior/face-attendance/internal/logging"
	"github.com/jotjunior/face-attendance/internal/queue"
	"github.com/jotjunior/face-attendance/internal/repository"
	"github.com/jotjunior/face-attendance/internal/scheduler"
	"github.com/jotjunior/face-attendance/internal/secrets"
	"github.com/jotjunior/face-attendance/internal/web"
	"github.com/jotjunior/face-attendance/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger := logging.New()

	// --- Database ---
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := pgxpool.New(dbCtx, cfg.DatabaseURL)
	dbCancel()
	if err != nil {
		return fmt.Errorf("db: connect: %w", err)
	}
	defer pool.Close()

	// --- Repositories ---
	memberRepo := repository.NewMemberRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	eventRepo := repository.NewAttendanceEventRepository(pool)
	outcomeRepo := repository.NewProcessingOutcomeRepository(pool)

	// Repositórios de fluxo (face-flow — FASE 4, tasks.md §4.3.3)
	flowRepo := repository.NewPgxFlowRepository(pool)
	bgImageRepo := repository.NewPgxBackgroundImageRepository(pool)
	flowLogRepo := repository.NewPgxFlowExecutionLogRepository(pool)

	// Criar diretório de imagens de fundo se não existir (tasks.md §4.3.2).
	if mkdirErr := os.MkdirAll(cfg.BackgroundImagesDir, 0o755); mkdirErr != nil {
		logger.Warn("http_server_started", "", "", fmt.Sprintf("background-images dir: %s: %s", cfg.BackgroundImagesDir, mkdirErr))
	}

	// --- GOB client ---
	gobClient := gob.New(cfg.GobStateURL, cfg.GobStateToken)

	// --- RabbitMQ ---
	// O publisher também é necessário quando o painel admin está ligado: o reenvio
	// individual de membro (POST /admin/api/members/{id}/resync) publica na fila,
	// não só o scheduler/workers.
	adminUIEnabled := cfg.AdminUsername != "" && cfg.AdminSessionSecret != ""
	var pub *queue.Publisher
	var amqpConn *amqp.Connection
	if cfg.RunScheduler || cfg.RunWorkers || adminUIEnabled {
		conn, ch, amqpErr := queue.Connect(cfg.RabbitMQURL)
		if amqpErr != nil {
			return fmt.Errorf("rabbitmq: %w", amqpErr)
		}
		amqpConn = conn
		defer conn.Close() //nolint:errcheck
		defer ch.Close()   //nolint:errcheck

		if topologyErr := queue.SetupTopology(ch); topologyErr != nil {
			return fmt.Errorf("rabbitmq: topology: %w", topologyErr)
		}

		pub = queue.NewPublisher(ch)
	}

	// --- Scheduler ---
	sched := scheduler.New(
		gobClient,
		scheduler.NewMemberRepository(memberRepo),
		publisherOrNoop(pub),
		logger,
		cfg.MemberSyncIntervalMinutes,
	)

	// --- Motor de fluxo (face-flow — FASE 4, tasks.md §4.3.3) ---
	// O motor é inicializado quando RUN_HTTP está ativo; wiring condicional por design
	// (tasks.md §4.4.2: "se o disparo do fluxo no webhook exigir o Engine no processo HTTP").
	var flowCipher *secrets.Cipher
	if len(cfg.ISAPICredKey) > 0 {
		// Reutilizar a chave ISAPI para cifrar segredos de header dos fluxos.
		c, cipherErr := secrets.NewCipher(cfg.ISAPICredKey)
		if cipherErr == nil {
			flowCipher = c
		}
	}

	// hikClientFor cria um cliente ISAPI para o device passado (para nós change_background /
	// qrcode_background). Usa o mesmo resolver de credenciais do worker (lê do banco).
	hikClientFor := func(device *domain.Device) (*hikvision.Client, error) {
		if device == nil {
			return nil, fmt.Errorf("device nil; não é possível criar cliente ISAPI")
		}
		// Tentar resolver credenciais do banco para este device.
		if device.IPAddress == nil || *device.IPAddress == "" {
			return nil, fmt.Errorf("device id=%d sem IP registrado", device.ID)
		}
		host := *device.IPAddress
		user := ""
		pass := ""
		// Fallback para credenciais do .env quando sem banco.
		if len(cfg.ISAPIDevices) > 0 {
			user = cfg.ISAPIDevices[0].Username
			pass = cfg.ISAPIDevices[0].Password
		}
		if flowCipher != nil {
			conns, listErr := deviceRepo.ListActiveConns(context.Background())
			if listErr == nil {
				for _, c := range conns {
					if c.ID != device.ID {
						continue
					}
					if c.IP != nil && *c.IP != "" {
						host = *c.IP
					}
					if len(c.PasswordEnc) > 0 {
						if dec, decErr := flowCipher.Decrypt(c.PasswordEnc); decErr == nil {
							user = c.Username
							pass = dec
						}
					}
					break
				}
			}
		}
		if host == "" {
			return nil, fmt.Errorf("device id=%d sem host resolvível", device.ID)
		}
		return hikvision.New(hikvision.DeviceConfig{Host: host, Username: user, Password: pass}), nil
	}

	var flowEngine *flowengine.Engine
	if cfg.RunHTTP {
		flowEngine = flowengine.New(flowengine.Config{
			HikClientFor: hikClientFor,
			FlowRepo:     flowRepo,
			LogRepo:      flowLogRepo,
			BgImageRepo:  bgImageRepo,
			BgImagesDir:  cfg.BackgroundImagesDir,
			Cipher:       flowCipher,
			Logger:       logger,
		})
	}

	// --- HTTP Handlers ---
	serializer := httphandler.NewSyncSerializer(cfg.AdminSyncMinIntervalSeconds)
	healthChecker := &appHealthChecker{pool: pool, amqpURL: cfg.RabbitMQURL}

	eventHandler := httphandler.NewEventHandler(
		deviceRepo,
		memberRepo,
		gobClient,
		eventRepo,
		logger,
	)
	// Injetar motor de fluxo no handler (nil-safe — tasks.md §4.4.2).
	if flowEngine != nil {
		eventHandler.SetFlowEngine(flowEngine)
	}
	healthHandler := httphandler.NewHealthHandler(healthChecker)
	adminHandler := httphandler.NewAdminSyncHandler(sched, serializer, logger)

	// Allowed IPs queried dynamically from DB (spec.md §FR-013)
	allowedIPs := func() []string {
		devices, listErr := deviceRepo.ListActive(context.Background())
		if listErr != nil {
			return nil
		}
		ips := make([]string, 0, len(devices))
		for _, d := range devices {
			if d.IPAddress != nil && *d.IPAddress != "" {
				ips = append(ips, *d.IPAddress)
			}
		}
		return ips
	}

	// Admin UI config (painel de administração web — FASE 2/3)
	adminLoginCfg := httphandler.AdminLoginConfig{
		Username:      cfg.AdminUsername,
		Password:      cfg.AdminPassword,
		SessionSecret: cfg.AdminSessionSecret,
		SessionTTL:    time.Duration(cfg.AdminSessionTTLHours) * time.Hour,
		CookieSecure:  cfg.AdminCookieSecure,
	}
	adminAPICfg := httphandler.AdminAPIConfig{
		MemberRepo:             memberRepo,
		DeviceRepo:             deviceRepo,
		AttendanceRepo:         eventRepo,
		DeviceOfflineThreshold: cfg.DeviceOfflineThresholdHours,
		Logger:                 logger,
	}

	// ISAPI credential cipher for device-config admin handlers (FASE 4 — device-config).
	// May be nil when ISAPI_CRED_KEY is absent; handlers return 503 in that case (CHK007/FR-007).
	var isapiAdminCipher *secrets.Cipher
	if len(cfg.ISAPICredKey) > 0 {
		c, cipherErr := secrets.NewCipher(cfg.ISAPICredKey)
		if cipherErr != nil {
			return fmt.Errorf("isapi cred cipher (admin): %w", cipherErr)
		}
		isapiAdminCipher = c
	}

	deviceConfigCfg := httphandler.DeviceConfigConfig{
		DeviceRepo:        deviceRepo,
		ISAPICipher:       isapiAdminCipher,
		Logger:            logger,
		WebhookPublicHost: cfg.WebhookPublicHost,
		WebhookPublicPort: cfg.WebhookPublicPort,
		WebhookPathSecret: cfg.WebhookPathSecret,
	}

	srv := httphandler.NewServer(httphandler.ServerConfig{
		Addr:                    ":8080",
		WebhookPathSecret:       cfg.WebhookPathSecret,
		AdminToken:              cfg.AdminToken,
		WebhookRateLimitPerMin:  cfg.WebhookRateLimitPerIPPerMin,
		AdminSyncMinIntervalSec: cfg.AdminSyncMinIntervalSeconds,
		EventHandler:            eventHandler,
		HealthHandler:           healthHandler,
		AdminHandler:            adminHandler,
		AllowedWebhookIPs:       allowedIPs,
		// Admin UI — habilitado quando as env vars obrigatórias estão presentes
		AdminUIEnabled: adminUIEnabled,
		AdminLoginCfg:  adminLoginCfg,
		AdminAPICfg:    adminAPICfg,
		AdminResyncCfg: httphandler.AdminResyncConfig{
			MemberFinder: memberRepo,
			Publisher:    pub, // não-nil quando adminUIEnabled (ver bloco RabbitMQ)
			Logger:       logger,
		},
		DeviceConfigCfg: deviceConfigCfg,
		// Fluxos de reconhecimento facial (face-flow — FASE 4, tasks.md §4.3.1)
		FlowsAPICfg: httphandler.AdminFlowsConfig{
			FlowRepo:   flowRepo,
			DeviceRepo: deviceRepo,
			LogRepo:    flowLogRepo,
			Cipher:     flowCipher,
			Logger:     logger,
		},
		BackgroundImgCfg: httphandler.AdminBackgroundImagesConfig{
			Repo:      bgImageRepo,
			ImagesDir: cfg.BackgroundImagesDir,
			Logger:    logger,
		},
		AdminAssets: http.FS(web.Assets), // embed.FS — assets populados na FASE 3
	})

	// --- Orchestration context (definido antes dos workers p/ o consumer usar) ---
	rootCtx, rootCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer rootCancel()

	// --- Worker: consumer da fila de provisionamento de membros ---
	// Consome member.processing e executa as operações ISAPI (cadastro de
	// usuário + upload de face) no leitor. A topologia é de fila única, então
	// provisiona o primeiro device configurado.
	if cfg.RunWorkers && pub != nil && amqpConn != nil && len(cfg.ISAPIDevices) > 0 {
		// .env fornece as credenciais de bootstrap (assume a mesma senha admin nos
		// leitores — caso comum num mesmo deploy; pode ser sobrescrito por device).
		devCfg := cfg.ISAPIDevices[0]

		// Cifra das credenciais ISAPI (opcional). Sem ISAPI_CRED_KEY, mantém o
		// comportamento legado: credenciais vêm do .env (sem persistir no banco).
		var credCipher *secrets.Cipher
		if len(cfg.ISAPICredKey) > 0 {
			c, cipherErr := secrets.NewCipher(cfg.ISAPICredKey)
			if cipherErr != nil {
				return fmt.Errorf("isapi cred cipher: %w", cipherErr)
			}
			credCipher = c
		}

		// Bootstrap: semeia as credenciais do .env (cifradas) em TODOS os devices
		// ativos que ainda não têm credenciais. A partir daí a fonte de verdade da
		// conexão é a tabela devices (IP segue o heartbeat).
		if credCipher != nil {
			if conns, listErr := deviceRepo.ListActiveConns(context.Background()); listErr == nil {
				for _, c := range conns {
					if len(c.PasswordEnc) > 0 {
						continue
					}
					if enc, encErr := credCipher.Encrypt(devCfg.Password); encErr == nil {
						if seedErr := deviceRepo.SetCredentials(context.Background(), c.ID, devCfg.Username, enc, 80); seedErr == nil {
							logger.Info("worker_started", "", "", fmt.Sprintf("credenciais ISAPI semeadas do .env no device id=%d (cifradas)", c.ID))
						}
					}
				}
			}
		}

		// Resolver multi-device: a cada mensagem resolve TODOS os devices ativos com
		// IP + credenciais CORRENTES do banco (absorve troca de IP e novos devices).
		// Fallback para o .env enquanto um device não tiver credenciais no banco.
		resolver := &dbConnResolver{
			repo:     deviceRepo,
			cipher:   credCipher,
			fallback: hikvision.DeviceConfig{Host: devCfg.Host, Username: devCfg.Username, Password: devCfg.Password},
		}

		// Best-effort: para cada device-alvo, busca serial/model/firmware via ISAPI
		// deviceInfo e persiste (identidade de hardware que o heartbeat não traz).
		go func() {
			infoCtx, infoCancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer infoCancel()
			targets, resErr := resolver.Resolve(infoCtx)
			if resErr != nil {
				logger.Warn("worker_started", "", "", "deviceInfo: resolver falhou: "+resErr.Error())
				return
			}
			for _, tgt := range targets {
				hc, ok := tgt.Client.(*hikvision.Client)
				if !ok {
					continue
				}
				// Provisiona os gates de acesso no device (relógio + verify mode de
				// face), independente do deviceInfo: garante que quem é reconhecido
				// seja LIBERADO, sem depender de config manual no leitor.
				ensureDeviceReady(infoCtx, hc, tgt.DeviceID, cfg, logger)
				info, infoErr := hc.FetchDeviceInfo(infoCtx)
				if infoErr != nil {
					logger.Warn("worker_started", "", "", fmt.Sprintf("deviceInfo (best-effort) device id=%d falhou: %s", tgt.DeviceID, infoErr.Error()))
					continue
				}
				if setErr := deviceRepo.SetDeviceInfo(infoCtx, tgt.DeviceID, info.SerialNumber, info.Model, info.FirmwareVersion); setErr == nil {
					logger.Info("worker_started", "", "", fmt.Sprintf("deviceInfo device id=%d: serial=%s model=%s", tgt.DeviceID, info.SerialNumber, info.Model))
				}
			}
		}()

		// webhookURL vazio + configureWebhook=false: o worker NÃO reconfigura o
		// webhook do leitor (configurado uma vez, fora do provisionamento).
		proc := worker.NewProcessor(
			resolver,
			outcomeRepo,
			pub,
			cfg.RetryMaxAttempts,
			cfg.RetryInitialBackoffMs,
			"",
			false,
		)

		consumerCh, chErr := amqpConn.Channel()
		if chErr != nil {
			return fmt.Errorf("rabbitmq: consumer channel: %w", chErr)
		}
		defer consumerCh.Close() //nolint:errcheck
		if qosErr := consumerCh.Qos(10, 0, false); qosErr != nil {
			return fmt.Errorf("rabbitmq: consumer qos: %w", qosErr)
		}
		deliveries, consErr := consumerCh.Consume(queue.QueueName, "", false, false, false, false, nil)
		if consErr != nil {
			return fmt.Errorf("rabbitmq: consume %s: %w", queue.QueueName, consErr)
		}

		go func() {
			for d := range deliveries {
				if procErr := proc.ProcessDelivery(rootCtx, d); procErr != nil {
					logger.Error("worker_processing", devCfg.Host, "", "delivery failed", procErr)
				}
			}
		}()

		logger.Info("worker_started", "", "", "consumer iniciado — provisiona em todos os leitores ativos (multi-device)")
	}

	if cfg.RunScheduler {
		go sched.Start(rootCtx)
	}

	if cfg.RunHTTP {
		go func() {
			logger.Info("http_server_started", "", "", "listening on :8080")
			if listenErr := srv.ListenAndServe(); listenErr != nil {
				logger.Error("http_server_started", "", "", "server error", listenErr)
				rootCancel()
			}
		}()
	}

	<-rootCtx.Done()
	logger.Info("http_server_started", "", "", "shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	return srv.Shutdown(shutdownCtx)
}

// dbConnResolver resolve a conexão ISAPI de TODOS os devices ativos lendo o banco a
// cada mensagem: pega o IP CORRENTE (atualizado pelo heartbeat) + as credenciais
// cifradas de cada device. Troca de IP é seguida e novos devices entram sozinhos.
// Fallback para o .env quando um device ainda não tem credenciais no banco.
type dbConnResolver struct {
	repo     *repository.DeviceRepository
	cipher   *secrets.Cipher // nil = sem cifragem (usa fallback do .env)
	fallback hikvision.DeviceConfig
}

// Resolve implements worker.ConnResolver — um Target por device ativo.
func (r *dbConnResolver) Resolve(ctx context.Context) ([]worker.Target, error) {
	conns, err := r.repo.ListActiveConns(ctx)
	if err != nil {
		return nil, err
	}
	var targets []worker.Target
	for _, c := range conns {
		host := r.fallback.Host
		user := r.fallback.Username
		pass := r.fallback.Password
		if c.IP != nil && *c.IP != "" {
			host = *c.IP
			if c.Port > 0 && c.Port != 80 {
				host = fmt.Sprintf("%s:%d", *c.IP, c.Port)
			}
		}
		// Credenciais do banco (cifradas) têm precedência sobre o .env.
		if r.cipher != nil && len(c.PasswordEnc) > 0 {
			dec, decErr := r.cipher.Decrypt(c.PasswordEnc)
			if decErr != nil {
				continue // pula este device sem derrubar os demais
			}
			user = c.Username
			pass = dec
		}
		if host == "" {
			continue
		}
		client := hikvision.New(hikvision.DeviceConfig{Host: host, Username: user, Password: pass})
		targets = append(targets, worker.Target{Client: client, DeviceID: c.ID})
	}
	return targets, nil
}

// ensureDeviceReady provisiona, no startup e por device, os gates de acesso que o
// código vinha vinculando mas NÃO garantia — a causa de um leitor reconhecer a face
// e mesmo assim NEGAR o acesso. Best-effort: nenhuma falha aqui derruba o worker.
//   - Relógio: drift fora do limite invalida a janela Valid/horário do usuário.
//   - VerifyWeekPlan: se o slot não aceita face, o reconhecimento não vira liberação.
func ensureDeviceReady(ctx context.Context, hc *hikvision.Client, deviceID int64, cfg *config.Config, logger *logging.Logger) {
	if cfg.DeviceClockGuard {
		ensureDeviceClock(ctx, hc, deviceID, cfg, logger)
	}
	if cfg.DeviceEnsureFaceVerifyMode {
		changed, err := hc.EnsureFaceVerifyMode(ctx)
		switch {
		case err != nil:
			logger.Warn("device_ready", "", "", fmt.Sprintf("device id=%d: EnsureFaceVerifyMode falhou: %s", deviceID, err.Error()))
		case changed:
			logger.Info("device_ready", "", "", fmt.Sprintf("device id=%d: verifyMode ajustado p/ aceitar face 24/7 (faceOrFpOrCardOrPw)", deviceID))
		}
	}
}

// ensureDeviceClock checa o desvio do relógio do device e, se DEVICE_CLOCK_AUTOCORRECT,
// corrige: device em manual → SetTime manual no MESMO fuso reportado; device em NTP →
// re-assert NTP (drift com NTP ligado indica servidor NTP inalcançável).
func ensureDeviceClock(ctx context.Context, hc *hikvision.Client, deviceID int64, cfg *config.Config, logger *logging.Logger) {
	td, err := hc.GetTime(ctx)
	if err != nil {
		logger.Warn("device_ready", "", "", fmt.Sprintf("device id=%d: GetTime falhou: %s", deviceID, err.Error()))
		return
	}
	devT, drift, ok := hikvision.ClockDrift(td.LocalTime, time.Now())
	if !ok {
		logger.Warn("device_ready", "", "", fmt.Sprintf("device id=%d: relógio sem offset de fuso (%q); não dá p/ medir drift com segurança", deviceID, td.LocalTime))
		return
	}
	if drift < 0 {
		drift = -drift
	}
	if drift <= time.Duration(cfg.DeviceClockMaxDriftSeconds)*time.Second {
		return // dentro do limite
	}
	logger.Warn("device_clock_drift", "", "", fmt.Sprintf("device id=%d: drift=%s mode=%s tz=%s", deviceID, drift.Round(time.Second), td.TimeMode, td.TimeZone))
	if !cfg.DeviceClockAutocorrect {
		return
	}
	if strings.EqualFold(td.TimeMode, "ntp") {
		if serr := hc.SetTime(ctx, hikvision.TimeSetRequest{TimeMode: "ntp", TimeZone: td.TimeZone}); serr != nil {
			logger.Warn("device_clock_drift", "", "", fmt.Sprintf("device id=%d: re-assert NTP falhou: %s", deviceID, serr.Error()))
		} else {
			logger.Info("device_clock_drift", "", "", fmt.Sprintf("device id=%d: NTP re-asserido (drift com NTP ligado sugere servidor NTP inalcançável)", deviceID))
		}
		return
	}
	// Manual: corrige p/ o agora no MESMO fuso que o device reportou (não inventa fuso).
	corrected := time.Now().In(devT.Location()).Format("2006-01-02T15:04:05")
	if serr := hc.SetTime(ctx, hikvision.TimeSetRequest{TimeMode: "manual", LocalTime: corrected, TimeZone: td.TimeZone}); serr != nil {
		logger.Warn("device_clock_drift", "", "", fmt.Sprintf("device id=%d: SetTime manual falhou: %s", deviceID, serr.Error()))
	} else {
		logger.Info("device_clock_drift", "", "", fmt.Sprintf("device id=%d: relógio corrigido (manual) p/ %s", deviceID, corrected))
	}
}

// appHealthChecker implements httphandler.HealthChecker.
type appHealthChecker struct {
	pool    *pgxpool.Pool
	amqpURL string
}

func (h *appHealthChecker) PingDB(ctx context.Context) error {
	return h.pool.Ping(ctx)
}

func (h *appHealthChecker) PingRabbitMQ() error {
	if h.amqpURL == "" {
		return fmt.Errorf("rabbitmq: not configured")
	}
	conn, ch, err := queue.Connect(h.amqpURL)
	if err != nil {
		return err
	}
	ch.Close()   //nolint:errcheck
	conn.Close() //nolint:errcheck
	return nil
}

// publisherOrNoop returns pub if not nil; otherwise a no-op publisher.
func publisherOrNoop(pub *queue.Publisher) scheduler.ProcessingPublisher {
	if pub != nil {
		return pub
	}
	return noopPublisher{}
}

type noopPublisher struct{}

func (noopPublisher) Publish(_ context.Context, _ domain.ProcessingMessage) error { return nil }

// Ensure *queue.Publisher satisfies scheduler.ProcessingPublisher at compile time.
var _ scheduler.ProcessingPublisher = (*queue.Publisher)(nil)

// Ensure appHealthChecker satisfies httphandler.HealthChecker.
var _ interface {
	PingDB(ctx context.Context) error
	PingRabbitMQ() error
} = (*appHealthChecker)(nil)

// net/http import used via httphandler — suppress unused warning.
var _ = http.StatusOK
