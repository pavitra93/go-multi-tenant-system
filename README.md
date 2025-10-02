# Go Multi-Tenant System

A scalable multi-tenant microservices architecture built with Go, featuring real-time location tracking and data streaming capabilities.

## Architecture

- **API Gateway**: Central entry point with authentication
- **Auth Service**: User authentication using AWS Cognito
- **Tenant Service**: Multi-tenant management and data isolation
- **Location Service**: Real-time location data collection and processing
- **Streaming Service**: Data streaming to third-party systems via Kafka

## Technology Stack

- **Backend**: Go (Gin framework)
- **Database**: PostgreSQL with multi-tenant schema
- **Authentication**: AWS Cognito with Redis session management
- **Cache**: Redis for session caching
- **Message Broker**: Apache Kafka
- **Containerization**: Docker & Docker Compose

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
- `POST /auth/logout` - User logout
- `GET /auth/sessions` - Get user sessions

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

- **Tenant ID**: Every database record includes a `tenant_id` field
- **Row-Level Security**: Database-level isolation using PostgreSQL RLS
- **Redis Sessions**: User profiles cached with tenant context

## Development

### Prerequisites
- Go 1.21+
- Docker and Docker Compose
- AWS Cognito User Pool
- PostgreSQL database
- Apache Kafka

### Local Development
1. Set up AWS Cognito User Pool
2. Configure environment variables in `.env`
3. Start services: `docker-compose up -d`

## License

MIT License