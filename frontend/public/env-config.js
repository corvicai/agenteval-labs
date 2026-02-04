// This file is used for runtime environment variables injection.
// During local development, this file will have an empty object.
// In production, the docker-entrypoint.sh script will populate this file 
// with actual environment variables starting with VITE_.
window._env_ = {};
