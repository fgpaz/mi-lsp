package daemon

import "github.com/fgpaz/mi-lsp/internal/service"

// q-v1 session memory is owned by the daemon process and evicts idle sessions
// according to the contract's bounded LRU policy.
func newQSessionState() *service.SessionState { return service.NewSessionState() }
