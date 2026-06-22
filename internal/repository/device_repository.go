package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jotjunior/face-attendance/internal/domain"
)

// DeviceRepository handles persistence for the devices table.
type DeviceRepository struct {
	pool *pgxpool.Pool
}

// NewDeviceRepository creates a new DeviceRepository.
func NewDeviceRepository(pool *pgxpool.Pool) *DeviceRepository {
	return &DeviceRepository{pool: pool}
}

// Upsert inserts a new device or updates last_heartbeat_at and ip_address on conflict.
// ON CONFLICT (device_identifier) — idempotent first heartbeat (FR-001) and subsequent ones (FR-002).
func (r *DeviceRepository) Upsert(ctx context.Context, d domain.Device) error {
	query := `
		INSERT INTO devices (
			device_identifier, ip_address, mac_address,
			last_heartbeat_at, is_active, webhook_configured,
			created_at, updated_at
		) VALUES ($1, $2, $3, now(), true, false, now(), now())
		ON CONFLICT (device_identifier) DO UPDATE SET
			ip_address       = EXCLUDED.ip_address,
			mac_address      = COALESCE(EXCLUDED.mac_address, devices.mac_address),
			last_heartbeat_at = now(),
			is_active        = true,
			updated_at       = now()
	`
	_, err := r.pool.Exec(ctx, query,
		d.DeviceIdentifier,
		d.IPAddress,
		d.MACAddress,
	)
	return err
}

// ListActive returns all devices with is_active = true.
func (r *DeviceRepository) ListActive(ctx context.Context) ([]domain.Device, error) {
	// host() retorna o IP sem a máscara CIDR (inet '1.2.3.4' renderiza como
	// "1.2.3.4/32" via ::text, o que quebraria a comparação do IP allowlist).
	query := `
		SELECT id, device_identifier, host(ip_address), mac_address,
		       last_heartbeat_at, is_active, webhook_configured,
		       created_at, updated_at,
		       serial_number, model, firmware_version, isapi_username, isapi_password_enc, isapi_port,
		       max_users, max_faces
		FROM devices
		WHERE is_active = true
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDevices(rows)
}

// FindByIdentifier finds a device by its device_identifier.
// Returns (nil, nil) if not found.
func (r *DeviceRepository) FindByIdentifier(ctx context.Context, identifier string) (*domain.Device, error) {
	query := `
		SELECT id, device_identifier, host(ip_address), mac_address,
		       last_heartbeat_at, is_active, webhook_configured,
		       created_at, updated_at,
		       serial_number, model, firmware_version, isapi_username, isapi_password_enc, isapi_port,
		       max_users, max_faces
		FROM devices
		WHERE device_identifier = $1
		LIMIT 1
	`
	rows, err := r.pool.Query(ctx, query, identifier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	devices, err := scanDevices(rows)
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, nil
	}
	return &devices[0], nil
}

// SetWebhookConfigured marks a device as having its webhook URL configured.
func (r *DeviceRepository) SetWebhookConfigured(ctx context.Context, identifier string, configured bool) error {
	query := `UPDATE devices SET webhook_configured = $1, updated_at = now() WHERE device_identifier = $2`
	_, err := r.pool.Exec(ctx, query, configured, identifier)
	return err
}

// SetWebhookConfiguredByID marks a device as having its webhook URL configured, by primary key.
// Used by admin handlers (factory-reset, delete-webhook) that operate on device ID, not identifier.
func (r *DeviceRepository) SetWebhookConfiguredByID(ctx context.Context, id int64, configured bool) error {
	query := `UPDATE devices SET webhook_configured = $1, updated_at = now() WHERE id = $2`
	_, err := r.pool.Exec(ctx, query, configured, id)
	return err
}

// CountDevicesByActivity conta dispositivos ativos e inativos conforme thresholdHours.
// Um dispositivo é considerado ativo se last_heartbeat_at >= now() - thresholdHours.
// Usa uma única query com CASE para evitar N+1 (CHK-P12).
func (r *DeviceRepository) CountDevicesByActivity(ctx context.Context, thresholdHours int) (active, inactive int, err error) {
	query := `
		SELECT
			COUNT(*) FILTER (WHERE last_heartbeat_at >= now() - ($1 * INTERVAL '1 hour')) AS active,
			COUNT(*) FILTER (WHERE last_heartbeat_at <  now() - ($1 * INTERVAL '1 hour')
			                    OR last_heartbeat_at IS NULL)                               AS inactive
		FROM devices
	`
	err = r.pool.QueryRow(ctx, query, thresholdHours).Scan(&active, &inactive)
	return active, inactive, err
}

// ListDevicesAll retorna todos os dispositivos sem paginação.
// Adequado para dezenas de dispositivos (on-premise single-tenant).
func (r *DeviceRepository) ListDevicesAll(ctx context.Context) ([]domain.Device, error) {
	query := `
		SELECT id, device_identifier, host(ip_address), mac_address,
		       last_heartbeat_at, is_active, webhook_configured,
		       created_at, updated_at,
		       serial_number, model, firmware_version, isapi_username, isapi_password_enc, isapi_port,
		       max_users, max_faces
		FROM devices
		ORDER BY device_identifier
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDevices(rows)
}

// GetDeviceByID retorna um dispositivo pelo ID primário.
// Retorna pgx.ErrNoRows se não encontrado (mapeado para 404 pelo handler).
func (r *DeviceRepository) GetDeviceByID(ctx context.Context, id int64) (*domain.Device, error) {
	query := `
		SELECT id, device_identifier, host(ip_address), mac_address,
		       last_heartbeat_at, is_active, webhook_configured,
		       created_at, updated_at,
		       serial_number, model, firmware_version, isapi_username, isapi_password_enc, isapi_port,
		       max_users, max_faces
		FROM devices
		WHERE id = $1
	`
	rows, err := r.pool.Query(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	devices, err := scanDevices(rows)
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, pgx.ErrNoRows
	}
	return &devices[0], nil
}

// FindByMAC finds a device by MAC address (or device_identifier == mac).
// Identidade estável do device — sobrevive a troca de IP. Retorna (nil, nil) se não achar.
func (r *DeviceRepository) FindByMAC(ctx context.Context, mac string) (*domain.Device, error) {
	query := `
		SELECT id, device_identifier, host(ip_address), mac_address,
		       last_heartbeat_at, is_active, webhook_configured,
		       created_at, updated_at,
		       serial_number, model, firmware_version, isapi_username, isapi_password_enc, isapi_port,
		       max_users, max_faces
		FROM devices
		WHERE mac_address = $1 OR device_identifier = $1
		LIMIT 1
	`
	rows, err := r.pool.Query(ctx, query, mac)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices, err := scanDevices(rows)
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, nil
	}
	return &devices[0], nil
}

// DeviceConn carrega os dados de conexão ISAPI de um device, incluindo a senha
// cifrada (isapi_password_enc). Usado pelo worker para conectar com o IP corrente.
type DeviceConn struct {
	ID          int64
	IP          *string
	Username    string
	PasswordEnc []byte
	Port        int
}

// GetConn retorna os dados de conexão ISAPI de um device pelo ID (IP corrente +
// credenciais cifradas). Retorna pgx.ErrNoRows se não encontrado.
func (r *DeviceRepository) GetConn(ctx context.Context, id int64) (*DeviceConn, error) {
	query := `
		SELECT id, host(ip_address), COALESCE(isapi_username, ''), isapi_password_enc, isapi_port
		FROM devices
		WHERE id = $1
	`
	var c DeviceConn
	err := r.pool.QueryRow(ctx, query, id).Scan(&c.ID, &c.IP, &c.Username, &c.PasswordEnc, &c.Port)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListActiveConns retorna os dados de conexão ISAPI de TODOS os devices ativos
// (IP corrente + credenciais cifradas). Usado pelo worker para provisionar um
// membro em todos os leitores (multi-device).
func (r *DeviceRepository) ListActiveConns(ctx context.Context) ([]DeviceConn, error) {
	query := `
		SELECT id, host(ip_address), COALESCE(isapi_username, ''), isapi_password_enc, isapi_port
		FROM devices
		WHERE is_active = true
		ORDER BY id
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var conns []DeviceConn
	for rows.Next() {
		var c DeviceConn
		if err := rows.Scan(&c.ID, &c.IP, &c.Username, &c.PasswordEnc, &c.Port); err != nil {
			return nil, err
		}
		conns = append(conns, c)
	}
	return conns, rows.Err()
}

// SetCredentials persiste as credenciais ISAPI cifradas de um device.
// passwordEnc é o blob AES-GCM (nonce||ciphertext) — nunca a senha em claro.
func (r *DeviceRepository) SetCredentials(ctx context.Context, id int64, username string, passwordEnc []byte, port int) error {
	if port <= 0 {
		port = 80
	}
	query := `
		UPDATE devices
		SET isapi_username = $2, isapi_password_enc = $3, isapi_port = $4, updated_at = now()
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, id, username, passwordEnc, port)
	return err
}

// HasCredentials reporta se o device já tem credenciais ISAPI persistidas (para
// não re-semear do .env a cada boot).
func (r *DeviceRepository) HasCredentials(ctx context.Context, id int64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT isapi_password_enc IS NOT NULL FROM devices WHERE id = $1`, id).Scan(&exists)
	return exists, err
}

// SetCapabilities persists the hardware capacity limits read from ISAPI GetCapabilities.
// maxUsers and maxFaces are pointers to allow NULL when not available.
// Ref: tasks.md §2.2.3; mirrors the SetCredentials pattern (device_repository.go:241).
func (r *DeviceRepository) SetCapabilities(ctx context.Context, id int64, maxUsers, maxFaces *int) error {
	query := `
		UPDATE devices
		SET max_users = $2, max_faces = $3, updated_at = now()
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, id, maxUsers, maxFaces)
	return err
}

// SetDeviceInfo persiste serial/model/firmware obtidos via ISAPI deviceInfo.
// Valores vazios viram NULL (não sobrescrevem com string vazia).
func (r *DeviceRepository) SetDeviceInfo(ctx context.Context, id int64, serial, model, firmware string) error {
	query := `
		UPDATE devices
		SET serial_number    = NULLIF($2, ''),
		    model            = NULLIF($3, ''),
		    firmware_version = NULLIF($4, ''),
		    updated_at       = now()
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, id, serial, model, firmware)
	return err
}

// scanDevices reads device rows.
// Column order must match all SELECT queries in this file (15 columns + 2 capability columns).
// isapi_password_enc é carregado (nunca serializado em JSON — json:"-") porque
// toDeviceResponse deriva isapi_credentials_set de username + password_enc não-nil.
func scanDevices(rows pgx.Rows) ([]domain.Device, error) {
	var devices []domain.Device
	for rows.Next() {
		var d domain.Device
		if err := rows.Scan(
			&d.ID,
			&d.DeviceIdentifier,
			&d.IPAddress,
			&d.MACAddress,
			&d.LastHeartbeatAt,
			&d.IsActive,
			&d.WebhookConfigured,
			&d.CreatedAt,
			&d.UpdatedAt,
			&d.SerialNumber,
			&d.Model,
			&d.FirmwareVersion,
			&d.ISAPIUsername,
			&d.ISAPIPasswordEnc,
			&d.ISAPIPort,
			&d.MaxUsers, // nullable: NULL → nil
			&d.MaxFaces, // nullable: NULL → nil
		); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}
