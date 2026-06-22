package hikvision_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jotjunior/face-attendance/internal/hikvision"
)

// TestEnsureFaceVerifyMode_Updates: slots com verifyMode que NÃO aceita face são
// reescritos p/ "faceOrFpOrCardOrPw" via read-modify-write, preservando os demais
// campos (week, TimeSegment, enable). changed=true e o PUT carrega o corpo ajustado.
func TestEnsureFaceVerifyMode_Updates(t *testing.T) {
	const getBody = `{"VerifyWeekPlanCfg":{"id":1,"enable":true,"WeekPlanCfg":[` +
		`{"week":"Monday","id":1,"enable":true,"TimeSegment":{"beginTime":"00:00","endTime":"23:59"},"verifyMode":"card"},` +
		`{"week":"Tuesday","id":1,"enable":true,"verifyMode":"faceOrFpOrCardOrPw"}` +
		`]}}`

	var putBody string
	var putCalled bool
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/AccessControl/VerifyWeekPlanCfg/1") {
			t.Errorf("path inesperado: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(getBody)) //nolint:errcheck
		case http.MethodPut:
			putCalled = true
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("método inesperado: %s", r.Method)
		}
	})
	defer srv.Close()

	changed, err := hikvision.NewWithHTTPClient(cfg, srv.Client()).EnsureFaceVerifyMode(context.Background())
	if err != nil {
		t.Fatalf("EnsureFaceVerifyMode: %v", err)
	}
	if !changed {
		t.Fatal("changed deveria ser true (slot 'card' precisa virar face)")
	}
	if !putCalled {
		t.Fatal("PUT deveria ter sido chamado")
	}
	// O slot 'card' some; ambos passam a aceitar face.
	if strings.Contains(putBody, `"verifyMode":"card"`) {
		t.Errorf("PUT ainda contém verifyMode 'card': %s", putBody)
	}
	if !strings.Contains(putBody, `"verifyMode":"faceOrFpOrCardOrPw"`) {
		t.Errorf("PUT não contém o modo de face: %s", putBody)
	}
	// Read-modify-write: campos não tocados são preservados.
	for _, want := range []string{`"week":"Monday"`, `"TimeSegment"`, `"beginTime":"00:00"`} {
		if !strings.Contains(putBody, want) {
			t.Errorf("PUT perdeu campo preservado %s: %s", want, putBody)
		}
	}
}

// TestEnsureFaceVerifyMode_Idempotent: se todos os slots já aceitam face, não há PUT
// e changed=false.
func TestEnsureFaceVerifyMode_Idempotent(t *testing.T) {
	const getBody = `{"VerifyWeekPlanCfg":{"WeekPlanCfg":[` +
		`{"week":"Monday","verifyMode":"faceOrFpOrCardOrPw"},` +
		`{"week":"Tuesday","verifyMode":"faceOrFpOrCardOrPw"}` +
		`]}}`

	var putCalled bool
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putCalled = true
		}
		w.Write([]byte(getBody)) //nolint:errcheck
	})
	defer srv.Close()

	changed, err := hikvision.NewWithHTTPClient(cfg, srv.Client()).EnsureFaceVerifyMode(context.Background())
	if err != nil {
		t.Fatalf("EnsureFaceVerifyMode: %v", err)
	}
	if changed {
		t.Error("changed deveria ser false (já está no modo de face)")
	}
	if putCalled {
		t.Error("PUT não deveria ter sido chamado (idempotente)")
	}
}

// TestClockDrift cobre offset presente (drift confiável) e ausente (não mede — não
// inventa fuso).
func TestClockDrift(t *testing.T) {
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		localTime string
		wantOK    bool
		wantDrift time.Duration
	}{
		{"em sincronia (offset)", "2026-06-22T09:00:00-03:00", true, 0},            // 12:00Z
		{"device atrasado 1h", "2026-06-22T08:00:00-03:00", true, time.Hour},       // 11:00Z → now-dev=+1h
		{"device adiantado 1h", "2026-06-22T10:00:00-03:00", true, -time.Hour},     // 13:00Z → now-dev=-1h
		{"sem offset → não mede", "2026-06-22T12:00:00", false, 0},
		{"vazio → não mede", "", false, 0},
		{"lixo → não mede", "not-a-date", false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, drift, ok := hikvision.ClockDrift(tc.localTime, now)
			if ok != tc.wantOK {
				t.Fatalf("ok: got %v want %v", ok, tc.wantOK)
			}
			if ok && drift != tc.wantDrift {
				t.Errorf("drift: got %v want %v", drift, tc.wantDrift)
			}
		})
	}
}
