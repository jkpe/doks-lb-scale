# doks-lb-scale

https://github.com/user-attachments/assets/9ab9f805-df87-49f1-98b4-edceb66a5b2f

A lightweight Kubernetes ([DOKS](https://docs.digitalocean.com/products/kubernetes/)) controller that automatically scales a DigitalOcean [Load Balancer](https://docs.digitalocean.com/products/networking/load-balancers/) based on metrics from either the DigitalOcean [API](https://docs.digitalocean.com/reference/api/digitalocean/) or Prometheus.

## How it works

- Watches `Service` objects of type `LoadBalancer` that include required annotations.
- Periodically fetches metrics from either:
  - **DigitalOcean API**: Direct load balancer metrics (e.g., throughput, requests)
  - **Prometheus**: Custom queries for ingress/application metrics
- Uses the configured metric to compute desired nodes.
- Computes the desired `size_unit` with hysteresis and min/max bounds and writes it back to the Service annotation.

DigitalOcean Cloud Controller Manager applies annotation changes to the actual Load Balancer.

## Prometheus Prerequisites

- Install from the DigitalOcean Kubernetes Marketplace:
  - [Kubernetes Metrics Server](https://marketplace.digitalocean.com/apps/kubernetes-metrics-server)
  - [Kubernetes Monitoring Stack](https://marketplace.digitalocean.com/apps/kubernetes-monitoring-stack)
  - [Nginx Ingress Controller](https://marketplace.digitalocean.com/apps/nginx-ingress-controller) (any ingress controller that exports metrics to Prometheus, such as Traefik, should work)

## Deploy

- Create a DigitalOcean API token with least privileges:
  - Create a token with [Custom Scopes](https://docs.digitalocean.com/reference/api/create-personal-access-token/)
  - Grant only these scopes:
    - `monitoring:read`
- Create a Kubernetes secret with your DigitalOcean API token:

```bash
kubectl -n kube-system create secret generic doks-lb-scale-secret --from-literal=token=$DO_API_TOKEN
```

- Apply RBAC and Deployment:
 
```bash
kubectl apply -f https://raw.githubusercontent.com/jkpe/doks-lb-scale/refs/heads/main/config/rbac.yaml
kubectl apply -f https://raw.githubusercontent.com/jkpe/doks-lb-scale/refs/heads/main/config/deployment.yaml
```

### Configuration Options

The controller supports two metrics sources:

1. **DigitalOcean API** (default): Set `DO_API_TOKEN` environment variable or `--do-token` flag
2. **Prometheus**: Set `PROMETHEUS_URL` environment variable or `--prom-url` flag

You can configure both sources simultaneously - the controller will route requests based on the metric prefix.

## Required annotations

- `kubernetes.digitalocean.com/load-balancer-id`: the DO LB ID.
- `doks-lb-scale/metric`: the metric to use:
  - **DO API metrics**: Direct metric names (e.g., `frontend_nlb_tcp_network_throughput`, `requests_per_second`)
  - **Prometheus metrics**: Must be prefixed with `promql:` (e.g., `promql:sum(rate(nginx_ingress_controller_requests[1m]))`)
- `doks-lb-scale/target-per-node`: REQUIRED: 
  - `req=<int>` for request-based metrics (HTTP requests, ingress metrics)
  - `nlb=<int>` for NLB throughput metrics specified in Mbps per node. The DigitalOcean API returns throughput in bytes/sec; the controller converts this to Mbps internally before computing desired nodes.

Optional annotations:
- `doks-lb-scale/hysteresis-percent`: default `20`.
- `doks-lb-scale/min-nodes`: default `1`.
- `doks-lb-scale/max-nodes`: default `200`.
- `doks-lb-scale/scale-down-delay-minutes`: optional. If set to a positive integer, delays any scale-down by the specified number of minutes. The controller first sets a not-before timestamp and only applies the scale-down once that time has passed. Scaling up clears any pending delay.
- `service.beta.kubernetes.io/do-loadbalancer-size-unit`: set by controller.

## Example Services

### Example 1: Prometheus Metrics (HTTP Requests)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx
  annotations:
    kubernetes.digitalocean.com/load-balancer-id: "your-load-balancer-id"
    service.beta.kubernetes.io/do-loadbalancer-type: "REGIONAL" # DigitalOcean HTTP Load Balancer
    service.beta.kubernetes.io/do-loadbalancer-size-unit: "1"
    doks-lb-scale/metric: "promql:sum(rate(nginx_ingress_controller_requests{ingress!=\"\",status!=\"\"}[1m]))"
    doks-lb-scale/target-per-node: "req=8000" # requests per node
    doks-lb-scale/hysteresis-percent: "20"
    doks-lb-scale/min-nodes: "1"
    doks-lb-scale/max-nodes: "50"
spec:
  type: LoadBalancer
  selector:
    app: nginx
  ports:
    - port: 80
      targetPort: 80
```

### Example 2: DigitalOcean API Metrics (Network Load Balancer Throughput)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx
  annotations:
    kubernetes.digitalocean.com/load-balancer-id: "your-load-balancer-id"
    service.beta.kubernetes.io/do-loadbalancer-type: "REGIONAL_NETWORK" # DigitalOcean Network Load Balancer
    service.beta.kubernetes.io/do-loadbalancer-size-unit: "1"
    doks-lb-scale/metric: "frontend_nlb_tcp_network_throughput"
    doks-lb-scale/target-per-node: "nlb=45" # Mbps per node (controller converts DO bytes/sec to Mbps)
    doks-lb-scale/hysteresis-percent: "20"
    doks-lb-scale/min-nodes: "1"
    doks-lb-scale/max-nodes: "50"
spec:
  type: LoadBalancer
  selector:
    app: nginx
  ports:
    - port: 80
      targetPort: 80
```

### Example 3: DigitalOcean API Metrics (Requests per Second)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx
  annotations:
    kubernetes.digitalocean.com/load-balancer-id: "your-load-balancer-id"
    service.beta.kubernetes.io/do-loadbalancer-type: "REGIONAL" # DigitalOcean HTTP Load Balancer
    service.beta.kubernetes.io/do-loadbalancer-size-unit: "1"
    doks-lb-scale/metric: "requests_per_second"
    doks-lb-scale/target-per-node: "req=8000" # requests per second per node
    doks-lb-scale/hysteresis-percent: "20"
    doks-lb-scale/min-nodes: "1"
    doks-lb-scale/max-nodes: "50"
spec:
  type: LoadBalancer
  selector:
    app: nginx
  ports:
    - port: 80
      targetPort: 80
```

## Example ingress-nginx Helm values

Use the following Helm values to deploy `ingress-nginx` with a `LoadBalancer` Service, metrics enabled for Prometheus scraping, and the required annotations for doks-lb-scale to manage the Load Balancer size:

```yaml
controller:
  replicaCount: 2
  service:
    type: LoadBalancer
    annotations:
      kubernetes.digitalocean.com/load-balancer-id: "your-load-balancer-id"
      service.beta.kubernetes.io/do-loadbalancer-type: "REGIONAL"
      doks-lb-scale/metric: "promql:sum(rate(nginx_ingress_controller_requests{ingress!=\"\",status!=\"\"}[1m]))"
      doks-lb-scale/target-per-node: "req=8000"
      doks-lb-scale/hysteresis-percent: "20"
      doks-lb-scale/min-nodes: "1"
      doks-lb-scale/max-nodes: "50"
      doks-lb-scale/scale-down-delay-minutes: "10"
      service.beta.kubernetes.io/do-loadbalancer-size-unit: "1"
  metrics:
    enabled: true
    service:
      servicePort: "9090"
  podAnnotations:
    prometheus.io/port: "10254"
    prometheus.io/scrape: "true"
```

## Metric Categories

The controller supports two metric categories:

### Request-based metrics (`req=INT`)
- **DO API**: `requests_per_second`, `http_requests_per_second`
- **Prometheus**: Any custom query prefixed with `promql:`
- **Use case**: HTTP/ingress traffic scaling

### NLB Throughput metrics (`nlb=INT`)
- **DO API**: `frontend_nlb_tcp_network_throughput`, `frontend_nlb_udp_network_throughput`
- **Prometheus**: Not supported for NLB metrics
- **Use case**: Network load balancer throughput scaling

The controller automatically detects the metric category and validates that the target configuration matches.

## Notes

- **DO API metrics**: The controller performs a direct API call to DigitalOcean's monitoring endpoint.
- **Prometheus metrics**: The controller performs a Prometheus instant query via `/api/v1/query?query=...` and uses the value from the first result.
- For up-to-date LB service annotations, see [DigitalOcean CCM annotations](https://github.com/digitalocean/digitalocean-cloud-controller-manager/blob/master/docs/controllers/services/annotations.md).
- For documented DigitalOcean Load Balancer node limits and scaling details, see the [DigitalOcean Load Balancer pricing and limits documentation](https://docs.digitalocean.com/products/networking/load-balancers/details/pricing/#regional-load-balancers).

## Hysteresis examples

`doks-lb-scale/hysteresis-percent` creates a no-change window around the current `size_unit`:

- lower = int(current × (1 − pct))
- upper = int(current × (1 + pct))

If desired is within [lower, upper], nothing changes.

Quick examples:
- current 10, pct 20% → window [8,12]; desired 12 = no change; 13 = scale up; 7 = scale down
- current 5, pct 10% → window [4,5]; desired 4 = no change; 6 = scale up; 3 = scale down
- current 1, pct 20% → window [0,1]; desired 1 = no change; ≥2 = scale up (min-nodes still applies)

## Verifying the Controller is Working

To verify that the doks-lb-scale controller is working properly, check the controller logs and monitor the service annotations.

### Check Controller Logs

View the controller logs to see the reconciliation process:

```bash
kubectl logs -n kube-system deployment/doks-lb-scale-controller -f
```

### Expected Log Output

When the controller starts successfully, you should see:

```log
[2025-08-14 09:39:02] INFO    setup       → starting manager
[2025-08-14 09:39:02] INFO    healthprobe → starting server at [::]:8080
[2025-08-14 09:39:02] INFO    leader      → attempting to acquire lease: kube-system/doks-lb-scale-controller
[2025-08-14 09:39:17] INFO    leader      → successfully acquired lease: kube-system/doks-lb-scale-controller
[2025-08-14 09:39:17] INFO    service     → Starting EventSource (kind: Service)
[2025-08-14 09:39:17] INFO    service     → Starting Controller (kind: Service)
[2025-08-14 09:39:17] INFO    service     → Starting workers (count: 1)
```

### Normal Operation Logs

During normal operation, you'll see periodic reconciliation logs:

```log
[2025-08-14 09:39:17] INFO  reconcile   service=ingress-nginx/ingress-nginx-controller
    ↳ Reconcile start
    ↳ Fetching metrics
        lbID    = 7a016a4b-20cb-4d97-9612-01dd421cea21
        metric  = promql: sum(rate(nginx_ingress_controller_requests{ingress!="",status!=""}[1m]))
    ↳ Metrics value
        value   = 0
    ↳ Computed desired nodes
        current = 2
        desired = 1
    ↳ Within hysteresis window — skipping update
        lower   = 1
        upper   = 2
        desired = 1
        current = 2
```

### Scaling Event Logs

When the controller scales the load balancer, you'll see:

```log
[2025-08-14 09:42:47] INFO  reconcile   service=ingress-nginx/ingress-nginx-controller
    ↳ Reconcile start
    ↳ Fetching metrics
        lbID    = 7a016a4b-20cb-4d97-9612-01dd421cea21
        metric  = promql: sum(rate(nginx_ingress_controller_requests{ingress!="",status!=""}[1m]))
    ↳ Metrics value
        value   = 2023.3658290843246
    ↳ Computed desired nodes
        current = 2
        desired = 3
    ↳ Updating service size-unit
        from    = 2
        to      = 3
    ↳ Service annotation updated
        size-unit = 3
```

### Delayed Scale Down Logs

When using the `doks-lb-scale/scale-down-delay-minutes` annotation, scale-down events are delayed:

```log
[2025-08-14 09:43:32] INFO  reconcile   service=ingress-nginx/ingress-nginx-controller
    ↳ Reconcile start
    ↳ Fetching metrics
        lbID    = 7a016a4b-20cb-4d97-9612-01dd421cea21
        metric  = promql: sum(rate(nginx_ingress_controller_requests{ingress!="",status!=""}[1m]))
    ↳ Metrics value
        value   = 131.66666666666666
    ↳ Computed desired nodes
        current = 3
        desired = 1
    ↳ Scale down scheduled after delay
        delayMinutes = 10
        notBefore    = 2025-08-14T09:53:32Z
        from         = 3
        to           = 1
```

The controller will show the delay being scheduled and then count down the remaining time until the scale-down can occur. If traffic increases during the delay period, the pending scale-down will be cancelled.

### Monitor Service Annotations

Check that the controller is updating the service annotation:

```bash
kubectl get service <your-service-name> -o yaml | grep -A 5 -B 5 "do-loadbalancer-size-unit"
```

You should see the `service.beta.kubernetes.io/do-loadbalancer-size-unit` annotation being updated as the controller scales the load balancer.

### Troubleshooting

If you don't see the expected logs:

1. **Check if the controller is running:**
   ```bash
   kubectl get pods -n kube-system | grep doks-lb-scale
   ```

2. **Verify the service has the required annotations:**
   ```bash
   kubectl get service <your-service-name> -o yaml | grep -A 10 -B 10 "doks-lb-scale"
   ```

## Contact

If you wish to learn more about DigitalOcean's services, you are welcome to reach out to the sales team at sales@digitalocean.com. A global team of talented engineers will be happy to provide assistance.

## License

This Kubernetes controller, associated scripts and documentation in this project are released under the MIT License.
