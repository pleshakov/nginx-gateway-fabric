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

- Total: 152
- Total Errors: 0
- Average Time: 148ms
- Reload distribution:
	- 500ms: 152
	- 1000ms: 152
	- 5000ms: 152
	- 10000ms: 152
	- 30000ms: 152
	- +Infms: 152

### Event Batch Processing

- Total: 410
- Average Time: 145ms
- Event Batch Processing distribution:
	- 500ms: 366
	- 1000ms: 405
	- 5000ms: 410
	- 10000ms: 410
	- 30000ms: 410
	- +Infms: 410

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

- Total: 152
- Total Errors: 0
- Average Time: 162ms
- Reload distribution:
	- 500ms: 152
	- 1000ms: 152
	- 5000ms: 152
	- 10000ms: 152
	- 30000ms: 152
	- +Infms: 152

### Event Batch Processing

- Total: 474
- Average Time: 137ms
- Event Batch Processing distribution:
	- 500ms: 423
	- 1000ms: 466
	- 5000ms: 474
	- 10000ms: 474
	- 30000ms: 474
	- +Infms: 474

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

- Total: 1145
- Total Errors: 0
- Average Time: 336ms
- Reload distribution:
	- 500ms: 918
	- 1000ms: 1145
	- 5000ms: 1145
	- 10000ms: 1145
	- 30000ms: 1145
	- +Infms: 1145

### Event Batch Processing

- Total: 1151
- Average Time: 403ms
- Event Batch Processing distribution:
	- 500ms: 824
	- 1000ms: 1150
	- 5000ms: 1150
	- 10000ms: 1150
	- 30000ms: 1150
	- +Infms: 1151

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
- Average Time: 94ms
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
Duration      [total, attack, wait]             30s, 29.999s, 845.744µs
Latencies     [min, mean, 50, 90, 95, 99, max]  556.146µs, 883.288µs, 841.635µs, 1.007ms, 1.062ms, 1.214ms, 21.097ms
Bytes In      [total, mean]                     4800000, 160.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:30000
Error Set:
```
```text
Requests      [total, rate, throughput]         30000, 1000.05, 1000.02
Duration      [total, attack, wait]             29.999s, 29.998s, 922.12µs
Latencies     [min, mean, 50, 90, 95, 99, max]  654.605µs, 965.039µs, 942.255µs, 1.128ms, 1.201ms, 1.338ms, 4.468ms
Bytes In      [total, mean]                     4800000, 160.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:30000
Error Set:
```
