# doks-lb-scale

A lightweight Kubernetes controller that automatically scales a DigitalOcean Load Balancer node size (size unit) based on metrics from DigitalOcean Monitoring.

## How it works
- Watches `Service` objects of type `LoadBalancer` that include required annotations.
- Periodically fetches the configured DigitalOcean metric for the referenced Load Balancer ID.
- Infers scaling category from the metric:
  - Throughput metrics (`frontend_nlb_{tcp,udp}_network_throughput`) are treated as NLB.
  - Other metrics (e.g., `frontend_http_requests_per_second`, `frontend_connections_current`) are treated as HTTP requests.
- Computes the desired `size_unit` with hysteresis and min/max bounds and writes it back to the Service annotation.

DigitalOcean Cloud Controller Manager applies annotation changes to the actual Load Balancer.

## Required annotations
- `kubernetes.digitalocean.com/load-balancer-id`: the DO LB ID.
- `doks-lb-scale/metric`: the DO Monitoring metric to use.
- `doks-lb-scale/target-per-node`: REQUIRED, must be exactly one of:
  - `req=<int>`: requests per second per node target (used for HTTP-style metrics)
  - `nlb=<int>`: Mbps per node target (used for NLB throughput metrics)

The controller infers whether the metric is NLB or HTTP. If the key in `target-per-node` does not match the inferred category, the controller skips scaling and logs.

Optional annotations:
- `doks-lb-scale/hysteresis-percent`: default `20`.
- `doks-lb-scale/min-nodes`: default `1`.
- `doks-lb-scale/max-nodes`: default `200`.
- `service.beta.kubernetes.io/do-loadbalancer-size-unit`: set by controller.

## Example Service (HTTP requests)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx
  annotations:
    kubernetes.digitalocean.com/load-balancer-id: "your-load-balancer-id"
    service.beta.kubernetes.io/do-loadbalancer-size-unit: "1"
    doks-lb-scale/metric: "frontend_http_requests_per_second"
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

## Example Service (NLB throughput)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: tcp-svc
  annotations:
    kubernetes.digitalocean.com/load-balancer-id: "your-load-balancer-id"
    service.beta.kubernetes.io/do-loadbalancer-size-unit: "1"
    doks-lb-scale/metric: "frontend_nlb_tcp_network_throughput" # or frontend_nlb_udp_network_throughput
    doks-lb-scale/target-per-node: "nlb=40" # Mbps per node
    doks-lb-scale/hysteresis-percent: "20"
    doks-lb-scale/min-nodes: "1"
    doks-lb-scale/max-nodes: "50"
spec:
  type: LoadBalancer
  selector:
    app: tcp-app
  ports:
    - port: 5432
      targetPort: 5432
```

## Deploy

- Create a DigitalOcean API token with least privileges:
  - Create a token with Custom Scopes following the official guide: [`Create a personal access token`](https://docs.digitalocean.com/reference/api/create-personal-access-token/)
  - Grant only these scopes:
    - `load_balancer:update`
    - `monitoring:read`
- Create a Kubernetes secret with your DigitalOcean API token:

```bash
kubectl -n kube-system create secret generic digitalocean-lb-scale --from-literal=token=DO_API_TOKEN
```
- Apply RBAC and Deployment:

```bash
kubectl apply -f config/rbac.yaml
kubectl apply -f config/deployment.yaml
```

## Contributing

### Build and push container image (for contributors)

```bash
# Build multi-arch image (adjust registry/name as needed)
IMAGE=ghcr.io/your-org/doks-lb-scale:latest
docker build --platform linux/amd64,linux/arm64 -t $IMAGE --push .
```

After pushing, update `config/deployment.yaml` to point to your published image.

## Notes
- NLBs only scale when using throughput metrics. If a non-throughput metric is used with `nlb=<int>`, scaling is skipped and logged.
- The controller queries DO Monitoring using `GET /v2/monitoring/metrics/load_balancer/{metric}?lb_id=...&start=...&end=...` with epoch seconds, and uses the latest datapoint.
- Up to date LB service annotations: [DigitalOcean CCM annotations](https://github.com/digitalocean/digitalocean-cloud-controller-manager/blob/master/docs/controllers/services/annotations.md)

## Hysteresis examples

`doks-lb-scale/hysteresis-percent` creates a no-change window around the current `size_unit`:

- lower = int(current × (1 − pct))
- upper = int(current × (1 + pct))

If desired is within [lower, upper], nothing changes.

Quick examples:
- current 10, pct 20% → window [8,12]; desired 12 = no change; 13 = scale up; 7 = scale down
- current 5, pct 10% → window [4,5]; desired 4 = no change; 6 = scale up; 3 = scale down
- current 1, pct 20% → window [0,1]; desired 1 = no change; ≥2 = scale up (min-nodes still applies)

Notes: integer truncation is used; min/max nodes are enforced.
