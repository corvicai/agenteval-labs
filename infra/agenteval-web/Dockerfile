# Build stage
FROM node:20-alpine AS builder

WORKDIR /app

# Copy package files
COPY frontend/package*.json ./
RUN npm install

# Copy source code
COPY frontend/. .

# Build the application
RUN npm run build

# Production stage
FROM nginx:alpine

# Install envsubst (comes with gettext)
RUN apk add --no-cache gettext

# Copy built files
COPY --from=builder /app/dist /usr/share/nginx/html

# Copy nginx config template
COPY frontend/nginx.conf.template /etc/nginx/templates/default.conf.template

# Default environment variables
ENV PORT=8080
ENV API_URL=http://go-api:8080

EXPOSE 8080

# The nginx image supports templates out of the box in newer versions, 
# but let's be explicit or use the built-in feature if available.
# Actually, nginx:alpine supports /etc/nginx/templates/*.template since 1.19
CMD ["nginx", "-g", "daemon off;"]
