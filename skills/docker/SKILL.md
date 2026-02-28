---
description: Docker container operations - build, run, stop, logs, exec, ps, images, volumes, networks, etc.
metadata:
    nanogrip:
        requires:
            bins:
                - docker
name: docker
---

# Docker Skill

Use docker for container operations. Most commands require docker daemon running.

## Container Operations

### Run containers
```bash
# Basic run
docker run nginx                    # run and remove
docker run -d nginx               # detached
docker run -p 8080:80 nginx       # port mapping
docker run -v /host:/container nginx  # volume mount
docker run -e VAR=value nginx      # environment variable
docker run --name mynginx nginx   # name container
docker run --rm nginx             # auto-remove on exit
docker run -it ubuntu bash        # interactive
```

### List containers
```bash
docker ps                         # running only
docker ps -a                    # all containers
docker ps -q                    # quiet (IDs only)
docker ps -l                    # latest container
```

### Stop/Start/Remove
```bash
docker stop container-id
docker start container-id
docker restart container-id
docker rm container-id           # remove
docker rm -f container-id       # force remove
docker rm $(docker ps -aq)      # remove all
```

### Container info
```bash
docker logs container-id
docker logs -f container-id     # follow
docker logs --tail 100 container-id
docker inspect container-id
docker stats container-id      # real-time stats
docker top container-id        # processes
docker exec -it container-id bash  # shell access
```

## Image Operations

### List images
```bash
docker images
docker images -a
docker images -q
```

### Pull/Remove images
```bash
docker pull nginx:latest
docker rmi image-id
docker rmi $(docker images -q)  # remove all
```

### Build images
```bash
docker build -t myapp:latest .
docker build -t myapp:latest -f Dockerfile.dev .
docker build --no-cache -t myapp:latest .
```

### Run image
```bash
docker run -d myapp:latest
docker run -d -p 3000:3000 myapp:latest
```

## Volume Operations

### List volumes
```bash
docker volume ls
docker volume ls -f dangling=true
```

### Create/Remove volumes
```bash
docker volume create myvolume
docker volume rm myvolume
docker volume prune  # remove unused
```

### Inspect volumes
```bash
docker volume inspect myvolume
```

## Network Operations

### List networks
```bash
docker network ls
```

### Create network
```bash
docker network create mynetwork
docker network create --driver bridge mynetwork
```

### Inspect network
```bash
docker network inspect mynetwork
```

### Connect/Disconnect
```bash
docker network connect mynetwork container
docker network disconnect mynetwork container
```

## Docker Compose

### Basic commands
```bash
docker-compose up -d
docker-compose down
docker-compose ps
docker-compose logs -f
docker-compose restart
```

### Build and start
```bash
docker-compose build
docker-compose up --build
docker-compose up -d --scale app=3
```

### Specific service
```bash
docker-compose up -d postgres
docker-compose logs -f postgres
docker-compose exec postgres psql
```

## System Operations

### Docker system info
```bash
docker info
docker version
docker system df
docker system prune
docker system prune -a  # remove all unused
```

### Clean up
```bash
docker container prune
docker image prune
docker volume prune
docker network prune
```
