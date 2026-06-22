//go:build integration
// +build integration

// Integration test (PostgreSQL real) para GET /admin/api/devices/{id} — task 4.1.4.
// Exercita o caminho completo handler → DeviceRepository.GetDeviceByID → toDeviceResponse
// contra o banco real, complementando os testes unitários (que usam mock do repo):
//   - sem capacidades/credenciais: max_users == null e isapi_credentials_set == false
//   - após SetCredentials: isapi_credentials_set == true
// Run: go test -tags integration ./internal/http/...
package httphandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/repository"
)

func itDevicePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://presenca:presenca_dev@localhost:5432/presenca_facial?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestAdminDeviceDetail_CredentialsAndCaps_Integration(t *testing.T) {
	pool := itDevicePool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE devices RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate devices: %v", err)
	}

	repo := repository.NewDeviceRepository(pool)
	const ident = "AA:11:BB:22:CC:33"
	if err := repo.Upsert(ctx, domain.Device{DeviceIdentifier: ident}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	dev, err := repo.FindByIdentifier(ctx, ident)
	if err != nil || dev == nil {
		t.Fatalf("FindByIdentifier: %v (dev=%v)", err, dev)
	}

	handler := AdminDeviceDetailHandler(AdminAPIConfig{DeviceRepo: repo, DeviceOfflineThreshold: 24})
	getDevice := func() map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/admin/api/devices/"+strconv.FormatInt(dev.ID, 10), nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
		}
		var m map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
			t.Fatalf("unmarshal resposta: %v", err)
		}
		return m
	}

	// Sem capacidades/credenciais: max_users null + isapi_credentials_set false.
	m := getDevice()
	if m["max_users"] != nil {
		t.Errorf("max_users = %v, want null", m["max_users"])
	}
	if m["isapi_credentials_set"] != false {
		t.Errorf("isapi_credentials_set = %v, want false", m["isapi_credentials_set"])
	}

	// Após SetCredentials (username + password_enc não-nil): isapi_credentials_set true.
	if err := repo.SetCredentials(ctx, dev.ID, "admin", []byte{0x01, 0x02, 0x03}, 80); err != nil {
		t.Fatalf("SetCredentials: %v", err)
	}
	m = getDevice()
	if m["isapi_credentials_set"] != true {
		t.Errorf("após SetCredentials: isapi_credentials_set = %v, want true", m["isapi_credentials_set"])
	}
	// A senha em claro/encriptada nunca deve aparecer na resposta (FR-005).
	if _, leaked := m["isapi_password"]; leaked {
		t.Error("resposta contém isapi_password — senha nunca deve ser serializada")
	}
}
