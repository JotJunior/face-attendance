package hikvision

import "errors"

// ErrKeyMissing is returned when ISAPI_CRED_KEY is absent or decryption fails.
// All ISAPI action handlers must map this error to HTTP 503 with an orientative message.
// CHK007: centralised key check prevents scattered nil-cipher panics across handlers.
var ErrKeyMissing = errors.New("hikvision: ISAPI_CRED_KEY ausente ou descriptografia falhou (503)")

// ErrNotImplemented is returned by stub methods pending empirical ISAPI validation.
var ErrNotImplemented = errors.New("hikvision: método não implementado — verificar empiricamente")

// ErrUnknownCommand is returned by ControlDoor when the command is not in commandToISAPICmd.
var ErrUnknownCommand = errors.New("hikvision: comando de porta desconhecido")
