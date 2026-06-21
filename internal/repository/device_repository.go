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
		       created_at, updated_at
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
		       created_at, updated_at
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
		       created_at, updated_at
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
		       created_at, updated_at
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

// scanDevices reads device rows.
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
		); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}
