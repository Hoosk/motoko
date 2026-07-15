# Plan MCP (Model Context Protocol) en Motoko

## Objetivo
Implementar el protocolo MCP 2025-11-25 en Motoko como Host/Client (conectarse a servidores MCP externos y usar sus tools en el Registry existente).

## Rol
- **Client (Host)** — Motoko se conecta a servidores MCP, no los expone como server.
- **Transports:** stdio (completado) → Streamable HTTP (pendiente).
- **Features core:** tools (completado), resources/prompts/sampling/roots (pendientes).

## Config
- `config.json` (global) + `.agents/mcp.json` (workspace) con merge por nombre.
- Protocol version: `"2025-11-25"`.
- Tools remotas con prefijo `mcp_<serverSlug>_<toolName>`.

## Estado Actual (15 Jul 2026)

### Completado (Fase 1 - stdio + tools)

| Archivo | Propósito |
|---|---|
| `internal/mcp/protocol.go` | Tipos JSON-RPC (RPCEnvelope, RequestID, Tool, InitializeResult, CallToolResult, etc.), constantes protocolo y códigos error. |
| `internal/mcp/transport.go` | Interfaz Transport, helpers EncodeMessage/DecodeMessage. |
| `internal/mcp/stdio.go` | StdioTransport: exec.Command, pipes stdin/stdout, framing newline-delimitado, Close con grace period+KILL. |
| `internal/mcp/client.go` | Cliente MCP: Start/Close, Initialize (handshake + initialized notification), Ping, ListTools (cursor+paginate), CallTool, Cancel, request/response map con canales, dispatcher de respuestas/notificaciones, server→client requests. |
| `internal/mcp/remote_tool.go` | RemoteTool (ToolAdapter): invoca tools/call, renderCallResult, isReadOnly. lookupServer en Manager. |
| `internal/mcp/manager.go` | Manager: Start/Stop/Servers, startOne/runServer, refreshTools (lista→registra/desregistra), handleNotification (tools/list_changed), unregisterServerTools. ToolRegistrar, defaultClientInfo/Capabilities, ToolPrefix+slugify. |
| `internal/mcp/errors.go` | ErrTransportClosed, ErrRequestCancelled, ProtocolError. |
| `internal/tools/mcp_tool.go` | MCPRemoteTool bridge (ToolAdapter → tools.Tool), DynamicSpec, IsReadOnly, ServerName, OriginalName. |
| `internal/tools/tools.go` | Unregister(name string) bool. |
| `internal/config/config.go` | MCPServerConfig (Name/Transport/Command/Args/Env/URL/Headers/Disabled), NormalizeTransport, EnvSlice, mergeMCPServers, LoadMCPFile, Merge extendido. Carga de `.agents/mcp.json`. |
| `internal/app/runtime.go` | Campo mcpMgr, construcción en NewRuntime, Start/Stop, mcpServerConfigs helper. |

**Tests:** protocol_test.go, client_test.go, manager_test.go, stdio_test.go, config/mcp_test.go, tools/mcp_tool_test.go — todos pasando con `go test ./...`.

### Próximos Pasos

#### Fase 2 — Streamable HTTP Transport
1. Implementar `internal/mcp/http.go` con `HTTPTransport`:
   - Conexión HTTP POST con SSE para respuestas streaming.
   - Sesión vía `Mcp-Session-Id` header.
   - Timeout, retry, reconnection.
2. Tests con servidor HTTP mock.

#### Fase 3 — Resources & Prompts
1. Recursos: `resources/list`, `resources/read`, `resources/subscribe` + notificaciones.
2. Prompts: `prompts/list`, `prompts/get`.
3. ResourceTemplate, Annotated, TextResourceContents, BlobResourceContents.
4. Manager: refreshResources, refreshPrompts.

#### Fase 4 — Sampling & Roots
1. Sampling: server→client `sampling/createMessage` request.
2. Roots: `roots/list` notify, client capabilities.
3. Elicitation: `elicitation/hint`, `elicitation/extract`.

#### Fase 5 — Polish & Edge Cases
1. Reconnection strategy (exponential backoff).
2. Graceful degradation si server no soporta ciertas capabilities.
3. Logging structured de operaciones MCP.
4. Rate limiting / concurrent request limiting.
5. Autenticación (Bearer token, headers custom).
