# Flux-Tyk-Gotenberg

This project provides a GitOps-based Kubernetes deployment using [Flux](https://fluxcd.io/) to manage a [Tyk API Gateway](https://tyk.io/) and [Gotenberg](https://gotenberg.dev/) (a developer-friendly API for PDF conversion). 

## Project Structure

- `apps/`: Contains the base manifests for the different components.
  - `gotenberg/`: Deployments and services for the Gotenberg PDF converter.
  - `tyk/`: Deployments and services for the Tyk API Gateway and Redis.
  - `tyk-apis/`: Tyk API configurations (ConfigMaps).
- `clusters/my-cluster/`: Flux kustomization manifests connecting the GitHub repository to the local cluster deployment.

---

## Infrastructure and Architecture Overview

This document provides a technical explanation of the current state of the project and how its infrastructure components work together.

### System Overview

The project implements a GitOps-driven Kubernetes architecture managed by **Flux CD**. The system consists of an API Gateway (**Tyk**) that manages and proxies traffic to a backend PDF conversion engine (**Gotenberg**). The complete deployment lifecycle, from foundational infrastructure to application configuration, is managed declaratively through Kubernetes manifests stored in this Git repository.

### Component Architecture

![Component Architecture Diagram](assets/component_architecture.png)

The infrastructure is logically separated into three main application domains located in the `apps/` directory:

#### 1. Gotenberg (`apps/gotenberg`)
Gotenberg is a Docker-powered stateless API for PDF conversion.
- **Deployment**: Runs a single replica of the `gotenberg/gotenberg:8` image.
- **Service**: Exposed internally within the cluster on port `3000`.
- **Namespace**: `gotenberg`

#### 2. Tyk API Gateway & Redis (`apps/tyk`)
Tyk acts as the primary entry point and API management layer. It requires a Redis instance for caching, rate limiting, and analytics.
- **Tyk Gateway**:
  - Runs `tykio/tyk-gateway:v5.3.0`.
  - Exposed via a Kubernetes Service on port `8080`.
  - Configured via environment variables to connect to Redis (`TYK_GW_REDIS_HOST="tyk-redis"`).
  - Dynamically mounts API definitions directly from a Kubernetes ConfigMap.
  - **Namespace**: `tyk`
- **Tyk Redis**:
  - Runs a lightweight `redis:6-alpine` instance.
  - Exposed internally to the gateway on port `6379`.

#### 3. API Configuration (`apps/tyk-apis`)
Instead of hardcoding APIs into the Tyk Gateway, APIs are defined as Kubernetes ConfigMaps, offering a decoupled configuration approach.
- **`gotenberg-api-definition` ConfigMap**:
  - Defines an API with the ID `gotenberg-v1`.
  - Listens on the path `/pdf/`.
  - Proxies traffic to the internal Gotenberg service using the fully qualified domain name (FQDN): `http://gotenberg.gotenberg.svc.cluster.local:3000/`.
  - Currently configured as `use_keyless: true` (open API, no authentication required).
  - The `tyk-gateway` Deployment mounts this ConfigMap into the `/opt/tyk-gateway/apps` directory, allowing Tyk to discover the API upon startup.

### GitOps Workflow (Flux CD)

![GitOps Workflow Diagram](assets/gitops_workflow.png)

The continuous delivery of the cluster state is managed by Flux CD, configured in `clusters/my-cluster/`. Flux continuously reconciles the cluster state against the GitHub repository.

Three specific `Kustomization` resources drive the synchronization every 1 minute:
1. **`infra-tyk-sync.yaml`**: Targets `./apps/tyk/` to deploy the Tyk Gateway and Redis infrastructure.
2. **`apps-gotenberg-sync.yaml`**: Targets `./apps/gotenberg/` to deploy the PDF conversion engine.
3. **`tyk-apis-sync.yaml`**: Targets `./apps/tyk-apis/` to inject the API definitions into the `tyk` namespace.

*Note: Since these syncs happen in parallel or sequentially depending on Flux's controller loops, there may occasionally be a race condition where the Tyk Gateway boots up before the API ConfigMap is injected. A manual rollout restart of the Tyk deployment resolves this (as detailed in the README).*

### Traffic Flow Request Lifecycle

![Traffic Flow Diagram](assets/traffic_flow_diagram.png)

From a technical point of view, when a user makes a request to convert a PDF, the traffic flow works as follows:

1. **Localhost Ingress**: The user opens a `kubectl port-forward` bridge from their local machine to the Tyk Gateway Pod on port `8080`.
2. **API Gateway Evaluation**: The HTTP request hits Tyk (e.g., `POST http://localhost:8080/pdf/...`). Tyk matches the `/pdf/` path to the `gotenberg-v1` API definition.
3. **Internal Proxying**: Tyk strips the `/pdf/` prefix from the URL path and proxies the modified request to the internal Kubernetes DNS address of the Gotenberg service (`http://gotenberg.gotenberg.svc.cluster.local:3000/`).
4. **Backend Processing**: The Gotenberg pod receives the request, spins up Chromium (or LibreOffice, depending on the route), renders the content, and generates a PDF file in memory.
5. **Response Delivery**: Gotenberg streams the PDF binary back through Tyk, which returns the file to the user over the port-forward tunnel.

---

## Daily Cheat Sheet

Here is the official, battle-tested daily cheat sheet for interacting with this project.

### Scenario A: The "Daily Routine"

If your computer went to sleep or restarted, but your local Kubernetes cluster (Rancher/Docker) is running, Flux automatically keeps your apps alive. You just need to open the door and test it.

**1. Check if the cluster is awake:**

```bash
kubectl get pods -A | grep -E "tyk|gotenberg"
```
*(Ensure tyk-gateway, tyk-redis, and gotenberg all say 1/1 Running).*

**2. Open the bridge to your localhost (Mac/Windows):**

Because your gateway lives safely isolated inside the Kubernetes cluster, you need to open a direct tunnel from your localhost into the Tyk pod. Run this in your first terminal window:

```bash
kubectl port-forward deployment/tyk-gateway 8080:8080 -n tyk
```
*(Crucial: Leave this terminal window open and running. If you close it or press Ctrl + C, the bridge collapses!)*

**3. Test the API:**

Open a brand new terminal tab (while the bridge is running in the background) and fire off your PDF conversion request:

```bash
curl -v -X POST http://localhost:8080/pdf/forms/chromium/convert/url \
  -F url="https://google.com" \
  -o daily_test.pdf

# Open the downloaded file (on MacOS: open, on Windows: Start or double click)
# open daily_test.pdf
```

---

### Scenario B: The "Total Rebuild"

If you ever wipe your cluster, get a new computer, or accidentally destroy your setup, this is exactly how you rebuild the entire architecture in 3 minutes using the code pushed to this repository.

**1. Install Flux and Link to GitHub:**

This installs the controllers and tells them to read your existing `flux-tyk-gotenberg` repo.

```bash
flux bootstrap github \
  --owner=AGarciaRipalda \
  --repository=flux-tyk-gotenberg \
  --branch=main \
  --path=./clusters/my-cluster \
  --personal
```

**2. Watch the Magic Happen:**

```bash
flux get kustomizations -w
```
*(Wait until all rows say True. Press Ctrl + C to exit).*

**3. FIX THE RACE CONDITION (Crucial Step!):**

Tyk might boot up too fast and miss the API ConfigMap. Force Tyk to restart so it reads the configuration file:

```bash
kubectl rollout restart deployment tyk-gateway -n tyk
```

**4. Open the Bridge and Test:**

Follow the steps in Scenario A (Daily Routine) to open the port-forward and test the conversion.
