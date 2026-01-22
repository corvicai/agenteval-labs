#!/bin/bash

./reset -soft-reset && docker compose -f docker-compose.proxy.yml down && docker compose -f docker-compose.proxy.yml up -d