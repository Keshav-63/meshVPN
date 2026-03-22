#!/bin/bash
# Test Analytics System
# This script deploys an app, generates traffic, and checks analytics

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo "=========================================="
echo "MeshVPN Analytics Test"
echo "=========================================="
echo ""

# Step 1: Deploy test app
echo -e "${YELLOW}[1/5] Deploying test app...${NC}"
RESPONSE=$(curl -s -X POST http://localhost:8080/deploy \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "https://github.com/dockersamples/static-site",
    "package": "small",
    "subdomain": "analytics-test-'"$(date +%s)"'"
  }')

DEPLOYMENT_ID=$(echo $RESPONSE | jq -r '.deployment_id')

if [ -z "$DEPLOYMENT_ID" ] || [ "$DEPLOYMENT_ID" = "null" ]; then
    echo -e "${RED}Failed to deploy app!${NC}"
    echo "$RESPONSE" | jq
    exit 1
fi

echo -e "${GREEN}✓ Deployed: $DEPLOYMENT_ID${NC}"

# Step 2: Wait for deployment to complete
echo ""
echo -e "${YELLOW}[2/5] Waiting for deployment to complete...${NC}"
for i in {1..30}; do
    STATUS=$(curl -s http://localhost:8080/deployments | jq -r ".deployments[] | select(.deployment_id==\"$DEPLOYMENT_ID\") | .status")
    echo -n "."

    if [ "$STATUS" = "running" ]; then
        echo ""
        echo -e "${GREEN}✓ Deployment running${NC}"
        break
    fi

    if [ "$STATUS" = "failed" ]; then
        echo ""
        echo -e "${RED}Deployment failed!${NC}"
        exit 1
    fi

    sleep 2
done

# Step 3: Generate traffic
echo ""
echo -e "${YELLOW}[3/5] Generating traffic (50 requests)...${NC}"
SUBDOMAIN=$(curl -s http://localhost:8080/deployments | jq -r ".deployments[] | select(.deployment_id==\"$DEPLOYMENT_ID\") | .subdomain")

for i in {1..50}; do
    curl -s "http://localhost/$SUBDOMAIN" > /dev/null 2>&1 || true
    echo -n "."
    sleep 0.1
done

echo ""
echo -e "${GREEN}✓ Traffic generated${NC}"

# Step 4: Wait for collector to run
echo ""
echo -e "${YELLOW}[4/5] Waiting for analytics collector (60s)...${NC}"
echo "Analytics collector runs every 1 minute. Waiting..."
sleep 65

# Step 5: Check analytics
echo ""
echo -e "${YELLOW}[5/5] Checking analytics...${NC}"
ANALYTICS=$(curl -s "http://localhost:8080/deployments/$DEPLOYMENT_ID/analytics")

echo ""
echo "=========================================="
echo -e "${GREEN}Analytics Results:${NC}"
echo "=========================================="
echo ""

echo "$ANALYTICS" | jq '.metrics'

# Verify analytics have data
REQUEST_COUNT=$(echo "$ANALYTICS" | jq -r '.metrics.request_count_total')

if [ "$REQUEST_COUNT" = "null" ] || [ "$REQUEST_COUNT" = "0" ]; then
    echo ""
    echo -e "${YELLOW}Warning: No request data yet. This might be normal if:${NC}"
    echo "  - Analytics collector hasn't run yet (wait another minute)"
    echo "  - Prometheus isn't scraping properly"
    echo "  - The app isn't receiving traffic yet"
    echo ""
    echo "Retry in 1-2 minutes:"
    echo "  curl http://localhost:8080/deployments/$DEPLOYMENT_ID/analytics | jq '.metrics'"
else
    echo ""
    echo -e "${GREEN}✓ Analytics working!${NC}"
    echo ""
    echo "View live stream:"
    echo "  curl -N http://localhost:8080/deployments/$DEPLOYMENT_ID/analytics/stream"
fi

echo ""
echo "=========================================="
echo "Deployment ID: $DEPLOYMENT_ID"
echo "Subdomain: $SUBDOMAIN"
echo "=========================================="
