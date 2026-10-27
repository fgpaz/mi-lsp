# Local and shared daemon

The default daemon uses the local profile endpoint and does not call `botsd`. Set `MI_LSP_ELASTIC=1` in both daemon and client environments to opt into the shared node-scoped pool. This mode requires `botsd` on PATH with a configured node identity and a reachable coordinator source. Restart the daemon when changing modes. Missing requirements are reported as errors; unset the variable to use the local daemon.

Doctor reports an absent optional daemon as skipped; communication failures with a running or explicitly selected daemon remain visible. Worker and backend availability are independent of the shared pool.
