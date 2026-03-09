# Informe Final: Plataforma GitOps de Generación PDF (Gotenberg + Tyk + Manager)

Este documento resume la investigación, arquitectura, implementación y despliegue del proyecto **Flux-Tyk-Gotenberg**, así como las propuestas estratégicas para la evolución futura de la plataforma.

---

## PARTE 1: Resumen de Investigación, Implementación y Despliegue

La plataforma se ha diseñado para resolver el problema de la generación de documentos PDF a escala, ofreciendo un servicio centralizado, seguro, medible y fácil de operar mediante metodologías GitOps.

### 1.1. Investigación y Arquitectura Base

La solución se divide en tres pilares fundamentales que interactúan entre sí:

1. **El Motor (Gotenberg):** Se seleccionó Gotenberg por ser una API Dockerizada *stateless* que envuelve Chromium y LibreOffice. Permite conversiones de URL, HTML, Markdown y documentos Office a PDF con alto rendimiento. Al ser *stateless*, es ideal para escalar horizontalmente en Kubernetes.
2. **El API Gateway (Tyk):** Se implementó Tyk Gateway apoyado en Redis para proteger Gotenberg. Tyk asume la responsabilidad de la autenticación dura (API Keys), el rate-limiting estricto y el enrutamiento.
3. **El Plano de Control (Gotenberg Manager):** Para dotar al sistema de "inteligencia de negocio", se desarrolló desde cero una aplicación en Go. Este componente llena el vacío entre la infraestructura cruda (Gotenberg/Tyk) y las necesidades de negocio (gestión de clientes, tracking de uso, interfaces visuales).

### 1.2. Implementación de *Gotenberg Manager*

*Gotenberg Manager* es un microservicio monolítico escrito en Go 1.22, diseñado con una arquitectura propia limpia y orientada a dominios:

#### Core Backend
* **Base de Datos (PostgreSQL):** Utiliza `pgxpool` para alto rendimiento. Almacena clientes, hashes de contraseñas, historial de estado (health checks) y cada registro individual de conversión (usage records).
* **Gestión de Sesiones y Seguridad:** 
  * La **API Admin** está protegida por un token Bearer estático (`admin-secret`) validado mediante middleware para todas las rutas `/api/*`.
  * El **Portal de Clientes** está protegido por un middleware de autenticación por cookies firmadas criptográficamente mediante HMAC (`SESSION_SECRET`). Las contraseñas se almacenan usando bcrypt.
* **Orquestación Tyk:** El código se comunica directamente con la Admin API de Tyk. Cuando un admin crea un cliente en la base de datos, el backend automáticamente negocia y aprovisiona una API Key real en Tyk (`tyk_key_id`) con los límites correspondientes al plan elegido.

#### Servicios Background
* **Health Monitor:** Una goroutine verifica asíncronamente el endpoint `/health` de Gotenberg cada 30 segundos, guardando el histórico en base de datos para ofrecer un SLA calculable.

#### Interfaces de Usuario (Frontend HTML/CSS)
Go renderiza nativamente (Server-Side Rendering con `html/template`) dos interfaces distintas, ambas estilizadas con un diseño CSS premium oscuro y *glassmorphism*:
1. **Dashboard de Administrador:** Permite visualizar el estado en tiempo real del cluster, gestionar instancias completas del CRUD de clientes y ver rankings de uso.
2. **Portal de Clientes:** Permite a los usuarios finales hacer login con *email y password*. Una vez dentro, los clientes pueden ver su *Quota* mensual (barra de progreso), las conversiones recientes, y usar una interfaz interactiva de 3 pestañas (URL, HTML, Subida de Archivo) para **generar PDFs directamente desde el navegador** sin necesidad de integraciones API complejas ni uso de `curl`. 
   * **Proxy Nativo en Go:** Cuando un cliente solicita un PDF desde el portal, el backend de Go intercepta la petición, verifica la cuota en base de datos (saltándose a Tyk) y construye nativamente una petición HTTP Multiparte hacia el motor de Gotenberg interno. Al recibir la respuesta, Go realiza un *stream* directo del binario PDF hacia el navegador del cliente en tiempo real, operando con máxima eficiencia sin guardar archivos temporales en disco.

### 1.3. Despliegue y GitOps (Flux CD)

Todo el ciclo de vida del software está gobernado por principios GitOps a través de **Flux CD** sobre Kubernetes:

* **Declarativo:** La configuración completa del gateway, los secretos de la base de datos, los deployments de Gotenberg, Tyk y el Manager están escritos en archivos YAML en el repositorio.
* **Reconciliación Automática:** Flux asegura que el clúster refleje exactamente la rama `main` del repositorio. Si un pod cae o se borra un recurso manualmente, Flux lo restaura en menos de 60 segundos.
* **Health Check Pipeline:** Se ha implementado un `CronJob` nativo de Kubernetes que cada 5 minutos inyecta una petición real a Tyk -> Gotenberg y reporta éxito, asegurando que toda la cadena de red está operativa de extremo a extremo.

---

## PARTE 2: Propuestas de Mejora y Evolución

Ahora que la infraestructura base (Plano de Datos + Plano de Control) es sólida y operativa, el proyecto está en la fase ideal para enfocarse en la experiencia del usuario final, el rendimiento extremo y la monetización.

### 2.1. Productización y Monetización Automática
* **Integración Nativa con Plataformas de Pago (Stripe):** Transformar el sistema de planes (Free, Starter, Pro) actual en modelos transaccionales. Gotenberg Manager podría procesar webhooks de Stripe Checkout; al completarse un pago, la cuota del cliente y los límites en Tyk se elevarían automáticamente y en tiempo real sin intervención humana.
* **Facturación por Volumen (Pay-as-you-go):** Aprovechar la tabla `usage_records` para emitir facturación dinámica a final de mes según las décimas de céntimo consumidas por conversión real exitosa.

### 2.2. Rendimiento y Caché (Ahorro de Cómputo)
* **Smart PDF Caching Middleware:** Implementar una capa en Gotenberg Manager que atrape las peticiones estáticas (ej. conversiones de la misma URL exacta). Usando una base de datos clave-valor (como el Redis que ya existe para Tyk), devolver el PDF ya pre-calculado si no han pasado *X* horas. Esto ahorraría levantar una pesada sesión de Chromium, reduciendo el consumo de CPU del cluster dramáticamente.

### 2.3. Ampliación del Portal de Clientes (Features)
* **Gestión Autónoma de API Keys:** Permitir que el cliente, desde su panel de control del portal web, revoque y genere nuevas "Tyk Keys" de forma autónoma por motivos de seguridad (rotación de claves sin depender del administrador).
* **Visor Interactivo Integrado:** En la generación manual desde el portal, en lugar de descargar el PDF forzosamente, pre-renderizarlo en un visor in-app (tipo `pdf.js`) permitiendo al cliente verificar márgenes antes de consumirlo.
* **Configuración Avanzada UI:** Exponer a los clientes controles visuales de Gotenberg que actualmente solo están disponibles vía API (formato Horizontal/Vertical, inserción de css para Print Backgrounds, escalado porcentual y selección de tamaño de papel A4/Letter/Custom).

### 2.4. DevOps Avanzado y Analítica
* **Exportador de Métricas (Prometheus/Grafana):** Aunque el Health Monitor en Go es útil, Gotenberg Manager debería exponer un endpoint `/metrics`. Flux podría desplegar el stack de monitoreo, permitiendo generar gráficas avanzadas (Dashboards operacionales) que crucen latencias HTTP con consumo de CPU de los nodos de Kubernetes.
* **Colas de Tareas Asíncronas:** Actualmente las subidas de archivos masivos o HTML pesados bloquean la petición HTTP. Para cargas pesadas tipo *Batch*, el portal podría aceptar el trabajo, meterlo en una cola (RabbitMQ/Redpanda), procesarlo en background y notificar por email o webhook al cliente cuando su PDF multi-megabyte esté listo.
