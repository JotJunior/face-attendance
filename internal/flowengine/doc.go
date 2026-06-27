// Package flowengine implements the face-flow execution engine.
//
// The engine listens to attendance events from HikVision devices, looks up the
// active flow configured for the device, and traverses the node graph performing
// the actions defined in each node type.
//
// Behaviour guarantees:
//   - Non-blocking: TriggerForDevice returns immediately after spawning a goroutine.
//   - Snapshot isolation: edits to the admin flow do not affect in-progress executions.
//   - Circuit-break + reset: any error or timeout resets execution to the initial state;
//     the flow is not left in a partially-executed state (spec FR-021/FR-022).
//   - Idempotency: a completed event_key is never re-executed (Constitution §II, spec FR-023).
//   - Concurrency bound: a semaphore caps simultaneous goroutines (performance CHK005).
//   - SSRF guard: the https_call node resolves hostnames and rejects RFC-1918 / loopback
//     / link-local targets before issuing any outbound HTTP request (security CHK001).
//   - Secret masking: logs never contain interpolated bodies, auth tokens, or raw CPF digits.
//
// Ref: docs/specs/face-flow/plan.md §3, tasks.md FASE 3.
package flowengine
