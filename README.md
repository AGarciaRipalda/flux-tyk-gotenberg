# Flux-Tyk-Gotenberg

This project provides a GitOps-based Kubernetes deployment using [Flux](https://fluxcd.io/) to manage a [Tyk API Gateway](https://tyk.io/) and [Gotenberg](https://gotenberg.dev/) (a developer-friendly API for PDF conversion). 

## Project Structure

- `apps/`: Contains the base manifests for the different components.
  - `gotenberg/`: Deployments and services for the Gotenberg PDF converter.
  - `tyk/`: Deployments and services for the Tyk API Gateway and Redis.
  - `tyk-apis/`: Tyk API configurations (ConfigMaps).
- `clusters/my-cluster/`: Flux kustomization manifests connecting the GitHub repository to the local cluster deployment.

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
