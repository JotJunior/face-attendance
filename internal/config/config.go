// Package config reads application configuration from environment variables.
// No secrets are stored in code (Constitution Principle V).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jotjunior/face-attendance/internal/secrets"
)

// Config holds all runtime configuration for the presenca-facial service.
type Config struct {
	// GOB API
	GobStateURL   string // GOB_STATE_URL
	GobStateToken string // GOB_STATE_TOKEN

	// API de disparo de mensagem (nó send_message do face-flow).
	// Contrato multipart fornecido pelo operador: POST <URL> com campos
	// appkey/authkey/to/message. Opcionais — sem eles, o nó send_message falha
	// (circuit-break) com mensagem clara. AppKey/AuthKey são SEGREDOS: nunca logar.
	SenderURL     string // SENDER_URL
	SenderAppKey  string // SENDER_APP_KEY (sensitive — never log)
	SenderAuthKey string // SENDER_AUTH_KEY (sensitive — never log)

	// Scheduler
	MemberSyncIntervalMinutes int // MEMBER_SYNC_INTERVAL_MINUTES (default: 60)

	// Retry policy
	RetryMaxAttempts      int // RETRY_MAX_ATTEMPTS (default: 3)
	RetryInitialBackoffMs int // RETRY_INITIAL_BACKOFF_MS (default: 1000)

	// Feature flags
	RunHTTP       bool // RUN_HTTP (default: true)
	RunScheduler  bool // RUN_SCHEDULER (default: true)
	RunWorkers    bool // RUN_WORKERS (default: true)

	// Endereço de escuta do servidor HTTP (default ":8080"). Permite rodar local
	// numa porta alternativa quando a 8080 está ocupada (ex.: HTTP_ADDR=":8090").
	HTTPAddr string // HTTP_ADDR

	// Security
	AdminToken        string // ADMIN_TOKEN
	WebhookPathSecret string // WEBHOOK_PATH_SECRET

	// Endereço público que os terminais HikVision usam para POSTar eventos no
	// webhook (httpHost). É o IP:porta do app alcançável na LAN pelos leitores —
	// NÃO é derivável do request (atrás de proxy/NAT), por isso é config explícita.
	// Necessário para a ação de provisionar/reparar o webhook pelo painel; vazio
	// → a ação retorna erro pedindo a configuração (nunca inventa o IP).
	WebhookPublicHost string // WEBHOOK_PUBLIC_HOST (opcional; ex.: 192.168.68.110)
	WebhookPublicPort int    // WEBHOOK_PUBLIC_PORT (default: 8080)

	// Admin UI authentication
	AdminUsername          string // ADMIN_USERNAME (required)
	AdminPassword          string // ADMIN_PASSWORD (required, sensitive — never log)
	AdminSessionSecret     string // ADMIN_SESSION_SECRET (required, sensitive — never log)
	AdminSessionTTLHours   int    // ADMIN_SESSION_TTL_HOURS (default: 8)
	AdminCookieSecure      bool   // ADMIN_COOKIE_SECURE (default: true; setar false p/ deploy HTTP on-premise sem TLS)

	// Device monitoring
	DeviceOfflineThresholdHours int // DEVICE_OFFLINE_THRESHOLD_HOURS (default: 24)

	// Device readiness (gates de acesso provisionados no startup, por device).
	// Garantem que o leitor LIBERA quem reconhece — sem depender de config manual.
	DeviceClockGuard           bool // DEVICE_CLOCK_GUARD (default: true) — checa drift do relógio
	DeviceClockAutocorrect     bool // DEVICE_CLOCK_AUTOCORRECT (default: true) — corrige drift via SetTime/NTP
	DeviceClockMaxDriftSeconds int  // DEVICE_CLOCK_MAX_DRIFT_SECONDS (default: 120)
	DeviceEnsureFaceVerifyMode bool // DEVICE_ENSURE_FACE_VERIFY_MODE (default: true) — garante face 24/7

	// Rate limiting
	WebhookRateLimitPerIPPerMin int // WEBHOOK_RATE_LIMIT_PER_IP_PER_MIN (default: 60)
	AdminSyncMinIntervalSeconds int // ADMIN_SYNC_MIN_INTERVAL_SECONDS (default: 60)

	// Database
	DatabaseURL string // DATABASE_URL (postgres DSN)

	// RabbitMQ
	RabbitMQURL string // RABBITMQ_URL (amqp DSN)

	// Per-device ISAPI credentials (ISAPI_DEVICE_{N}_HOST, _USER, _PASSWORD).
	// Legado/bootstrap: usados apenas para semear as credenciais no banco na 1ª
	// inicialização. A fonte de verdade da conexão passa a ser a tabela devices.
	ISAPIDevices []ISAPIDeviceConfig

	// Chave mestra (32 bytes) para cifrar credenciais ISAPI no banco (AES-256-GCM).
	// Env: ISAPI_CRED_KEY (hex de 64 chars ou base64). Opcional: vazio mantém o
	// comportamento legado (credenciais do .env, sem persistir/cifrar no banco).
	ISAPICredKey []byte

	// BackgroundImagesDir é o diretório local onde as imagens de fundo dos fluxos são
	// armazenadas. Env: BACKGROUND_IMAGES_DIR (default: ./data/background-images).
	// Criado automaticamente na inicialização se não existir (tasks.md §4.3.2).
	BackgroundImagesDir string
}

// ISAPIDeviceConfig holds credentials for one HikVision device.
// Env vars: ISAPI_DEVICE_{N}_HOST, ISAPI_DEVICE_{N}_USER, ISAPI_DEVICE_{N}_PASSWORD
// where N starts at 1 and increments until no ISAPI_DEVICE_{N}_HOST is found.
type ISAPIDeviceConfig struct {
	Index    int
	Host     string // hostname or IP:port
	Username string
	Password string // sensitive — never log
}

// Load reads the configuration from environment variables.
// Returns an error listing all missing required variables.
func Load() (*Config, error) {
	var missing []string

	require := func(name string) string {
		v := os.Getenv(name)
		if v == "" {
			missing = append(missing, name)
		}
		return v
	}

	optionalStr := func(name, defaultVal string) string {
		if v := os.Getenv(name); v != "" {
			return v
		}
		return defaultVal
	}

	optionalInt := func(name string, defaultVal int) int {
		s := os.Getenv(name)
		if s == "" {
			return defaultVal
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			// log-safe: invalid int env var uses default
			return defaultVal
		}
		return n
	}

	optionalBool := func(name string, defaultVal bool) bool {
		s := strings.ToLower(os.Getenv(name))
		if s == "" {
			return defaultVal
		}
		return s == "true" || s == "1" || s == "yes"
	}

	cfg := &Config{
		GobStateURL:               require("GOB_STATE_URL"),
		GobStateToken:             require("GOB_STATE_TOKEN"),
		SenderURL:                 optionalStr("SENDER_URL", ""),
		SenderAppKey:              optionalStr("SENDER_APP_KEY", ""),
		SenderAuthKey:             optionalStr("SENDER_AUTH_KEY", ""),
		HTTPAddr:                  optionalStr("HTTP_ADDR", ":8080"),
		AdminToken:                require("ADMIN_TOKEN"),
		WebhookPathSecret:         require("WEBHOOK_PATH_SECRET"),
		WebhookPublicHost:         optionalStr("WEBHOOK_PUBLIC_HOST", ""),
		WebhookPublicPort:         optionalInt("WEBHOOK_PUBLIC_PORT", 8080),
		DatabaseURL:               require("DATABASE_URL"),
		RabbitMQURL:               require("RABBITMQ_URL"),
		MemberSyncIntervalMinutes: optionalInt("MEMBER_SYNC_INTERVAL_MINUTES", 60),
		RetryMaxAttempts:          optionalInt("RETRY_MAX_ATTEMPTS", 3),
		RetryInitialBackoffMs:     optionalInt("RETRY_INITIAL_BACKOFF_MS", 1000),
		RunHTTP:                   optionalBool("RUN_HTTP", true),
		RunScheduler:              optionalBool("RUN_SCHEDULER", true),
		RunWorkers:                optionalBool("RUN_WORKERS", true),
		WebhookRateLimitPerIPPerMin: optionalInt("WEBHOOK_RATE_LIMIT_PER_IP_PER_MIN", 60),
		AdminSyncMinIntervalSeconds: optionalInt("ADMIN_SYNC_MIN_INTERVAL_SECONDS", 60),
		// Admin UI authentication (FASE 2 — interface de administração)
		AdminUsername:               require("ADMIN_USERNAME"),
		AdminPassword:               require("ADMIN_PASSWORD"),
		AdminSessionSecret:          require("ADMIN_SESSION_SECRET"),
		AdminSessionTTLHours:        optionalInt("ADMIN_SESSION_TTL_HOURS", 8),
		AdminCookieSecure:           optionalBool("ADMIN_COOKIE_SECURE", true),
		// Device monitoring threshold
		DeviceOfflineThresholdHours: optionalInt("DEVICE_OFFLINE_THRESHOLD_HOURS", 24),
		// Device readiness (gates provisionados no startup)
		DeviceClockGuard:           optionalBool("DEVICE_CLOCK_GUARD", true),
		DeviceClockAutocorrect:     optionalBool("DEVICE_CLOCK_AUTOCORRECT", true),
		DeviceClockMaxDriftSeconds: optionalInt("DEVICE_CLOCK_MAX_DRIFT_SECONDS", 120),
		DeviceEnsureFaceVerifyMode: optionalBool("DEVICE_ENSURE_FACE_VERIFY_MODE", true),
	}

	// Load per-device ISAPI configs: ISAPI_DEVICE_1_HOST, ISAPI_DEVICE_2_HOST, ...
	cfg.ISAPIDevices = loadISAPIDevices(optionalStr)

	cfg.BackgroundImagesDir = optionalStr("BACKGROUND_IMAGES_DIR", "./data/background-images")

	// Chave de cifragem das credenciais ISAPI (opcional). Se setada mas inválida,
	// é um erro de configuração (não silenciamos um segredo malformado).
	if raw := os.Getenv("ISAPI_CRED_KEY"); raw != "" {
		key, keyErr := secrets.ParseKey(raw)
		if keyErr != nil {
			missing = append(missing, "ISAPI_CRED_KEY (inválida: "+keyErr.Error()+")")
		} else {
			cfg.ISAPICredKey = key
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// loadISAPIDevices scans ISAPI_DEVICE_{N}_HOST/USER/PASSWORD until no host is found.
func loadISAPIDevices(optionalStr func(string, string) string) []ISAPIDeviceConfig {
	var devices []ISAPIDeviceConfig
	for n := 1; ; n++ {
		host := os.Getenv(fmt.Sprintf("ISAPI_DEVICE_%d_HOST", n))
		if host == "" {
			break
		}
		user := optionalStr(fmt.Sprintf("ISAPI_DEVICE_%d_USER", n), "admin")
		pass := os.Getenv(fmt.Sprintf("ISAPI_DEVICE_%d_PASSWORD", n))
		devices = append(devices, ISAPIDeviceConfig{
			Index:    n,
			Host:     host,
			Username: user,
			Password: pass,
		})
	}
	return devices
}

// Validate performs semantic validation beyond presence checks.
func (c *Config) Validate() error {
	var errs []string
	if c.MemberSyncIntervalMinutes <= 0 {
		errs = append(errs, "MEMBER_SYNC_INTERVAL_MINUTES must be > 0")
	}
	if c.RetryMaxAttempts <= 0 {
		errs = append(errs, "RETRY_MAX_ATTEMPTS must be > 0")
	}
	if c.RetryInitialBackoffMs <= 0 {
		errs = append(errs, "RETRY_INITIAL_BACKOFF_MS must be > 0")
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
