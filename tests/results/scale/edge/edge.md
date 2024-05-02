# Results

## Test environment

NGINX Plus: false

GKE Cluster:

- Node count: 12
- k8s version: v1.28.7-gke.1026000
- vCPUs per node: 16
- RAM per node: 65855096Ki
- Max pods per node: 110
- Zone: us-central1-c
- Instance Type: n2d-standard-16

## Test TestScale_Listeners

### Reloads

- Total: 155
- Total Errors: 0
- Average Time: 140ms
- Reload distribution:
	- 500ms: 155
	- 1000ms: 155
	- 5000ms: 155
	- 10000ms: 155
	- 30000ms: 155
	- +Infms: 155

### Event Batch Processing

- Total: 413
- Average Time: 138ms
- Event Batch Processing distribution:
	- 500ms: 373
	- 1000ms: 409
	- 5000ms: 413
	- 10000ms: 413
	- 30000ms: 413
	- +Infms: 413

### Errors

- NGF errors: 0
- NGF container restarts: 0
- NGINX errors: 0
- NGINX container restarts: 0

### Graphs and Logs

See [output directory](./TestScale_Listeners) for more details.
The logs are attached only if there are errors.

## Test TestScale_HTTPSListeners

### Reloads

- Total: 154
- Total Errors: 0
- Average Time: 160ms
- Reload distribution:
	- 500ms: 154
	- 1000ms: 154
	- 5000ms: 154
	- 10000ms: 154
	- 30000ms: 154
	- +Infms: 154

### Event Batch Processing

- Total: 476
- Average Time: 136ms
- Event Batch Processing distribution:
	- 500ms: 429
	- 1000ms: 470
	- 5000ms: 476
	- 10000ms: 476
	- 30000ms: 476
	- +Infms: 476

### Errors

- NGF errors: 0
- NGF container restarts: 0
- NGINX errors: 0
- NGINX container restarts: 0

### Graphs and Logs

See [output directory](./TestScale_HTTPSListeners) for more details.
The logs are attached only if there are errors.

## Test TestScale_HTTPRoutes

### Reloads

- Total: 34
- Total Errors: 0
- Average Time: 130ms
- Reload distribution:
	- 500ms: 34
	- 1000ms: 34
	- 5000ms: 34
	- 10000ms: 34
	- 30000ms: 34
	- +Infms: 34

### Event Batch Processing

- Total: 39
- Average Time: 153ms
- Event Batch Processing distribution:
	- 500ms: 38
	- 1000ms: 39
	- 5000ms: 39
	- 10000ms: 39
	- 30000ms: 39
	- +Infms: 39

### Errors

- NGF errors: 0
- NGF container restarts: 0
- NGINX errors: 0
- NGINX container restarts: 0

### Graphs and Logs

See [output directory](./TestScale_HTTPRoutes) for more details.
The logs are attached only if there are errors.

## Test TestScale_UpstreamServers

### Reloads

- Total: 6
- Total Errors: 0
- Average Time: 126ms
- Reload distribution:
	- 500ms: 6
	- 1000ms: 6
	- 5000ms: 6
	- 10000ms: 6
	- 30000ms: 6
	- +Infms: 6

### Event Batch Processing

- Total: 9
- Average Time: 93ms
- Event Batch Processing distribution:
	- 500ms: 9
	- 1000ms: 9
	- 5000ms: 9
	- 10000ms: 9
	- 30000ms: 9
	- +Infms: 9

### Errors

- NGF errors: 0
- NGF container restarts: 0
- NGINX errors: 0
- NGINX container restarts: 0

### Graphs and Logs

See [output directory](./TestScale_UpstreamServers) for more details.
The logs are attached only if there are errors.

## Test TestScale_HTTPMatches

```text
Requests      [total, rate, throughput]         30000, 1000.04, 1000.01
Duration      [total, attack, wait]             30s, 29.999s, 742.541µs
Latencies     [min, mean, 50, 90, 95, 99, max]  552.289µs, 872.413µs, 856.637µs, 1.008ms, 1.077ms, 1.254ms, 12.501ms
Bytes In      [total, mean]                     4800000, 160.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:30000
Error Set:
```
```text
Requests      [total, rate, throughput]         30000, 1000.06, 1000.03
Duration      [total, attack, wait]             29.999s, 29.998s, 971.232µs
Latencies     [min, mean, 50, 90, 95, 99, max]  631.338µs, 959.596µs, 934.51µs, 1.124ms, 1.191ms, 1.338ms, 11.814ms
Bytes In      [total, mean]                     4800000, 160.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:30000
Error Set:
```
