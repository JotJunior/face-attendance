package httphandler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/logging"
)

type resyncFakeFinder struct {
	member *domain.Member
	err    error
}

func (f *resyncFakeFinder) FindByID(context.Context, int64) (*domain.Member, error) {
	return f.member, f.err
}

type resyncFakePublisher struct {
	published []domain.ProcessingMessage
	err       error
}

func (p *resyncFakePublisher) Publish(_ context.Context, msg domain.ProcessingMessage) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, msg)
	return nil
}

func resyncTestMember() *domain.Member {
	sel := "https://example.com/face.jpg"
	return &domain.Member{ID: 42, GobID: 7, FederalDocument: "12345678901", Name: "Fulano", URLSelfie: &sel}
}

func newResyncHandler(finder memberByIDFinder, pub processingPublisher) http.Handler {
	return AdminMemberResyncHandler(AdminResyncConfig{
		MemberFinder: finder,
		Publisher:    pub,
		Logger:       logging.NewWithHandler(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestResync_Success_PublishesExactlyOne(t *testing.T) {
	pub := &resyncFakePublisher{}
	h := newResyncHandler(&resyncFakeFinder{member: resyncTestMember()}, pub)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/admin/api/members/42/resync", nil))

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", rr.Code, rr.Body.String())
	}
	if len(pub.published) != 1 {
		t.Fatalf("publicou %d mensagens, want 1", len(pub.published))
	}
	got := pub.published[0]
	if got.FederalDocument != "12345678901" || got.GobID != 7 || got.URLSelfie == "" || got.Name != "Fulano" {
		t.Errorf("mensagem publicada incorreta: %+v", got)
	}
}

func TestResync_NotFound(t *testing.T) {
	pub := &resyncFakePublisher{}
	h := newResyncHandler(&resyncFakeFinder{member: nil}, pub)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/admin/api/members/99/resync", nil))

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	if len(pub.published) != 0 {
		t.Errorf("não deveria publicar quando o membro não existe")
	}
}

func TestResync_NoSelfie_Unprocessable(t *testing.T) {
	pub := &resyncFakePublisher{}
	m := resyncTestMember()
	m.URLSelfie = nil
	h := newResyncHandler(&resyncFakeFinder{member: m}, pub)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/admin/api/members/42/resync", nil))

	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rr.Code)
	}
	if len(pub.published) != 0 {
		t.Errorf("não deveria publicar sem selfie")
	}
}

func TestResync_MethodNotAllowed(t *testing.T) {
	h := newResyncHandler(&resyncFakeFinder{member: resyncTestMember()}, &resyncFakePublisher{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/api/members/42/resync", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestResync_BadPath_NotFound(t *testing.T) {
	h := newResyncHandler(&resyncFakeFinder{member: resyncTestMember()}, &resyncFakePublisher{})
	for _, p := range []string{
		"/admin/api/members/42",          // sem /resync
		"/admin/api/members/abc/resync",  // id não-numérico
		"/admin/api/members//resync",     // id vazio
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, p, nil))
		if rr.Code != http.StatusNotFound {
			t.Errorf("path %q: status = %d, want 404", p, rr.Code)
		}
	}
}
