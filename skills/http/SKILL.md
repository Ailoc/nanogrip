---
description: HTTP client operations - GET, POST, PUT, DELETE requests, headers, authentication, JSON handling.
metadata:
    nanogrip:
        requires:
            bins:
                - curl
name: http
---

# HTTP Skill

Use curl for HTTP requests. This skill covers common API operations.

## Basic Requests

### GET request
```bash
curl https://api.example.com/data
curl -s https://api.example.com/data  # silent
curl -i https://api.example.com/data  # include headers
```

### POST request
```bash
curl -X POST https://api.example.com/data
curl -X POST -d 'key=value' https://api.example.com/data
curl -X POST --data '{"key":"value"}' https://api.example.com/data
```

### PUT request
```bash
curl -X PUT -d 'key=value' https://api.example.com/data/1
```

### DELETE request
```bash
curl -X DELETE https://api.example.com/data/1
```

## Headers

### Custom headers
```bash
curl -H "Content-Type: application/json" https://api.example.com
curl -H "Authorization: Bearer TOKEN" https://api.example.com
curl -H "Accept: application/json" https://api.example.com
```

### Multiple headers
```bash
curl -H "Content-Type: application/json" \
     -H "Authorization: Bearer TOKEN" \
     https://api.example.com
```

## JSON Operations

### Send JSON
```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"name":"value","age":25}' \
  https://api.example.com/users
```

### Pretty print JSON response
```bash
curl -s https://api.example.com/data | jq '.'
curl -s https://api.example.com/data | python -m json.tool
```

### Extract JSON fields
```bash
curl -s https://api.example.com/data | jq '.data[0].name'
curl -s https://api.example.com/data | jq -r '.token'
```

## Authentication

### Basic Auth
```bash
curl -u username:password https://api.example.com

# Base64 encoded
curl -H "Authorization: Basic BASE64_CREDENTIALS" https://api.example.com
```

### Bearer Token
```bash
curl -H "Authorization: Bearer YOUR_TOKEN" https://api.example.com
```

### API Key
```bash
curl -H "X-API-Key: YOUR_KEY" https://api.example.com
```

## Query Parameters

### URL parameters
```bash
curl "https://api.example.com/search?q=keyword&limit=10"
curl "https://api.example.com/users?page=1&size=20"
```

### Build query string
```bash
# Use --data-urlencode for proper encoding
curl -G --data-urlencode "q=hello world" \
     --data-urlencode "page=1" \
     https://api.example.com/search
```

## Forms

### Form data
```bash
curl -X POST -d "username=user&password=pass" https://example.com/login

# Multipart form
curl -F "file=@/path/to/file.txt" https://example.com/upload
curl -F "file=@/path/to/image.png" -F "name=test" https://example.com/upload
```

## File Operations

### Download file
```bash
curl -O https://example.com/file.zip
curl -o custom-name.zip https://example.com/file.zip

# Download with progress
curl -# -O https://example.com/large-file.zip
```

### Upload file
```bash
curl -X POST -F "file=@/path/to/file.txt" https://api.example.com/upload
```

## Response Handling

### Save response to file
```bash
curl -s https://api.example.com/data -o response.json
```

### Show only headers
```bash
curl -I https://example.com
curl -s -I https://example.com
```

### Show HTTP status
```bash
curl -w "%{http_code}" -s -o /dev/null https://example.com
curl -w "\n%{http_code}\n" -s https://example.com
```

### Verbose for debugging
```bash
curl -v https://example.com
curl --verbose https://example.com
```

## Common Examples

### GitHub API
```bash
# Get user info
curl -s https://api.github.com/users/username

# Create issue
curl -X POST -H "Authorization: Bearer TOKEN" \
     -H "Accept: application/vnd.github.v3+json" \
     -d '{"title":"Bug","body":"Description"}' \
     https://api.github.com/repos/owner/repo/issues
```

### JSONPlaceholder (testing)
```bash
# Get posts
curl -s https://jsonplaceholder.typicode.com/posts

# Create post
curl -X POST -H "Content-Type: application/json" \
     -d '{"title":"Test","body":"Content","userId":1}' \
     https://jsonplaceholder.typicode.com/posts
```

## Error Handling

### Follow redirects
```bash
curl -L https://example.com/redirect
```

### Timeout
```bash
curl --max-time 10 https://example.com
curl --connect-timeout 5 https://example.com
```

### Retry on failure
```bash
curl --retry 3 --retry-delay 5 https://example.com
```
