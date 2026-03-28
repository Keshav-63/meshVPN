# Swagger API Documentation

## Overview

The MeshVPN Control Plane API now includes interactive Swagger documentation, similar to FastAPI's automatic documentation.

## Accessing Swagger UI

Once the control plane is running, access the Swagger UI at:

```
http://localhost:8080/swagger/index.html
```

## Features

### Interactive API Documentation
- **Try it out**: Test API endpoints directly from the browser
- **Request/Response schemas**: View detailed data structures
- **Authentication**: Test authenticated endpoints with Bearer tokens
- **Examples**: See example requests and responses

### Available Endpoints

#### System
- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics

#### Authentication
- `GET /auth/whoami` - Get current user information

#### Deployments
- `POST /deploy` - Deploy a new application
- `GET /deployments` - List all deployments
- `GET /deployments/{id}/build-logs` - Get build logs
- `GET /deployments/{id}/app-logs` - Get application runtime logs

## Authentication

To test authenticated endpoints:

1. Click the **Authorize** button in Swagger UI
2. Enter your JWT token in the format: `Bearer YOUR_JWT_TOKEN`
3. Click **Authorize** and then **Close**

Now all requests will include your authentication token.

## Regenerating Documentation

If you modify API endpoints, regenerate the Swagger docs:

```bash
cd control-plane
swag init -g cmd/control-plane/main.go -o docs --parseDependency --parseInternal
```

## OpenAPI Specification

The raw OpenAPI specification files are available at:
- JSON: `http://localhost:8080/swagger/doc.json`
- YAML: Available in `docs/swagger.yaml`

## Adding New Endpoints

To add Swagger annotations to a new endpoint:

```go
// @Summary      Short description
// @Description  Detailed description
// @Tags         Category
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name  path      type    required  "Description"
// @Param        body  body      Model   true      "Description"
// @Success      200   {object}  ResponseModel
// @Failure      400   {object}  ErrorResponse
// @Router       /endpoint [method]
func HandlerFunction(c *gin.Context) {
    // handler code
}
```

After adding annotations, regenerate the docs using the command above.
