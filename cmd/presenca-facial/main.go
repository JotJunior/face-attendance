package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/jotjunior/face-attendance/internal/config"
	"github.com/jotjunior/face-attendance/internal/domain"
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
		devCfg := cfg.ISAPIDevices[0]
		if len(cfg.ISAPIDevices) > 1 {
			logger.Info("worker_started", "", "", "múltiplos devices configurados; a fila única provisiona apenas o primeiro")
		}

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

		// Resolve o device-alvo do provisionamento. Em deployment single-device
		// (o caso real), é o ÚNICO device ativo — independente de IP, que é o ponto
		// central do pedido: o device troca de IP e o alvo continua o mesmo. Com
		// múltiplos devices, casa o host do .env com o IP corrente (melhor esforço).
		var deviceID int64
		if devices, listErr := deviceRepo.ListActive(context.Background()); listErr == nil {
			if len(devices) == 1 {
				deviceID = devices[0].ID
			} else {
				for _, d := range devices {
					if d.IPAddress != nil && *d.IPAddress == devCfg.Host {
						deviceID = d.ID
						break
					}
				}
			}
		}

		// Bootstrap: semeia as credenciais do .env no banco (cifradas) uma única
		// vez, quando a cifragem está ligada e o device ainda não tem credenciais.
		// A partir daí a fonte de verdade da conexão é a tabela devices.
		if credCipher != nil && deviceID != 0 {
			if has, _ := deviceRepo.HasCredentials(context.Background(), deviceID); !has {
				if enc, encErr := credCipher.Encrypt(devCfg.Password); encErr == nil {
					if seedErr := deviceRepo.SetCredentials(context.Background(), deviceID, devCfg.Username, enc, 80); seedErr == nil {
						logger.Info("worker_started", devCfg.Host, "", "credenciais ISAPI semeadas do .env para o banco (cifradas)")
					}
				}
			}
		}

		// Resolver per-message: usa IP + credenciais CORRENTES do banco (absorve
		// troca de IP). Fallback para o .env enquanto não houver credenciais no banco.
		resolver := &dbConnResolver{
			repo:     deviceRepo,
			cipher:   credCipher,
			deviceID: deviceID,
			fallback: hikvision.DeviceConfig{Host: devCfg.Host, Username: devCfg.Username, Password: devCfg.Password},
		}

		// Best-effort: busca serial/model/firmware via ISAPI deviceInfo e persiste
		// (a identidade de hardware que o heartbeat não traz). Usa o resolver — ou
		// seja, o IP CORRENTE do banco, não o host estático do .env. Não bloqueia o boot.
		if deviceID != 0 {
			go func() {
				infoCtx, infoCancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer infoCancel()
				client, id, resErr := resolver.Resolve(infoCtx)
				if resErr != nil {
					return
				}
				hc, ok := client.(*hikvision.Client)
				if !ok {
					return
				}
				if info, infoErr := hc.FetchDeviceInfo(infoCtx); infoErr == nil && info != nil {
					if setErr := deviceRepo.SetDeviceInfo(infoCtx, id, info.SerialNumber, info.Model, info.FirmwareVersion); setErr == nil {
						logger.Info("worker_started", "", "", "deviceInfo (serial/model/firmware) atualizado via ISAPI")
					}
				}
			}()
		}

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

		logger.Info("worker_started", devCfg.Host, "", "consumer iniciado",
			slog.Int("device_index", devCfg.Index),
			slog.Int64("device_id", deviceID))
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

// dbConnResolver resolve a conexão ISAPI do device-alvo lendo o banco a cada
// mensagem: pega o IP CORRENTE (atualizado pelo heartbeat) + as credenciais
// cifradas. Assim, se o device troca de IP, o provisionamento segue o IP novo —
// nunca se perde a conexão. Fallback para o .env enquanto o banco não tem credenciais.
type dbConnResolver struct {
	repo     *repository.DeviceRepository
	cipher   *secrets.Cipher // nil = sem cifragem (usa fallback do .env)
	deviceID int64
	fallback hikvision.DeviceConfig
}

// Resolve implements worker.ConnResolver.
func (r *dbConnResolver) Resolve(ctx context.Context) (worker.ISAPIClient, int64, error) {
	host := r.fallback.Host
	user := r.fallback.Username
	pass := r.fallback.Password
	devID := r.deviceID

	if r.deviceID != 0 {
		if conn, err := r.repo.GetConn(ctx, r.deviceID); err == nil && conn != nil {
			devID = conn.ID
			if conn.IP != nil && *conn.IP != "" {
				host = *conn.IP
				if conn.Port > 0 && conn.Port != 80 {
					host = fmt.Sprintf("%s:%d", *conn.IP, conn.Port)
				}
			}
			// Credenciais do banco (cifradas) têm precedência sobre o .env.
			if r.cipher != nil && len(conn.PasswordEnc) > 0 {
				dec, decErr := r.cipher.Decrypt(conn.PasswordEnc)
				if decErr != nil {
					return nil, 0, fmt.Errorf("decrypt device %d credentials: %w", conn.ID, decErr)
				}
				user = conn.Username
				pass = dec
			}
		}
	}

	if host == "" {
		return nil, 0, fmt.Errorf("no ISAPI host for device %d", devID)
	}
	client := hikvision.New(hikvision.DeviceConfig{Host: host, Username: user, Password: pass})
	return client, devID, nil
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
