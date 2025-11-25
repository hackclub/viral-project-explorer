# Production Dockerfile for SvelteKit frontend
# Multi-stage build for minimal final image

# --- Build Stage ---
FROM node:20-alpine AS builder

WORKDIR /app

# Build argument for backend URL (required at build time for SvelteKit static env)
ARG PUBLIC_BACKEND_URL
ENV PUBLIC_BACKEND_URL=$PUBLIC_BACKEND_URL

# Copy package files first for layer caching
COPY package.json package-lock.json ./

# Install all dependencies (including devDependencies for build)
RUN npm ci

# Copy the rest of the source code
COPY . .

# Build the production app
RUN npm run build

# --- Runtime Stage ---
FROM node:20-alpine

WORKDIR /app

# Copy package files for production dependencies
COPY package.json package-lock.json ./

# Install only production dependencies
RUN npm ci --omit=dev && npm cache clean --force

# Copy the built application from builder
COPY --from=builder /app/build ./build

# Change ownership to non-root node user (already exists in node:alpine)
RUN chown -R node:node /app

# Switch to non-root user
USER node

# Expose the production port
EXPOSE 3000

# Set production environment
ENV NODE_ENV=production
ENV PORT=3000
ENV HOST=0.0.0.0

# Run the SvelteKit server
CMD ["node", "build"]

