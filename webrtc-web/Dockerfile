# Stage 1: Build the Next.js static assets
FROM node:20-alpine AS builder

WORKDIR /app

# Copy package files
COPY package.json package-lock.json ./

# Install dependencies
RUN npm ci

# Copy the rest of the application
COPY . .

# Build the Next.js application (static export)
RUN npm run build


FROM docker.io/library/caddy:latest AS prod-caddy

# Copy static assets from builder stage
COPY --from=builder /app/out /usr/share/nginx/html


FROM docker.io/library/nginx:alpine AS prod-nginx

# Copy static assets from builder stage
COPY --from=builder /app/out /usr/share/nginx/html
