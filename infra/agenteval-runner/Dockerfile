FROM python:3.11-slim

WORKDIR /app

# Install dependencies
RUN apt-get update && apt-get install -y \
    curl \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y nodejs \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

COPY server_python/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy source code
COPY server_python/server.py .

# Expose port (default 3003, but Cloud Run uses $PORT)
EXPOSE 3003

# Run the application
# We use a shell to allow environment variable expansion if needed, 
# but server.py should be updated to read PORT environment variable.
CMD ["python", "server.py"]
