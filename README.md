# Go Multi-Tenant System

A scalable multi-tenant microservices architecture built with Go, featuring real-time location tracking and data streaming capabilities with enterprise-grade resilience patterns.

## Architecture Overview

This system consists of the following microservices:

- **API Gateway**: Central entry point with JWT authentication
- **Auth Service**: User authentication using AWS Cognito with custom attributes
- **Tenant Service**: Multi-tenant management and data isolation
- **Location Service**: Real-time location data collection and processing
- **Streaming Service**: Data streaming to third-party systems via Kafka


## Technology Stack

- **Backend**: Go (Gin framework)
- **Database**: PostgreSQL with multi-tenant schema
- **Authentication**: AWS Cognito with JWT tokens
- **Cache**: Redis (JWT token caching)
- **Message Broker**: Apache Kafka
- **Containerization**: Docker & Docker Compose
- **API Gateway**: Custom implementation with JWT verification

## Quick Start

1. **Clone the repository**:
   ```bash
   git clone https://github.com/pavitra93/go-multi-tenant-system.git
   cd go-multi-tenant-system
   ```

2. **Set up environment variables**:
   ```bash
   cp env.example .env
   # Edit .env with your AWS Cognito and database credentials
   ```

3. **Start the services**:
   ```bash
   docker-compose up -d
   ```

4. **Access the API**:
   - API Gateway: http://localhost:8080
   - Individual services: http://localhost:8001-8004

## API Endpoints

### Authentication
- `POST /auth/login` - User login
- `POST /auth/register` - User registration
- `POST /auth/refresh` - Refresh JWT token

### Tenant Management
- `GET /tenants` - List tenants (admin only)
- `POST /tenants` - Create new tenant (admin only)
- `GET /tenants/{id}` - Get tenant details
- `PUT /tenants/{id}` - Update tenant

### Location Tracking
- `POST /location/session/start` - Start location tracking session
- `POST /location/update` - Submit location data
- `GET /location/session/{id}` - Get session data
- `POST /location/session/{id}/stop` - Stop tracking session

## Multi-Tenant Data Isolation

The system ensures complete data isolation between tenants using:

- **Tenant ID**: Every database record includes a `tenant_id` field
- **JWT Claims**: Tenant information embedded in authentication tokens
- **Service-Level Filtering**: All queries filtered by tenant ID

## Development

### Project Structure
```
├── services/
│   ├── auth/          # Authentication service
│   ├── tenant/        # Tenant management service
│   ├── location/      # Location tracking service
│   └── streaming/     # Data streaming service
├── gateway/           # API Gateway
├── shared/            # Shared utilities and models
├── docker-compose.yml # Service orchestration
└── README.md
```

### Running Individual Services

```bash
# Auth Service
cd services/auth
go run main.go

# Tenant Service
cd services/tenant
go run main.go

# Location Service
cd services/location
go run main.go

# Streaming Service
cd services/streaming
go run main.go

# API Gateway
cd gateway
go run main.go
```

## Configuration

### AWS Cognito Setup

**📚 Detailed Setup Guide**: See [docs/COGNITO_SETUP.md](docs/COGNITO_SETUP.md)

Quick setup:
1. Create a User Pool in AWS Cognito
2. Add custom attributes: `custom:tenant_id` and `custom:role`
3. Enable `ALLOW_USER_PASSWORD_AUTH` authentication flow
4. Configure the User Pool ID and Client ID in your `.env` file
5. Set up appropriate token expiration policies

### Database Schema

The PostgreSQL database uses a shared schema with tenant isolation:

- All tables include a `tenant_id` column
- Row-level security policies enforce tenant boundaries
- Indexes optimize multi-tenant queries

### Kafka Topics

- `location-updates`: Real-time location data
- `session-events`: Session start/stop events
- `tenant-events`: Tenant management events

## Monitoring and Logging

- Structured logging with logrus
- Health check endpoints for each service
- Metrics collection for performance monitoring

## Resilience & CAP Theorem

This system prioritizes **CP** (Consistency + Partition Tolerance) with improved **Availability**:

- ✅ **Consistency**: Saga pattern ensures no orphaned users across systems
- ✅ **Availability**: Circuit breakers maintain service availability during Cognito failures
- ✅ **Partition Tolerance**: Graceful degradation during network partitions

**Trade-offs**: Write operations may be slower due to distributed transaction handling, but data consistency is guaranteed.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

This project is licensed under the MIT License.
