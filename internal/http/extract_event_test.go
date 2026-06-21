package httphandler

import (
	"bytes"
	"mime/multipart"
	"net/textproto"
	"testing"
)

type mpPart struct{ name, ctype, content string }

func mkMultipart(t *testing.T, parts ...mpPart) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, p := range parts {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="`+p.name+`"`)
		if p.ctype != "" {
			h.Set("Content-Type", p.ctype)
		}
		pw, err := w.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pw.Write([]byte(p.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return w.FormDataContentType(), buf.Bytes()
}

// Os terminais HikVision variam o NOME da parte JSON conforme o firmware:
// "event_log" (DS-K1T673DWX em .107) ou "AccessControllerEvent" (device em .123),
// e às vezes mandam imagem junto. extractEventJSON deve achar o JSON em todos.
func TestExtractEventJSON_PartNameVariants(t *testing.T) {
	const evt = `{"eventType":"AccessControllerEvent","macAddress":"04:03:12:06:a2:11","ipAddress":"192.168.68.123"}`
	cases := []struct {
		label string
		parts []mpPart
	}{
		{"event_log", []mpPart{{"event_log", "application/json", evt}}},
		{"AccessControllerEvent", []mpPart{{"AccessControllerEvent", "application/json", evt}}},
		{"nome desconhecido sem content-type", []mpPart{{"weird_name", "", evt}}},
		{"imagem antes do json", []mpPart{
			{"Picture", "image/jpeg", "\xff\xd8\xff\x00bin\x00ary"},
			{"AccessControllerEvent", "", evt},
		}},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			ct, body := mkMultipart(t, c.parts...)
			got, err := extractEventJSON(ct, body)
			if err != nil {
				t.Fatalf("extractEventJSON: %v", err)
			}
			if string(bytes.TrimSpace(got)) != evt {
				t.Errorf("got %q\nwant %q", got, evt)
			}
		})
	}
}

func TestExtractEventJSON_RawJSON(t *testing.T) {
	body := []byte(`{"eventType":"x"}`)
	got, err := extractEventJSON("application/json", body)
	if err != nil || string(got) != string(body) {
		t.Errorf("raw json: got %q err %v", got, err)
	}
}

func TestExtractEventJSON_NoJSONPart(t *testing.T) {
	ct, body := mkMultipart(t, mpPart{"Picture", "image/jpeg", "\xff\xd8\xffbinary"})
	if _, err := extractEventJSON(ct, body); err == nil {
		t.Error("esperava erro quando não há parte JSON no multipart")
	}
}
