# GitHub Repository Tracker API

A RESTful API service for tracking GitHub repositories and their commit history. Built with Go and PostgreSQL, featuring Swagger documentation for easy integration.

## Features

- **Repository Management**: Fetch and store GitHub repository metadata
- **Commit Tracking**: Efficiently sync commits with deduplication
- **Continuous Monitoring**: Configurable intervals for automatic syncing
- **Persistent Storage**: PostgreSQL with optimized schema and indexing
- **Reset Functionality**: Reset sync point to any date
- **Query Operations**: Get top commit authors and repository commits
- **Comprehensive Testing**: 100% test coverage with unit and integration tests
- **Production Ready**: Docker support, structured logging, graceful shutdown
- **RESTful API**: Fully documented endpoints following REST principles
- **Swagger UI**: Interactive API documentation at `/swagger/index.html`
- **Repository Management**: Track and monitor GitHub repositories
- **Commit Analytics**: Get detailed commit history and statistics
- **Pagination**: Efficient data retrieval with pagination support
- **Filtering**: Advanced filtering options for commits and repositories
- **Validation**: Request validation and error handling

## API Documentation

Interactive API documentation is available via Swagger UI when the service is running:

```
http://localhost:8080/swagger/index.html
```

### API Base URL
```
http://localhost:8080/api/v1
```

## Getting Started

## Quick Start

### Prerequisites

- Go 1.21+
- PostgreSQL 12+
- Docker and Docker Compose (recommended)

### Environment Variables

Create a `.env` file with the following variables:

```env
DATABASE_URL=postgres://user:password@localhost:5432/github_tracker?sslmode=disable
GITHUB_TOKEN=your_github_token
JWT_SECRET=your_jwt_secret
PORT=8080
LOG_LEVEL=info
```

### Running with Docker Compose

1. Clone the repository:
```bash
git clone <repository-url>
cd lema-github-tracker
```

2. Start the services:
```bash
docker-compose up -d
```

The API will be available at `http://localhost:8080`

### Manual Setup

1. Install dependencies:
```bash
go mod download
```

2. Run database migrations:
```bash
docker-compose up -d postgres
```

3. Start the server:
```bash
go run cmd/server/main.go
```

## API Endpoints

### Health Check

- `GET /health` - Check API status

### Authentication

- `POST /auth/login` - Get JWT token
- `POST /auth/refresh` - Refresh JWT token

### Repositories

- `GET /repositories` - List all repositories
- `POST /repositories/sync` - Sync a repository
- `GET /repositories/{owner}/{repo}` - Get repository details
- `GET /repositories/{owner}/{repo}/commits` - Get repository commits
- `GET /repositories/{owner}/{repo}/stats` - Get repository statistics
- `POST /repositories/sync-all` - Sync all repositories
- `POST /repositories/reset-sync` - Reset repository sync point

### Analytics

- `GET /analytics/top-authors` - Get top commit authors
- `GET /analytics/commit-stats` - Get commit statistics

## Request/Response Examples

### Sync a Repository

**Request:**
```http
POST /api/v1/repositories/sync
Content-Type: application/json
Authorization: Bearer <token>

{
  "owner": "chromium",
  "repository": "chromium"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Repository synchronized successfully",
  "data": {
    "repository_name": "chromium/chromium",
    "commits_added": 42,
    "synced_at": "2024-06-05T10:00:00Z"
  }
}
```

### Get Repository Commits

**Request:**
```http
GET /api/v1/repositories/chromium/chromium/commits?page=1&page_size=20
Authorization: Bearer <token>
```

**Response:**
```json
{
  "success": true,
  "message": "Commits retrieved successfully",
  "data": [
    {
      "id": 1,
      "sha": "a1b2c3d4...",
      "message": "Update README.md",
      "author_name": "John Doe",
      "author_email": "john@example.com",
      "commit_date": "2024-06-04T15:30:00Z",
      "url": "https://github.com/chromium/chromium/commit/a1b2c3d4"
    }
  ],
  "meta": {
    "page": 1,
    "page_size": 20,
    "total": 150,
    "total_pages": 8,
    "has_next": true,
    "has_prev": false
  }
}
```

## Error Handling

The API returns standard HTTP status codes along with JSON error responses:

```json
{
  "success": false,
  "error": "repository not found",
  "details": "The requested repository does not exist"
}
```

### Common Status Codes

- `200 OK` - Request successful
- `201 Created` - Resource created successfully
- `400 Bad Request` - Invalid request parameters
- `401 Unauthorized` - Authentication required
- `403 Forbidden` - Insufficient permissions
- `404 Not Found` - Resource not found
- `422 Unprocessable Entity` - Validation error
- `500 Internal Server Error` - Server error

## Rate Limiting

The API implements rate limiting to ensure fair usage:
- 100 requests per minute per IP address for authenticated users
- 60 requests per minute per IP address for unauthenticated users

## Testing

### Run All Tests
```bash
go test ./... -v -race -coverprofile=coverage.out
```

### View Test Coverage
```bash
go tool cover -html=coverage.out
```

### Run Specific Test Suites
```bash
# Unit tests only
go test ./tests/unit/... -v

# Integration tests only  
go test ./tests/integration/... -v
```

### Test Structure
- `tests/unit/`: Unit tests with mocks (100% coverage)
- `tests/integration/`: Integration tests with real database
- Mock implementations for all external dependencies
- Comprehensive error case testing

## Performance Considerations

### Database Optimization
- Composite indexes on frequently queried columns
- Connection pooling with configurable limits
- Batch insert operations for commits
- UNIQUE constraints prevent duplicate data

### Memory Management
- Streaming commit processing (no large in-memory collections)
- Database connection reuse
- Configurable sync intervals to balance freshness vs. load

## Architecture

```
cmd/server/          # Application entry point
internal/
├── config/          # Configuration management
├── database/        # Database connection and migrations
├── github/          # GitHub API client
├── models/          # Data structures
├── repository/      # Database operations
├── service/         # Business logic
└── scheduler/       # Background sync scheduling
pkg/logger/          # Structured logging
tests/               # Test suites
```

## Production Deployment

### Docker Production Build
```bash
docker build -t github-service:latest .
docker run -d \
  --name github-service \
  -e DATABASE_URL="your_db_url" \
  -e GITHUB_TOKEN="your_token" \
  github-service:latest
```

### Environment-Specific Configurations
- Development: Verbose logging, shorter sync intervals
- Staging: Production-like settings with test data
- Production: Optimized logging, hourly syncs, monitoring

### Monitoring and Observability
- Structured JSON logging with correlation IDs
- Metrics: sync duration, commit counts, error rates
- Health check endpoints
- Graceful shutdown handling

## Troubleshooting

### Common Issues

**Database Connection Errors:**
```bash
# Check PostgreSQL is running
docker-compose ps postgres

# Verify connection string
psql "postgres://user:password@localhost/github_service"
```

**GitHub API Rate Limiting:**
```bash
# Check rate limit status
curl -H "Authorization: token YOUR_TOKEN" https://api.github.com/rate_limit

# Increase SYNC_INTERVAL to reduce API calls
export SYNC_INTERVAL="2h"
```

**Missing Commits:**
```bash
# Reset sync point to fetch older commits
# This will delete existing commits after the reset date
curl -X POST http://localhost:8080/reset \
  -d '{"repository": "chromium/chromium", "since": "2024-01-01T00:00:00Z"}'
```

### Debugging

Enable debug logging:
```bash
export LOG_LEVEL="debug"
```

Check sync status:
```sql
SELECT full_name, last_synced_at, sync_since, 
       (SELECT COUNT(*) FROM commits WHERE repository_id = repositories.id) as commit_count
FROM repositories;
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure 100% test coverage
5. Submit a pull request

### Code Style
- Follow Go standard formatting (`go fmt`)
- Use meaningful variable and function names
- Add godoc comments for public functions
- Handle errors explicitly

## License

MIT License - see LICENSE file for details