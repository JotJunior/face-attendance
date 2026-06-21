package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/hikvision"
)

// fakeClient conta as chamadas ISAPI e pode injetar erro no UploadFace.
type fakeClient struct {
	upserts   int
	uploads   int
	uploadErr error
}

func (f *fakeClient) UpsertUser(context.Context, string, string) error { f.upserts++; return nil }
func (f *fakeClient) UploadFace(context.Context, string, string) error { f.uploads++; return f.uploadErr }
func (f *fakeClient) ConfigureWebhook(context.Context, string) error    { return nil }

func newTestProcessor(r ConnResolver) *Processor {
	// outcomeRepo nil → saveOutcome é no-op (sem DB).
	return &Processor{resolver: r, maxRetryAttempts: 3, initialBackoffMs: 1}
}

const testCPF = "12345678901"

func testMsg() domain.ProcessingMessage {
	return domain.ProcessingMessage{FederalDocument: testCPF, Name: "Fulano", URLSelfie: "http://x/s.jpg"}
}

// Multi-device: o membro deve ser provisionado em TODOS os devices ativos.
func TestProcessMessage_EnrollsAllDevices(t *testing.T) {
	c1, c2 := &fakeClient{}, &fakeClient{}
	p := newTestProcessor(StaticResolver{Targets: []Target{{Client: c1, DeviceID: 1}, {Client: c2, DeviceID: 2}}})

	if err := p.processMessage(context.Background(), testMsg()); err != nil {
		t.Fatalf("processMessage: %v", err)
	}
	if c1.upserts != 1 || c2.upserts != 1 {
		t.Errorf("UpsertUser deveria ser chamado nos 2 devices: c1=%d c2=%d", c1.upserts, c2.upserts)
	}
	if c1.uploads != 1 || c2.uploads != 1 {
		t.Errorf("UploadFace deveria ser chamado nos 2 devices: c1=%d c2=%d", c1.uploads, c2.uploads)
	}
}

// Falha NÃO-retriável num device (ex.: selfie sem rosto) não bloqueia os outros
// nem re-enfileira a mensagem.
func TestProcessMessage_NonRetriableOneDevice_StillEnrollsOthers(t *testing.T) {
	c1 := &fakeClient{uploadErr: &hikvision.NonRetriableError{Op: "UploadFace", Status: 400}}
	c2 := &fakeClient{}
	p := newTestProcessor(StaticResolver{Targets: []Target{{Client: c1, DeviceID: 1}, {Client: c2, DeviceID: 2}}})

	if err := p.processMessage(context.Background(), testMsg()); err != nil {
		t.Errorf("falha não-retriável num device não deve re-enfileirar; got %v", err)
	}
	if c2.upserts != 1 || c2.uploads != 1 {
		t.Errorf("c2 deveria ser enrolado mesmo com c1 falhando: upserts=%d uploads=%d", c2.upserts, c2.uploads)
	}
}

// Falha RETRIÁVEL propaga para re-enfileirar (re-provisionar é idempotente).
func TestProcessMessage_RetriableRequeues(t *testing.T) {
	c1 := &fakeClient{uploadErr: errors.New("timeout transitório")}
	p := newTestProcessor(StaticResolver{Targets: []Target{{Client: c1, DeviceID: 1}}})

	err := p.processMessage(context.Background(), testMsg())
	if err == nil || hikvision.IsNonRetriable(err) {
		t.Errorf("erro retriável deveria propagar p/ re-enfileirar; got %v", err)
	}
}

// Sem devices-alvo ativos → erro (mensagem re-enfileira até haver device).
func TestProcessMessage_NoTargets(t *testing.T) {
	p := newTestProcessor(StaticResolver{Targets: nil})
	if err := p.processMessage(context.Background(), testMsg()); err == nil {
		t.Error("sem devices-alvo deveria retornar erro")
	}
}
