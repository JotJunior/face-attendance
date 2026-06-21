package hikvision

// client_faces.go implements ISAPI face library operations.
// CHK-PROPOSTA-9: the FDLib/clear endpoint has NOT been verified empirically.
// ClearFaces is a STUB returning ErrNotImplemented until empirical verification on a
// real device determines the correct endpoint path.
//
// To resolve: run `PUT /ISAPI/Intelligent/FDLib/FDSearch/Delete?format=json` or
// alternatives against the device and document in hikvision-isapi.md §Clear faces.
// Then replace this stub with the real implementation and update the test.
//
// Ref: spec.md §FR-017, research.md Decision 9, tasks.md §3.5.

import (
	"context"
	"fmt"
)

// ClearFaces removes all face data from the device's FDLib.
// [PROPOSTA — stub]: returns ErrNotImplemented until endpoint verified empirically.
// CHK-PROPOSTA-9: see file-level doc comment.
func (c *Client) ClearFaces(ctx context.Context) error {
	// TODO(tasks §3.5.2): verify empirically and replace stub.
	// Candidate path: PUT /ISAPI/Intelligent/FDLib/FDSearch/Delete?format=json
	// with body {"FDID":"1","faceLibType":"blackFD"} or body {} depending on firmware.
	return fmt.Errorf("%w: endpoint FDLib/clear não verificado — validar empiricamente (CHK-PROPOSTA-9)",
		ErrNotImplemented)
}
