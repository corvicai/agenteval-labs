# Build stage
FROM node:20-alpine AS builder
ARG VITE_GIT_COMMIT=unknown
ENV VITE_GIT_COMMIT=$VITE_GIT_COMMIT

WORKDIR /app

# Copy package files
COPY package*.json ./
RUN npm install

# Copy source code
COPY . .

# Build the application
RUN npm run build

# Production stage
FROM nginx:alpine
ARG VITE_GIT_COMMIT=unknown
ENV VITE_GIT_COMMIT=$VITE_GIT_COMMIT

# Install envsubst (comes with gettext)
RUN apk add --no-cache gettext

# Copy built files
COPY --from=builder /app/dist /usr/share/nginx/html

# Copy nginx config template
COPY nginx.conf.template /etc/nginx/templates/default.conf.template

# Copy entrypoint script
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

EXPOSE 8080

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
