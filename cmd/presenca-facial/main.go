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
	var pub *queue.Publisher
	var amqpConn *amqp.Connection
	if cfg.RunScheduler || cfg.RunWorkers {
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

		isapiClient := hikvision.New(hikvision.DeviceConfig{
			Host:     devCfg.Host,
			Username: devCfg.Username,
			Password: devCfg.Password,
		})

		// Resolve o deviceID pelo device ativo cujo IP == host configurado
		// (o device se registra por MAC via heartbeat, não pelo host ISAPI).
		var deviceID int64
		if devices, listErr := deviceRepo.ListActive(context.Background()); listErr == nil {
			for _, d := range devices {
				if d.IPAddress != nil && *d.IPAddress == devCfg.Host {
					deviceID = d.ID
					break
				}
			}
		}

		// webhookURL vazio + configureWebhook=false: o worker NÃO reconfigura o
		// webhook do leitor (configurado uma vez, fora do provisionamento).
		proc := worker.NewProcessor(
			isapiClient,
			outcomeRepo,
			pub,
			deviceID,
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
