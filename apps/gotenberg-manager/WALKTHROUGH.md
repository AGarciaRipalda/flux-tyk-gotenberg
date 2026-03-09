# Gotenberg Manager — Walkthrough

## ¿Qué se ha construido?

Una **aplicación Go completa** que actúa como panel de control para tu pipeline Gotenberg + Tyk. Incluye:

- **Health monitoring** de Gotenberg en tiempo real
- **Gestión de clientes** con CRUD completo
- **Seguridad** con API keys, tokens de admin, y login con email+password (bcrypt)
- **Tracking de uso** por cliente (diario/mensual/total)
- **Dashboard web de admin** con tema oscuro premium
- **Portal de clientes** para generar PDFs sin curl, ver quota y suscripción
- **Deploy listo** para Docker Compose y Kubernetes (Flux)

---

## Arquitectura

```mermaid
graph TB
    subgraph "Gotenberg Manager :9090"
        MAIN[main.go] --> CFG[Config]
        MAIN --> DB[(PostgreSQL)]
        MAIN --> SVC[Services]
        MAIN --> HDLR[Handlers]
        MAIN --> MW[Middleware]
        
        SVC --> CS[ClientService]
        SVC --> HS[HealthService]
        SVC --> US[UsageService]
        
        HDLR --> API[API Handler]
        HDLR --> DASH[Dashboard Handler]
        HDLR --> HH[Health Handler]
        HDLR --> PORTAL[Portal Handler]
        
        MW --> AUTH[Admin Auth]
        MW --> CAUTH[Client Session Auth]
        MW --> LOG[Request Logger]
    end

    CS -->|Admin API| TYK[Tyk Gateway :8080]
    HS -->|GET /health| GOT[Gotenberg :3000]
    PORTAL -->|PDF proxy| GOT
    TYK --> GOT
    
    style MAIN fill:#7c5cfc,color:#fff
    style DB fill:#5e9cff,color:#fff
    style TYK fill:#34d399,color:#000
    style GOT fill:#fbbf24,color:#000
    style PORTAL fill:#f59e0b,color:#000
```

---

## Estructura de ficheros

```
apps/gotenberg-manager/
├── cmd/server/main.go           ← Entrypoint
├── internal/
│   ├── config/config.go         ← Variables de entorno
│   ├── database/postgres.go     ← Conexión PG + migraciones
│   ├── models/models.go         ← Structs, DTOs y View Models
│   ├── services/
│   │   ├── client.go            ← CRUD + API keys + Tyk + Auth (bcrypt)
│   │   ├── health.go            ← Polling Gotenberg /health
│   │   └── usage.go             ← Contadores + stats
│   ├── handlers/
│   │   ├── api.go               ← REST endpoints (admin)
│   │   ├── dashboard.go         ← Dashboard HTML (admin)
│   │   ├── health.go            ← GET /health
│   │   └── portal.go            ← ★ Portal de clientes (login, PDF, quota)
│   ├── middleware/
│   │   ├── auth.go              ← Bearer token admin
│   │   ├── client_auth.go       ← ★ Sesiones HMAC para portal
│   │   └── logger.go            ← Logging de peticiones
│   └── tyk/client.go            ← Wrapper de Tyk Admin API
├── migrations/
│   ├── 001_init.sql             ← Schema inicial
│   └── 002_client_portal.sql    ← ★ Añade password_hash
├── web/
│   ├── templates/
│   │   ├── layout.html          ← Layout admin (sidebar)
│   │   ├── dashboard.html       ← Admin dashboard
│   │   ├── clients.html         ← Admin lista de clientes
│   │   ├── client_detail.html   ← Admin detalle de cliente
│   │   ├── client_form.html     ← Admin formulario (+ password)
│   │   ├── health_page.html     ← Admin health
│   │   ├── portal_layout.html   ← ★ Layout portal (top navbar)
│   │   ├── portal_login.html    ← ★ Login email+password
│   │   ├── portal_dashboard.html← ★ Dashboard cliente + quota
│   │   ├── portal_generate.html ← ★ Generación PDF (3 modos)
│   │   └── portal_subscription.html ← ★ Plan y comparativa
│   └── static/style.css         ← CSS dark theme + portal styles
├── tyk-apis/gotenberg-api.json  ← API def para Docker
├── k8s/                         ← Manifiestos Kubernetes
├── Dockerfile                   ← Multi-stage build
├── docker-compose.yml           ← Stack completo local
├── go.mod / go.sum
└── .gitignore
```

---

## Componentes en detalle

### 1. `cmd/server/main.go` — Entrypoint

Orquesta todo el arranque:

1. Carga configuración desde variables de entorno
2. Conecta a PostgreSQL y ejecuta migraciones automáticas
3. Inicializa el cliente Tyk (para crear/eliminar API keys)
4. Arranca los 3 servicios (clients, health, usage)
5. Lanza el **health checker en background** como goroutine
6. Configura el router `chi` con todas las rutas
7. Arranca el servidor HTTP con graceful shutdown

> [!TIP]
> El health checker es una goroutine que se lanza con `healthSvc.Start(ctx)` y se cancela automáticamente cuando el servidor hace shutdown.

### 2. `internal/config/config.go` — Configuración

Lee **todas** las configuraciones desde variables de entorno con valores por defecto:

| Variable | Default | Descripción |
|---|---|---|
| `PORT` | `9090` | Puerto del servidor |
| `DATABASE_URL` | `postgres://...localhost...` | Conexión PostgreSQL |
| `GOTENBERG_URL` | `http://localhost:3000` | URL de Gotenberg |
| `TYK_URL` | `http://localhost:8080` | URL de Tyk Gateway |
| `TYK_ADMIN_KEY` | `foo` | Secret de admin de Tyk |
| `ADMIN_TOKEN` | `admin-secret` | Token para la API REST |
| `SESSION_SECRET` | `default-session-secret-change-me` | Secreto HMAC para firmar cookies del portal |
| `HEALTH_CHECK_INTERVAL` | `30` | Segundos entre health checks |

### 3. `internal/database/postgres.go` — Base de datos

- Crea un **connection pool** usando `pgxpool` (el driver PostgreSQL más rápido para Go)
- `RunMigrations()` lee archivos `.sql` de la carpeta `migrations/`, los ejecuta en orden, y los registra en una tabla `schema_migrations` para no re-ejecutarlos

### 4. `internal/models/models.go` — Modelos

Structs para las 3 entidades principales:

| Entidad | Campos clave |
|---|---|
| `Client` | id, name, email, api_key, tyk_key_id, password_hash, plan, monthly_limit, is_active |
| `UsageRecord` | client_id, endpoint, status_code, response_time_ms |
| `HealthCheck` | service, status, response_time_ms, details |

Incluye **DTOs** para requests/responses, **View Models** para el portal (`PortalDashboardData`, `PortalGenerateData`, `PortalSubscriptionData`) y **PlanLimits** predefinidos:
- `free` → 100/mes
- `starter` → 1,000/mes
- `pro` → 10,000/mes
- `enterprise` → 100,000/mes

### 5. `internal/services/client.go` — Servicio de Clientes

- **Create**: genera API key (`gm_` + 64 hex chars), hashea password con bcrypt si se proporciona, intenta crear key en Tyk (no falla si Tyk no está disponible), inserta en DB
- **List / GetByID / GetByAPIKey**: consultas con todos los campos incluyendo `password_hash`
- **Update**: actualiza nombre, email, plan, límite, estado activo
- **Delete**: elimina la key de Tyk + borra de la DB (con CASCADE en usage_records)
- **RotateKey**: genera nueva API key manteniendo el mismo cliente
- **Authenticate**: valida email + password con `bcrypt.CompareHashAndPassword`
- **SetPassword**: establece o actualiza la contraseña de un cliente

### 6. `internal/services/health.go` — Health Monitor

- Goroutine que hace `GET gotenberg:3000/health` cada N segundos
- Guarda el resultado en DB (`health_checks` table) y en memoria (mutex-protected)
- Clasifica como: `healthy` (200 OK), `degraded` (otro código), `unhealthy` (error de conexión)
- `GetFullHealth()` consolida: estado app + estado Gotenberg + estado DB

### 7. `internal/services/usage.go` — Tracking de Uso

- `Record()`: inserta un registro por cada petición PDF
- `GetClientStats()`: contador hoy / este mes / total para un cliente
- `GetSummary()`: resumen global + top 5 clientes por uso mensual
- `CheckLimit()`: verifica si un cliente ha excedido su límite mensual

### 8. `internal/tyk/client.go` — Cliente Tyk

Wrapper HTTP para la Admin API de Tyk:
- `CreateKey(rate, per, quotaMax)` → `POST /tyk/keys/create` con el header `x-tyk-authorization`
- `DeleteKey(keyID)` → `DELETE /tyk/keys/{keyID}`

### 9. `internal/handlers/` — Controladores HTTP

**`api.go`** — REST API (protegida por Bearer token):

| Método | Ruta | Acción |
|---|---|---|
| GET | `/api/clients` | Listar clientes |
| POST | `/api/clients` | Crear cliente |
| GET | `/api/clients/{id}` | Detalle |
| PUT | `/api/clients/{id}` | Actualizar |
| DELETE | `/api/clients/{id}` | Eliminar |
| POST | `/api/clients/{id}/rotate-key` | Rotar API key |
| GET | `/api/clients/{id}/usage` | Stats de uso |
| GET | `/api/usage/summary` | Resumen global |

**`dashboard.go`** — Dashboard web (páginas HTML admin):

| Ruta | Página |
|---|---|
| `/dashboard` | Overview con stats, health, top clients |
| `/dashboard/clients` | Tabla de todos los clientes |
| `/dashboard/clients/new` | Formulario de creación (con campo password) |
| `/dashboard/clients/{id}` | Detalle + uso + actividad |

**`portal.go`** — ★ Portal de clientes (sesión por cookie):

| Ruta | Método | Página |
|---|---|---|
| `/portal/login` | GET | Formulario de login |
| `/portal/login` | POST | Autenticar email+password |
| `/portal` | GET | Dashboard del cliente (quota, stats, actividad) |
| `/portal/generate` | GET | Formulario de generación PDF (3 modos) |
| `/portal/generate` | POST | Generar PDF (proxy a Gotenberg + registro uso) |
| `/portal/subscription` | GET | Plan actual y comparativa de planes |
| `/portal/logout` | POST | Cerrar sesión |

> [!NOTE]
> El portal envía las peticiones de PDF **directamente a Gotenberg** (sin pasar por Tyk), lo que permite control total de quota a nivel de aplicación y registro automático del uso.

**`health.go`** — `GET /health` (público, JSON)

### 10. `internal/middleware/` — Middleware

- **`auth.go`**: valida `Authorization: Bearer <token>` contra `ADMIN_TOKEN` configurado
- **`client_auth.go`**: valida cookie de sesión HMAC-firmada para el portal de clientes. Inyecta el `clientID` en el contexto de la request
- **`logger.go`**: registra `METHOD PATH STATUS DURATION` para cada petición

### 11. `web/templates/` — Templates HTML

**Admin** — 6 templates con layout de sidebar:
- **layout.html**: sidebar con navegación + estructura base
- **dashboard.html**: cards de stats, banner de health, tablas de top clients y clientes recientes
- **clients.html**: tabla completa con badges de plan y estados
- **client_detail.html**: stats de uso, barra de progreso mensual, detalles de API key, actividad reciente
- **client_form.html**: formulario con selector de plan + campo password para portal
- **health_page.html**: estado del sistema

**Portal de clientes** — 5 templates con layout de top navbar:
- **portal_layout.html**: navbar superior con Dashboard / Generate PDF / Subscription + nombre del cliente + logout
- **portal_login.html**: página de login centrada con email + password
- **portal_dashboard.html**: barra de progreso de quota, stats cards (hoy/mes/total/límite), tabla de conversiones recientes
- **portal_generate.html**: interfaz con 3 pestañas (URL → PDF, HTML → PDF, Archivo → PDF) con drag & drop
- **portal_subscription.html**: card del plan actual con stats, barra de progreso, y grid comparativo de 4 planes

### 12. `web/static/style.css` — Diseño

Tema oscuro premium (~1300 líneas) con:
- Paleta `#0f1117` / `#7c5cfc` (púrpura) / `#5e9cff` (azul)
- Glassmorphism en cards con `box-shadow` y bordes sutiles
- Badges de color por plan (free/starter/pro/enterprise)
- Barra de progreso con gradiente para uso mensual (+ estados warning/danger)
- Animación `pulse` en el indicador de health
- Layout responsive (sidebar se colapsa en móvil)
- **Portal**: login card con fondo radial gradient, navbar fija, tabs para modos de conversión, zona de drag & drop para archivos, grid de planes con highlight del activo

---

## Guía de Instalación y Configuración

### Requisitos previos

- Docker + Docker Compose
- (Opcional) Go 1.22+ para desarrollo local
- (Opcional) Kubernetes + Flux para producción

### Opción 1: Docker Compose (recomendada para desarrollo)

```bash
# 1. Ir al directorio de la app
cd apps/gotenberg-manager

# 2. Levantar todo el stack (5 servicios)
docker compose up --build -d

# 3. Verificar que todo funciona
docker compose ps

# 4. Abrir el dashboard
open http://localhost:9090/dashboard

# 5. Verificar health
curl http://localhost:9090/health
```

> [!IMPORTANT]
> El `docker-compose.yml` levanta: **PostgreSQL** (:5432), **Gotenberg** (:3000), **Tyk Gateway** (:8080), **Redis** (:6379), y **Gotenberg Manager** (:9090). Todo auto-configurado.

### Opción 2: Desarrollo local (sin Docker para la app)

```bash
# 1. Tener PostgreSQL corriendo (o usar el de docker-compose)
docker compose up postgres gotenberg -d

# 2. Configurar variables de entorno
export DATABASE_URL="postgres://gotenberg_manager:gotenberg_manager@localhost:5432/gotenberg_manager?sslmode=disable"
export GOTENBERG_URL="http://localhost:3000"
export ADMIN_TOKEN="mi-token-secreto"

# 3. Compilar y ejecutar
cd apps/gotenberg-manager
go run ./cmd/server/
```

### Opción 3: Kubernetes con Flux

```bash
# 1. Construir y pushear la imagen Docker
docker build -t tu-registry/gotenberg-manager:latest apps/gotenberg-manager/
docker push tu-registry/gotenberg-manager:latest

# 2. Actualizar la imagen en k8s/deployment.yaml
# Cambiar: image: gotenberg-manager:latest
# A:       image: tu-registry/gotenberg-manager:latest

# 3. El fichero clusters/my-cluster/gotenberg-manager-sync.yaml
#    ya está creado y Flux lo sincronizará automáticamente

# 4. Verificar el despliegue
kubectl get pods -n gotenberg-manager

# 5. Abrir el port-forward
kubectl port-forward svc/gotenberg-manager 9090:9090 -n gotenberg-manager

# 6. Acceder al dashboard
open http://localhost:9090/dashboard
```

### Usar la API REST

```bash
# Crear un cliente con password para el portal (con Admin Token)
curl -X POST http://localhost:9090/api/clients \
  -H "Authorization: Bearer admin-secret" \
  -H "Content-Type: application/json" \
  -d '{"name":"Acme Corp","email":"admin@acme.com","password":"secreto123","plan":"pro"}'

### Usar el Servicio como Cliente (vía API Gateway Tyk)

Los clientes que quieran programar integraciones (en lugar de usar el Portal visual) pueden atacar a Tyk directamente:

```bash
# 1. El administrador crea el cliente en Gotenberg Manager (Dashboard o API)
# La respuesta de la API o la vista de detalle en el Dashboard mostrará un "Tyk Key ID"
# Proporciona esta llave al cliente. No uses el "API Key" interno (gm_...) para Tyk.

# 2. El cliente usa su "Tyk Key ID" para convertir URLs a PDF vía Tyk
curl -X POST http://localhost:8080/pdf/forms/chromium/convert/url \
  -H "Authorization: YOUR_TYK_KEY_HERE" \
  -F url="https://example.com" \
  -o output.pdf

# Listar clientes (como Admin)
curl -H "Authorization: Bearer admin-secret" \
  http://localhost:9090/api/clients

# Ver uso de un cliente (como Admin)
curl -H "Authorization: Bearer admin-secret" \
  http://localhost:9090/api/clients/{ID}/usage

# Resumen global (como Admin)
curl -H "Authorization: Bearer admin-secret" \
  http://localhost:9090/api/usage/summary

# Rotar API key (como Admin)
curl -X POST -H "Authorization: Bearer admin-secret" \
  http://localhost:9090/api/clients/{ID}/rotate-key
```

### Usar el Portal de Clientes

El portal permite a los clientes generar PDFs directamente desde el navegador sin necesidad de `curl`:

```bash
# 1. Crear un cliente con password desde el admin
#    Dashboard → Clients → New Client → rellenar con password
#    O vía API como se muestra arriba

# 2. Acceder al portal
open http://localhost:9090/portal/login

# 3. Iniciar sesión con email + password del cliente
# 4. Desde el dashboard del portal:
#    - Ver quota mensual (barra de progreso)
#    - Generar PDFs (URL / HTML / archivo)
#    - Consultar plan y comparar opciones
```

> [!TIP]
> El portal genera PDFs directamente contra Gotenberg (sin Tyk), registrando automáticamente cada conversión en la base de datos y controlando la quota del cliente.

---

## Validación

| Check | Resultado |
|---|---|
| `go mod tidy` | Dependencias resueltas |
| `go build ./cmd/server/` | Compila sin errores |
| Estructura de proyecto | 25+ ficheros organizados por dominio |
| Templates HTML admin | 6 templates con layout sidebar |
| Templates HTML portal | 5 templates con layout navbar |
| Portal login | ✅ Email + password con bcrypt |
| Portal PDF generation | ✅ URL/HTML/File → PDF (proxy directo a Gotenberg) |
| Portal quota tracking | ✅ Se actualiza tras cada conversión |
| Dockerfile | Multi-stage, non-root, healthcheck |
| docker-compose.yml | 5 servicios con healthcheck de PG |
| K8s manifests | Namespace + ConfigMap + Secret + Deployments + Services + PVC |
| Flux sync | Kustomization con dependsOn de Tyk y Gotenberg |
