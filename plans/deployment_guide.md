# Deployment Guide

## Overview
This guide provides comprehensive deployment instructions for the Frontend Router MCP service across different environments: local development, staging, and production.

## Prerequisites

### System Requirements
- **Operating System**: Linux (Ubuntu 20.04+), macOS (10.15+), or Windows 10+
- **Go Version**: 1.21 or later
- **Memory**: Minimum 2GB RAM, Recommended 8GB+
- **Storage**: Minimum 10GB free space
- **Network**: Ports 8080 (WebSocket) and 8081 (MCP) available

### Required Tools
- Docker & Docker Compose
- Kubernetes (kubectl)
- Terraform (for infrastructure)
- Git
- Make

## Local Development Setup

### 1. Environment Setup

**Clone Repository:**
```bash
git clone https://github.com/jcronq/frontend_router_mcp.git
cd frontend_router_mcp
```

**Install Dependencies:**
```bash
# Install Go dependencies
go mod download

# Verify installation
go version
```

**Environment Variables:**
```bash
# Create .env file
cat > .env << EOF
# Server Configuration
WS_PORT=8080
MCP_PORT=8081
LOG_LEVEL=debug

# Redis Configuration (optional for local dev)
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_DB=0

# Database Configuration (optional)
DB_HOST=localhost
DB_PORT=5432
DB_NAME=frontend_router_mcp
DB_USER=postgres
DB_PASSWORD=password
EOF
```

### 2. Local Development with Docker

**Docker Compose Configuration:**
```yaml
# docker-compose.dev.yml
version: '3.8'

services:
  app:
    build:
      context: .
      dockerfile: Dockerfile.dev
    ports:
      - "8080:8080"
      - "8081:8081"
    environment:
      - WS_PORT=8080
      - MCP_PORT=8081
      - LOG_LEVEL=debug
    volumes:
      - .:/app
      - /app/vendor
    depends_on:
      - redis
      - postgres

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: frontend_router_mcp
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: password
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  redis_data:
  postgres_data:
```

**Start Development Environment:**
```bash
# Start all services
docker-compose -f docker-compose.dev.yml up -d

# View logs
docker-compose -f docker-compose.dev.yml logs -f app

# Stop services
docker-compose -f docker-compose.dev.yml down
```

### 3. Native Development

**Run Locally:**
```bash
# Build the application
make build

# Run with default configuration
./frontend-router-mcp

# Run with custom ports
./frontend-router-mcp -ws-port 8080 -mcp-port 8081

# Run with environment variables
WS_PORT=8080 MCP_PORT=8081 ./frontend-router-mcp
```

**Development Scripts:**
```bash
# Create Makefile
cat > Makefile << 'EOF'
.PHONY: build test clean run dev docker-build docker-run

# Build the application
build:
	go build -o frontend-router-mcp ./cmd/server

# Run tests
test:
	go test ./...

# Clean build artifacts
clean:
	rm -f frontend-router-mcp

# Run application
run: build
	./frontend-router-mcp

# Development mode with auto-reload
dev:
	air -c .air.toml

# Build Docker image
docker-build:
	docker build -t frontend-router-mcp .

# Run in Docker
docker-run: docker-build
	docker run -p 8080:8080 -p 8081:8081 frontend-router-mcp

# Run tests with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out
EOF
```

## Staging Environment

### 1. Staging Infrastructure

**Terraform Configuration:**
```hcl
# infrastructure/staging/main.tf
provider "aws" {
  region = var.aws_region
}

# VPC Configuration
resource "aws_vpc" "staging" {
  cidr_block           = "10.1.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name        = "frontend-router-mcp-staging"
    Environment = "staging"
  }
}

# EKS Cluster
resource "aws_eks_cluster" "staging" {
  name     = "frontend-router-mcp-staging"
  role_arn = aws_iam_role.eks_cluster.arn
  version  = "1.24"

  vpc_config {
    subnet_ids = [
      aws_subnet.private_a.id,
      aws_subnet.private_b.id
    ]
  }

  depends_on = [
    aws_iam_role_policy_attachment.eks_cluster_policy,
  ]
}

# RDS Instance
resource "aws_db_instance" "staging" {
  identifier     = "frontend-router-mcp-staging"
  engine         = "postgres"
  engine_version = "15.3"
  instance_class = "db.t3.micro"
  
  allocated_storage = 20
  storage_type     = "gp2"
  
  db_name  = "frontend_router_mcp"
  username = "postgres"
  password = var.db_password
  
  vpc_security_group_ids = [aws_security_group.rds.id]
  db_subnet_group_name   = aws_db_subnet_group.staging.name
  
  skip_final_snapshot = true
  
  tags = {
    Name        = "frontend-router-mcp-staging"
    Environment = "staging"
  }
}

# ElastiCache Redis
resource "aws_elasticache_subnet_group" "staging" {
  name       = "frontend-router-mcp-staging"
  subnet_ids = [aws_subnet.private_a.id, aws_subnet.private_b.id]
}

resource "aws_elasticache_cluster" "staging" {
  cluster_id           = "frontend-router-mcp-staging"
  engine               = "redis"
  node_type            = "cache.t3.micro"
  num_cache_nodes      = 1
  parameter_group_name = "default.redis7"
  port                 = 6379
  subnet_group_name    = aws_elasticache_subnet_group.staging.name
  security_group_ids   = [aws_security_group.elasticache.id]
  
  tags = {
    Name        = "frontend-router-mcp-staging"
    Environment = "staging"
  }
}
```

### 2. Kubernetes Deployment

**Namespace and ConfigMap:**
```yaml
# k8s/staging/namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: frontend-router-mcp-staging
  labels:
    name: frontend-router-mcp-staging
    environment: staging

---
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: frontend-router-mcp-staging
data:
  WS_PORT: "8080"
  MCP_PORT: "8081"
  LOG_LEVEL: "info"
  REDIS_HOST: "frontend-router-mcp-staging.cache.amazonaws.com"
  REDIS_PORT: "6379"
```

**Deployment:**
```yaml
# k8s/staging/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend-router-mcp
  namespace: frontend-router-mcp-staging
spec:
  replicas: 2
  selector:
    matchLabels:
      app: frontend-router-mcp
  template:
    metadata:
      labels:
        app: frontend-router-mcp
    spec:
      containers:
      - name: frontend-router-mcp
        image: frontend-router-mcp:staging
        ports:
        - containerPort: 8080
          name: websocket
        - containerPort: 8081
          name: mcp
        - containerPort: 8082
          name: metrics
        env:
        - name: WS_PORT
          valueFrom:
            configMapKeyRef:
              name: app-config
              key: WS_PORT
        - name: MCP_PORT
          valueFrom:
            configMapKeyRef:
              name: app-config
              key: MCP_PORT
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: app-secrets
              key: db-password
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8082
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8082
          initialDelaySeconds: 5
          periodSeconds: 5
```

**Service and Ingress:**
```yaml
# k8s/staging/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: frontend-router-mcp-service
  namespace: frontend-router-mcp-staging
spec:
  selector:
    app: frontend-router-mcp
  ports:
  - name: websocket
    port: 8080
    targetPort: 8080
  - name: mcp
    port: 8081
    targetPort: 8081
  type: ClusterIP

---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: frontend-router-mcp-ingress
  namespace: frontend-router-mcp-staging
  annotations:
    kubernetes.io/ingress.class: "nginx"
    nginx.ingress.kubernetes.io/websocket-services: "frontend-router-mcp-service"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
spec:
  rules:
  - host: staging.frontend-router-mcp.example.com
    http:
      paths:
      - path: /ws
        pathType: Prefix
        backend:
          service:
            name: frontend-router-mcp-service
            port:
              number: 8080
      - path: /mcp
        pathType: Prefix
        backend:
          service:
            name: frontend-router-mcp-service
            port:
              number: 8081
```

### 3. Deployment Scripts

**Staging Deployment Script:**
```bash
#!/bin/bash
# scripts/deploy-staging.sh

set -e

echo "Starting staging deployment..."

# Build and push Docker image
echo "Building Docker image..."
docker build -t frontend-router-mcp:staging .

# Push to ECR
echo "Pushing to ECR..."
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin 123456789012.dkr.ecr.us-east-1.amazonaws.com
docker tag frontend-router-mcp:staging 123456789012.dkr.ecr.us-east-1.amazonaws.com/frontend-router-mcp:staging
docker push 123456789012.dkr.ecr.us-east-1.amazonaws.com/frontend-router-mcp:staging

# Apply Kubernetes manifests
echo "Applying Kubernetes manifests..."
kubectl apply -f k8s/staging/namespace.yaml
kubectl apply -f k8s/staging/configmap.yaml
kubectl apply -f k8s/staging/secrets.yaml
kubectl apply -f k8s/staging/deployment.yaml
kubectl apply -f k8s/staging/service.yaml
kubectl apply -f k8s/staging/ingress.yaml

# Wait for deployment to be ready
echo "Waiting for deployment to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment/frontend-router-mcp -n frontend-router-mcp-staging

# Run health check
echo "Running health check..."
kubectl get pods -n frontend-router-mcp-staging
kubectl get svc -n frontend-router-mcp-staging
kubectl get ingress -n frontend-router-mcp-staging

echo "Staging deployment completed successfully!"
```

## Production Environment

### 1. Production Infrastructure

**High-Availability Architecture:**
```hcl
# infrastructure/production/main.tf
provider "aws" {
  region = var.aws_region
}

# Multi-AZ EKS Cluster
resource "aws_eks_cluster" "production" {
  name     = "frontend-router-mcp-production"
  role_arn = aws_iam_role.eks_cluster.arn
  version  = "1.24"

  vpc_config {
    subnet_ids = [
      aws_subnet.private_a.id,
      aws_subnet.private_b.id,
      aws_subnet.private_c.id
    ]
    endpoint_private_access = true
    endpoint_public_access  = true
  }

  depends_on = [
    aws_iam_role_policy_attachment.eks_cluster_policy,
  ]
}

# RDS Multi-AZ
resource "aws_db_instance" "production" {
  identifier     = "frontend-router-mcp-production"
  engine         = "postgres"
  engine_version = "15.3"
  instance_class = "db.r6g.large"
  
  allocated_storage     = 100
  max_allocated_storage = 1000
  storage_type         = "gp3"
  storage_encrypted    = true
  
  db_name  = "frontend_router_mcp"
  username = "postgres"
  password = var.db_password
  
  vpc_security_group_ids = [aws_security_group.rds.id]
  db_subnet_group_name   = aws_db_subnet_group.production.name
  
  backup_retention_period = 7
  backup_window          = "03:00-04:00"
  maintenance_window     = "sun:04:00-sun:05:00"
  
  multi_az               = true
  publicly_accessible    = false
  
  tags = {
    Name        = "frontend-router-mcp-production"
    Environment = "production"
  }
}

# ElastiCache Redis Cluster
resource "aws_elasticache_replication_group" "production" {
  replication_group_id       = "frontend-router-mcp-prod"
  description                = "Redis cluster for Frontend Router MCP"
  
  node_type            = "cache.r6g.large"
  port                 = 6379
  parameter_group_name = "default.redis7.cluster.on"
  
  num_cache_clusters         = 3
  automatic_failover_enabled = true
  multi_az_enabled          = true
  
  subnet_group_name = aws_elasticache_subnet_group.production.name
  security_group_ids = [aws_security_group.elasticache.id]
  
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
  
  tags = {
    Name        = "frontend-router-mcp-production"
    Environment = "production"
  }
}
```

### 2. Production Kubernetes Configuration

**Production Deployment:**
```yaml
# k8s/production/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend-router-mcp
  namespace: frontend-router-mcp-production
spec:
  replicas: 10
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
  selector:
    matchLabels:
      app: frontend-router-mcp
  template:
    metadata:
      labels:
        app: frontend-router-mcp
    spec:
      containers:
      - name: frontend-router-mcp
        image: frontend-router-mcp:production
        ports:
        - containerPort: 8080
          name: websocket
        - containerPort: 8081
          name: mcp
        - containerPort: 8082
          name: metrics
        env:
        - name: WS_PORT
          value: "8080"
        - name: MCP_PORT
          value: "8081"
        - name: LOG_LEVEL
          value: "info"
        - name: REDIS_HOST
          valueFrom:
            configMapKeyRef:
              name: app-config
              key: REDIS_HOST
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: app-secrets
              key: db-password
        resources:
          requests:
            memory: "1Gi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "1000m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8082
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /ready
            port: 8082
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 5
          failureThreshold: 3
        securityContext:
          runAsNonRoot: true
          runAsUser: 1000
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
```

**Horizontal Pod Autoscaler:**
```yaml
# k8s/production/hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: frontend-router-mcp-hpa
  namespace: frontend-router-mcp-production
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: frontend-router-mcp
  minReplicas: 5
  maxReplicas: 50
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
      - type: Percent
        value: 100
        periodSeconds: 15
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Percent
        value: 10
        periodSeconds: 60
```

### 3. Production Monitoring

**Prometheus Configuration:**
```yaml
# k8s/production/monitoring.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: frontend-router-mcp-production
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
      evaluation_interval: 15s
    
    rule_files:
    - "/etc/prometheus/rules/*.yml"
    
    scrape_configs:
    - job_name: 'frontend-router-mcp'
      kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
          - frontend-router-mcp-production
      relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
        action: replace
        target_label: __metrics_path__
        regex: (.+)
      - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2
        target_label: __address__
    
    alerting:
      alertmanagers:
      - static_configs:
        - targets:
          - alertmanager:9093
```

**Alerting Rules:**
```yaml
# k8s/production/alerts.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-alerts
  namespace: frontend-router-mcp-production
data:
  alerts.yml: |
    groups:
    - name: frontend-router-mcp
      rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High error rate detected"
          description: "Error rate is {{ $value }} errors per second"
      
      - alert: HighMemoryUsage
        expr: (container_memory_usage_bytes{pod=~"frontend-router-mcp-.*"} / container_spec_memory_limit_bytes) > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage detected"
          description: "Memory usage is {{ $value | humanizePercentage }}"
      
      - alert: PodCrashLooping
        expr: rate(kube_pod_container_status_restarts_total[10m]) > 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Pod is crash looping"
          description: "Pod {{ $labels.pod }} is restarting frequently"
```

### 4. Production Deployment Pipeline

**CI/CD Pipeline (GitHub Actions):**
```yaml
# .github/workflows/production-deploy.yml
name: Production Deploy

on:
  push:
    branches: [ main ]
    tags: [ 'v*' ]

jobs:
  deploy:
    runs-on: ubuntu-latest
    if: startsWith(github.ref, 'refs/tags/')
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v3
      with:
        go-version: 1.21
    
    - name: Run tests
      run: |
        go test -v ./...
        go test -race ./...
    
    - name: Build Docker image
      run: |
        docker build -t frontend-router-mcp:${{ github.ref_name }} .
    
    - name: Configure AWS credentials
      uses: aws-actions/configure-aws-credentials@v1
      with:
        aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
        aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
        aws-region: us-east-1
    
    - name: Login to Amazon ECR
      id: login-ecr
      uses: aws-actions/amazon-ecr-login@v1
    
    - name: Push to ECR
      run: |
        docker tag frontend-router-mcp:${{ github.ref_name }} ${{ steps.login-ecr.outputs.registry }}/frontend-router-mcp:${{ github.ref_name }}
        docker push ${{ steps.login-ecr.outputs.registry }}/frontend-router-mcp:${{ github.ref_name }}
    
    - name: Update Kubernetes manifests
      run: |
        sed -i 's|image: frontend-router-mcp:.*|image: ${{ steps.login-ecr.outputs.registry }}/frontend-router-mcp:${{ github.ref_name }}|' k8s/production/deployment.yaml
    
    - name: Deploy to Kubernetes
      run: |
        aws eks update-kubeconfig --name frontend-router-mcp-production
        kubectl apply -f k8s/production/
        kubectl rollout status deployment/frontend-router-mcp -n frontend-router-mcp-production
    
    - name: Run health check
      run: |
        kubectl get pods -n frontend-router-mcp-production
        kubectl get svc -n frontend-router-mcp-production
        
        # Wait for all pods to be ready
        kubectl wait --for=condition=ready pod -l app=frontend-router-mcp -n frontend-router-mcp-production --timeout=300s
```

## Health Checks and Monitoring

### Application Health Endpoints

**Health Check Implementation:**
```go
// internal/health/health.go
package health

import (
    "encoding/json"
    "net/http"
    "time"
)

type HealthChecker struct {
    checks map[string]HealthCheck
}

type HealthCheck interface {
    Check() error
}

type HealthResponse struct {
    Status    string            `json:"status"`
    Timestamp time.Time         `json:"timestamp"`
    Checks    map[string]string `json:"checks"`
}

func (h *HealthChecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    response := HealthResponse{
        Status:    "healthy",
        Timestamp: time.Now(),
        Checks:    make(map[string]string),
    }
    
    for name, check := range h.checks {
        if err := check.Check(); err != nil {
            response.Status = "unhealthy"
            response.Checks[name] = err.Error()
        } else {
            response.Checks[name] = "ok"
        }
    }
    
    if response.Status == "unhealthy" {
        w.WriteHeader(http.StatusServiceUnavailable)
    }
    
    json.NewEncoder(w).Encode(response)
}
```

### Deployment Verification

**Post-Deployment Tests:**
```bash
#!/bin/bash
# scripts/verify-deployment.sh

set -e

NAMESPACE=${1:-frontend-router-mcp-production}
TIMEOUT=${2:-300}

echo "Verifying deployment in namespace: $NAMESPACE"

# Check if all pods are ready
echo "Checking pod readiness..."
kubectl wait --for=condition=ready pod -l app=frontend-router-mcp -n $NAMESPACE --timeout=${TIMEOUT}s

# Check if service is available
echo "Checking service availability..."
kubectl get svc -n $NAMESPACE

# Run health check
echo "Running health check..."
POD=$(kubectl get pods -n $NAMESPACE -l app=frontend-router-mcp -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n $NAMESPACE $POD -- wget -qO- http://localhost:8082/health

# Test WebSocket connection
echo "Testing WebSocket connection..."
kubectl exec -n $NAMESPACE $POD -- timeout 5 nc -z localhost 8080

# Test MCP connection
echo "Testing MCP connection..."
kubectl exec -n $NAMESPACE $POD -- timeout 5 nc -z localhost 8081

echo "Deployment verification completed successfully!"
```

This comprehensive deployment guide provides step-by-step instructions for deploying the Frontend Router MCP service across all environments, ensuring reliable and scalable deployments with proper monitoring and health checks.