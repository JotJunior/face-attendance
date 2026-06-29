package domain

import "testing"

// TestAttendanceEvent_IsAuthorized cobre a regra usada pelo motor de fluxo para
// ramificar a decisão: o campo Authorized (computado pelo handler via accessGranted)
// tem precedência; na ausência dele, recai no attendanceStatus == "authorized".
func TestAttendanceEvent_IsAuthorized(t *testing.T) {
	bp := func(b bool) *bool { return &b }
	sp := func(s string) *string { return &s }

	cases := []struct {
		name       string
		authorized *bool
		status     *string
		want       bool
	}{
		// Caso real do bug: device sinaliza sucesso via major=5/sub=75; attendanceStatus
		// vem vazio (nil), mas o handler seta Authorized=true → decisão "valid".
		{"authorized true sem status", bp(true), nil, true},
		{"authorized false sem status", bp(false), nil, false},
		// Authorized tem precedência sobre attendanceStatus.
		{"authorized false sobrepõe status authorized", bp(false), sp("authorized"), false},
		{"authorized true sobrepõe status vazio", bp(true), sp(""), true},
		// Fallback (Authorized nil): usa attendanceStatus.
		{"fallback status authorized", nil, sp("authorized"), true},
		{"fallback status denied", nil, sp("denied"), false},
		{"fallback status nil", nil, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &AttendanceEvent{Authorized: tc.authorized, AttendanceStatus: tc.status}
			if got := e.IsAuthorized(); got != tc.want {
				t.Errorf("IsAuthorized() = %v, want %v", got, tc.want)
			}
		})
	}
}
