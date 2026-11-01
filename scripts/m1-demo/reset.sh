#!/bin/bash
# Resets the loopback M1 devspace demo (only its own process on :8098, its DB and dirs).
cd /workspace/projects/shippin-console
pkill -f "^./bin/catalog-demo"; pkill -f "code-server.*\.run/devspace"; sleep 1
rm -rf .run/devspace /tmp/m1-profile screenshots/m1/*
psql -q -h 127.0.0.1 -p 55433 -U postgres -c "drop database shippin_demo" -c "create database shippin_demo"
CATALOG_DEMO_LISTEN=127.0.0.1:8098 CATALOG_DEMO_DATABASE_URL="postgres://postgres@127.0.0.1:55433/shippin_demo?sslmode=disable" \
 CATALOG_DEMO_SEED=config/demo/seed.sql CATALOG_DEMO_IDENTITY=config/demo/identity-devspace.json \
 CATALOG_DEMO_ENGINE=/workspace/tools/code-server/current/bin/code-server nohup ./bin/catalog-demo > .run/demo.log 2>&1 &
sleep 2
