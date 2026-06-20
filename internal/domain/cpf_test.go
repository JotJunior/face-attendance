package domain_test

import (
	"testing"

	"github.com/jotjunior/face-attendance/internal/domain"
)

func TestFormatCPF(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"digits only", "12345678901", "123.456.789-01", false},
		{"already masked", "123.456.789-01", "123.456.789-01", false},
		{"too short", "1234567890", "", true},
		{"too long", "123456789012", "", true},
		{"empty", "", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.FormatCPF(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("FormatCPF(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
				return
			}
			if got != tc.want {
				t.Errorf("FormatCPF(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseCPF(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"masked format", "123.456.789-01", "12345678901", false},
		{"already digits", "12345678901", "12345678901", false},
		{"too short digits", "1234567890", "", true},
		{"empty", "", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.ParseCPF(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("ParseCPF(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
				return
			}
			if got != tc.want {
				t.Errorf("ParseCPF(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateCPF(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid digits", "12345678901", true},
		{"valid masked", "123.456.789-01", true},
		{"too short", "1234567890", false},
		{"too long", "123456789012", false},
		{"empty", "", false},
		{"alpha chars", "abc", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.ValidateCPF(tc.input)
			if got != tc.want {
				t.Errorf("ValidateCPF(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestNormalizeCPF_Roundtrip(t *testing.T) {
	original := "12345678901"

	masked, err := domain.FormatCPF(original)
	if err != nil {
		t.Fatalf("FormatCPF failed: %v", err)
	}

	normalized, err := domain.NormalizeCPF(masked)
	if err != nil {
		t.Fatalf("NormalizeCPF failed: %v", err)
	}

	if normalized != original {
		t.Errorf("roundtrip failed: got %q, want %q", normalized, original)
	}
}

func TestMaskCPFForLog(t *testing.T) {
	tests := []struct {
		name   string
		digits string
		want   string
	}{
		{"valid 11 digits", "12345678901", "***.***.***-01"},
		{"valid digits last 2", "00000000099", "***.***.***-99"},
		{"empty", "", "***.***.***-**"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.MaskCPFForLog(tc.digits)
			if got != tc.want {
				t.Errorf("MaskCPFForLog(%q) = %q, want %q", tc.digits, got, tc.want)
			}
		})
	}
}
