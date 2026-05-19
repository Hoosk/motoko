# Motoko Backlog

## Objetivo
Llevar Motoko desde un prototipo visual a un coding agent terminal usable, con una experiencia cercana a opencode pero apoyada en una arquitectura propia basada en Tachikomas y contexto semántico persistente.

## Principios
- Priorizar capacidad real antes que estética adicional.
- Separar claramente UI, runtime del agente, tools y contexto en background.
- Mantener permisos explícitos para shell y futuras ediciones.
- Usar Tachikomas para reducir búsquedas ciegas y consumo de tokens.
- Empezar por un sistema operativo y luego refinar streaming, contexto y edición avanzada.

## Hecho

### Runtime y sesión
- Creado `internal/app` como runtime principal de sesión.
- Modos `plan` y `build` implementados.
- Modos de entrada `chat` y `shell` implementados.
- Parser de slash commands implementado.
- Shell directo operativo con política de permisos y aprobaciones.
- `!shell` y ejecución directa en modo shell funcionando.
- Añadido flag de debug para el agente.

### Tools mínimas
- Implementadas tools reales:
- `read`
- `glob`
- `grep`
- `bash`
- `patch`
- Registro de tools operativo y visible desde la UI.

### Providers y modelos
- Soporte multi-provider implementado:
- `openai`
- `anthropic`
- `gemini`
- Persistencia de providers en config local.
- `/provider add`, `/provider list`, `/provider use`, `/provider remove` implementados.
- `/models` implementado.
- Listado remoto de modelos implementado.
- Cache de modelos persistida para autocompletado.
- Autocompletado de `/models` corregido para usar modelos cacheados.

### Agente
- Loop base del agente implementado: prompt -> modelo -> tool -> resultado -> respuesta.
- Tool calling mínimo operativo con máximo de iteraciones.
- Acumulación de uso/tokens integrada.
- Contexto del sistema enviado al provider.
- Ejecución async desde la TUI conectada al runtime.

### UI/TUI
- UI simplificada a timeline + input + footer + popups.
- Popup de tools con `Ctrl+T` implementado.
- Popup de provider implementado.
- Thinking state y animación básica implementados.
- Input limpiado al enviar.
- Scroll del timeline corregido.
- Input rehecho con altura inicial de 3 líneas y crecimiento dinámico.
- Prompt visual del input reducido a un único marcador centrado.
- Sugerencias aceptables con `Tab`, `Right` y `Enter`.
- `internal/ui/model.go` refactorizado en varios archivos más pequeños.
- Tests añadidos para lógica refactorizada de UI.

### Tachikomas y contexto
- `WorkspaceTachikoma` implementado.
- `GitTachikoma` implementado.
- `CodeTachikoma` implementado.
- Estado de Tachikomas expuesto a la UI y al contexto del agente.

### Contexto semántico
- Nuevo paquete `internal/semantic` implementado.
- Índice semántico con `go-tree-sitter` añadido.
- Extracción de símbolos para `go`, `js`, `jsx`, `ts`, `tsx`.
- Resumen semántico del repo generado por snapshot.
- Detección de archivos cambiados integrada.
- Ranking heurístico de archivos relevantes por prompt implementado.
- Archivos relevantes inyectados al contexto del agente antes de cada ejecución.
- Selección automática de fragmentos cortos por símbolo/archivo relevante implementada.
- Snippets estructurados inyectados en el contexto inicial del agente.

### Tests y validación
- Tests para traducción de resultados del agente a timeline.
- Tests para altura dinámica del input.
- Tests para autocompletado de `/models`.
- Tests para índice semántico y selección de archivos relevantes.
- `go test ./...` pasando.

## En curso

### Streaming y respuesta progresiva
- Renderizado progresivo real de texto/código del agente.
- Exposición incremental de pasos del agente en vez de resultado final por lote.

### Contexto semántico de siguiente nivel
- Añadir un grafo de relaciones:
- imports
- exports
- referencias básicas entre símbolos
- Afinar el ranking con historial conversacional y no solo con el prompt actual.

## Siguientes pasos

### Fase 1 - Contexto semántico útil de verdad
- Priorizar archivos cambiados cuando el usuario pida review o feedback del código.
- Incluir top-level symbols y líneas aproximadas en el contexto de trabajo.

### Fase 2 - Grafo de código
- Construir relaciones simples entre archivos:
- imports
- llamadas conocidas
- referencias de símbolos exportados
- Añadir una heurística de “archivos vecinos” para ampliar contexto sin búsquedas ciegas.

### Fase 3 - Mejor selección por intención
- Detectar prompts de review, bugfix, explicación o navegación.
- Ajustar la estrategia de selección según intención.
- Usar señales de Tachikoma más específicas para cada tipo de tarea.

### Fase 4 - Streaming del agente
- Soportar respuesta progresiva del provider si está disponible.
- Reflejar texto y bloques de código mientras se generan.
- Mostrar pasos del agente de forma incremental sin esperar al final.

### Fase 5 - Edición y validación
- Hacer que el agente edite archivos por parches estructurados de forma más autónoma.
- Añadir validación posterior automática con tests/build cuando aplique.
- Mejorar el resumen final de cambios y validaciones.

## Riesgos / deuda actual
- El índice semántico es heurístico; aún no usa un grafo fuerte de referencias.
- El agente sigue apoyándose mucho en `read/glob/grep` tras el primer contexto.
- El resumen semántico todavía no inyecta snippets concretos automáticamente.
- La UI no tiene streaming real del provider todavía.

## Decisiones activas
- UX principal: slash commands + shell directo.
- Providers soportados: `openai`, `anthropic`, `gemini`.
- Política inicial: comandos sensibles con confirmación.
- Estrategia: contexto semántico ligero primero, grafo semántico después.
- Tree-sitter es la base del contexto estructurado del repo.
