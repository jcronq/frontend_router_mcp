# Frontend Router MCP Monitoring

This directory contains comprehensive monitoring and observability configuration for the Frontend Router MCP service.

## Overview

The monitoring stack includes:
- **Health Checks**: HTTP endpoints for service health, readiness, and liveness
- **Metrics**: Prometheus metrics collection for performance monitoring
- **Logging**: Structured logging with correlation IDs and request tracing
- **Dashboards**: Grafana dashboards for visualization
- **Alerting**: Prometheus alerting rules and AlertManager configuration

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│                 │    │                 │    │                 │
│  Frontend       │    │  WebSocket      │    │  MCP Server     │
│  Router MCP     │────│  Server         │────│                 │
│  :8080, :8081   │    │  :8080          │    │  :8081          │
│                 │    │                 │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │
         │ Monitoring Port :8082
         │
         ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│                 │    │                 │    │                 │
│  Prometheus     │────│  Grafana        │    │  AlertManager   │
│  :9090          │    │  :3000          │    │  :9093          │
│                 │    │                 │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Components

### 1. Health Checks (`/health`, `/ready`, `/live`)

The service exposes three health check endpoints:

- **`/health`**: Detailed health information including component status
- **`/ready`**: Readiness check (returns 200 if ready to serve traffic)
- **`/live`**: Liveness check (returns 200 if service is alive)

Example health check response:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-01T12:00:00Z",
  "version": "1.0.0",
  "components": {
    "websocket_server": {
      "name": "websocket_server",
      "status": "healthy",
      "message": "WebSocket server is running",
      "last_checked": "2024-01-01T12:00:00Z",
      "metadata": {
        "client_count": "5"
      }
    },
    "mcp_server": {
      "name": "mcp_server",
      "status": "healthy",
      "message": "MCP server is running",
      "last_checked": "2024-01-01T12:00:00Z"
    }
  },
  "system_info": {
    "goroutines": 42,
    "memory_usage_bytes": 50331648,
    "cpu_count": 8,
    "go_version": "go1.21.0",
    "os": "linux",
    "architecture": "amd64"
  },
  "uptime": "1h30m45s",
  "request_id": "req-12345"
}
```

### 2. Metrics Collection

Prometheus metrics are exposed on `/metrics` endpoint:

#### HTTP Metrics
- `http_requests_total`: Total HTTP requests by method, endpoint, and status code
- `http_request_duration_seconds`: HTTP request duration histogram
- `http_response_size_bytes`: HTTP response size histogram
- `http_requests_in_flight`: Current number of HTTP requests being processed

#### WebSocket Metrics
- `websocket_connections_active`: Current number of active WebSocket connections
- `websocket_messages_total`: Total WebSocket messages by direction and type
- `websocket_message_size_bytes`: WebSocket message size histogram
- `websocket_connections_total`: Total WebSocket connections by event type

#### MCP Metrics
- `mcp_requests_total`: Total MCP requests by method and status
- `mcp_request_duration_seconds`: MCP request duration histogram
- `mcp_tool_executions_total`: Total MCP tool executions by tool name and status
- `mcp_active_requests`: Current number of active MCP requests

#### System Metrics
- `goroutines_active`: Number of active goroutines
- `memory_usage_bytes`: Memory usage in bytes
- `channel_utilization_percent`: Channel utilization percentage

#### Business Metrics
- `user_questions_total`: Total user questions by status
- `user_response_time_seconds`: User response time histogram
- `active_user_sessions`: Number of active user sessions

### 3. Structured Logging

The service uses structured JSON logging with correlation IDs:

```json
{
  "timestamp": "2024-01-01T12:00:00Z",
  "level": "INFO",
  "message": "HTTP request processed",
  "request_id": "req-12345",
  "trace_id": "trace-67890",
  "span_id": "span-abc",
  "service": "frontend-router-mcp",
  "component": "http",
  "http_method": "GET",
  "http_path": "/health",
  "status_code": 200,
  "duration": 0.001234,
  "client_ip": "192.168.1.1"
}
```

### 4. Middleware

The service includes comprehensive middleware for:
- Request ID generation and propagation
- Distributed tracing with trace and span IDs
- Structured logging with context
- Metrics collection
- CORS handling
- Security headers
- Panic recovery
- Rate limiting

## Deployment

### Using Docker Compose

1. **Start the monitoring stack**:
   ```bash
   cd monitoring
   docker-compose -f docker-compose.monitoring.yml up -d
   ```

2. **Access the services**:
   - Application: http://localhost:8080 (WebSocket), http://localhost:8081 (MCP)
   - Monitoring: http://localhost:8082/health
   - Prometheus: http://localhost:9090
   - Grafana: http://localhost:3000 (admin/admin)
   - AlertManager: http://localhost:9093

### Manual Deployment

1. **Install dependencies**:
   ```bash
   go mod download
   ```

2. **Build the application**:
   ```bash
   go build -o frontend-router-mcp cmd/server/main_monitoring.go
   ```

3. **Run the application**:
   ```bash
   ./frontend-router-mcp --ws-port=8080 --mcp-port=8081 --monitoring-port=8082
   ```

4. **Configure Prometheus** to scrape metrics from `http://localhost:8082/metrics`

## Configuration

### Environment Variables

- `LOG_LEVEL`: Set log level (debug, info, warn, error, fatal)
- `WS_PORT`: WebSocket server port (default: 8080)
- `MCP_PORT`: MCP server port (default: 8081)
- `MONITORING_PORT`: Monitoring endpoints port (default: 8082)

### Command Line Flags

```bash
./frontend-router-mcp --help
  -ws-port int
        WebSocket server port (default 8080)
  -mcp-port int
        MCP server port (default 8081)
  -monitoring-port int
        Monitoring endpoints port (default 8082)
  -log-level string
        Log level (debug, info, warn, error, fatal) (default "info")
```

## Monitoring Stack Configuration

### Prometheus Configuration

- **Scrape Interval**: 15 seconds
- **Targets**: Application metrics and health endpoints
- **Retention**: 200 hours
- **Alert Rules**: Comprehensive alerting for service health and performance

### Grafana Dashboard

The dashboard includes panels for:
- Service health status
- HTTP request rates and response times
- WebSocket connection metrics
- MCP server performance
- System resource utilization
- Business metrics (user questions, sessions)
- Error rates and top endpoints

### AlertManager

Configured alerts include:
- **Critical**: Service down, high error rates
- **Warning**: High response times, memory usage, goroutine count
- **Info**: No active user sessions

## Alerting Rules

### Service Health
- `ServiceDown`: Service is unreachable
- `HighErrorRate`: HTTP 5xx error rate > 10%
- `HighResponseTime`: 95th percentile response time > 2s

### Performance
- `HighMemoryUsage`: Memory usage > 1GB
- `HighGoroutineCount`: Goroutine count > 10,000
- `ChannelUtilizationHigh`: Channel utilization > 80%

### WebSocket
- `WebSocketConnectionsHigh`: WebSocket connections > 1,000
- `WebSocketMessageBacklog`: Message backlog > 100

### MCP
- `MCPHighErrorRate`: MCP error rate > 5%
- `MCPToolExecutionFailures`: Tool execution failure rate > 10%

### Business Logic
- `UserQuestionTimeout`: User question timeout rate > 1%
- `NoActiveUserSessions`: No active sessions for > 10 minutes

## Troubleshooting

### Common Issues

1. **Metrics not appearing in Prometheus**:
   - Check if the application is running on the correct port
   - Verify Prometheus configuration
   - Check firewall settings

2. **Grafana dashboard not loading**:
   - Verify Prometheus datasource configuration
   - Check Grafana logs for errors
   - Ensure dashboard JSON is valid

3. **Alerts not firing**:
   - Check AlertManager configuration
   - Verify alert rule expressions
   - Check notification channel settings

### Debugging Commands

```bash
# Check application health
curl http://localhost:8082/health

# Check metrics endpoint
curl http://localhost:8082/metrics

# Check Prometheus targets
curl http://localhost:9090/api/v1/targets

# Check AlertManager configuration
curl http://localhost:9093/api/v1/status/config
```

## Best Practices

1. **Monitor your monitors**: Set up alerts for monitoring infrastructure
2. **Use correlation IDs**: Always propagate request IDs for tracing
3. **Log structured data**: Use structured logging for better searchability
4. **Set appropriate SLIs/SLOs**: Define service level indicators and objectives
5. **Regular dashboard reviews**: Keep dashboards up to date with system changes
6. **Alert fatigue prevention**: Tune alerts to reduce false positives
7. **Documentation**: Keep monitoring documentation current

## Performance Considerations

- Metrics collection has minimal performance impact (<1% CPU overhead)
- Structured logging is optimized for high throughput
- Health checks are lightweight and cacheable
- Prometheus retention is configurable based on storage requirements

## Security

- Health check endpoints are secured with CORS and security headers
- Metrics don't expose sensitive information
- Logs can be configured to exclude sensitive data
- Access to monitoring endpoints should be restricted in production