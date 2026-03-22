#!/bin/bash
echo "========================================"
echo "MeshVPN Analytics Verification"
echo "========================================"
echo ""

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0;33m'

echo -n "1. Control-plane... "
curl -s http://localhost:8080/health > /dev/null 2>&1 && echo -e "${GREEN}✓${NC}" || echo -e "${RED}✗${NC}"

echo -n "2. Prometheus... "
curl -s http://localhost:9090/-/ready > /dev/null 2>&1 && echo -e "${GREEN}✓${NC}" || echo -e "${RED}✗${NC}"

echo -n "3. Grafana... "
curl -s http://localhost:3001/api/health > /dev/null 2>&1 && echo -e "${GREEN}✓${NC}" || echo -e "${RED}✗${NC}"

echo ""
echo "Platform Metrics:"
curl -s http://localhost:8080/metrics | grep -E "^platform_" | head -5

echo ""
echo "Access URLs:"
echo "  Prometheus: http://localhost:9090"
echo "  Grafana:    http://localhost:3001"
echo "  Platform:   http://localhost:8080/platform/analytics"
