#!/bin/bash

# MeshVPN End-to-End Testing Script
# This script performs comprehensive testing of all Phase 2 features

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
BASE_URL="${BASE_URL:-http://localhost:8080}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
DEPLOYMENT_ID=""

# Counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
print_header() {
    echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
}

print_test() {
    echo -e "${YELLOW}[TEST]${NC} $1"
    TESTS_RUN=$((TESTS_RUN + 1))
}

print_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

print_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

# Check if control-plane is running
check_control_plane() {
    print_header "Checking Prerequisites"

    print_test "Control-plane health check"
    if curl -s "${BASE_URL}/health" | grep -q "LaptopCloud running"; then
        print_pass "Control-plane is running"
    else
        print_fail "Control-plane is not responding"
        exit 1
    fi
}

# Test 1: Health & Metrics
test_health_metrics() {
    print_header "Test 1: Health & Metrics"

    print_test "GET /health"
    RESPONSE=$(curl -s "${BASE_URL}/health")
    if echo "$RESPONSE" | grep -q "status"; then
        print_pass "Health endpoint returns valid response"
    else
        print_fail "Health endpoint failed"
    fi

    print_test "GET /metrics"
    RESPONSE=$(curl -s "${BASE_URL}/metrics")
    if echo "$RESPONSE" | grep -q "deployment_queue_length"; then
        print_pass "Metrics endpoint returns Prometheus format"
    else
        print_fail "Metrics endpoint failed"
    fi
}

# Test 2: Deploy with Small Package
test_deploy_small() {
    print_header "Test 2: Deploy Small Package (Auto-Subdomain)"

    print_test "POST /deploy (small package)"
    RESPONSE=$(curl -s -X POST "${BASE_URL}/deploy" \
        -H "Content-Type: application/json" \
        -d '{
            "repo": "https://github.com/vercel/next.js",
            "package": "small",
            "port": 3000
        }')

    # Extract deployment_id
    DEPLOYMENT_ID=$(echo "$RESPONSE" | grep -o '"deployment_id":"[^"]*"' | cut -d'"' -f4)

    if [ -n "$DEPLOYMENT_ID" ]; then
        print_pass "Deployment created: $DEPLOYMENT_ID"
        export DEPLOYMENT_ID
    else
        print_fail "Failed to create deployment"
        return
    fi

    # Check package
    if echo "$RESPONSE" | grep -q '"package":"small"'; then
        print_pass "Package set to small"
    else
        print_fail "Package not set correctly"
    fi

    # Check CPU/Memory
    if echo "$RESPONSE" | grep -q '"cpu_cores":0.5'; then
        print_pass "CPU cores set to 0.5"
    else
        print_fail "CPU cores incorrect"
    fi

    if echo "$RESPONSE" | grep -q '"memory_mb":512'; then
        print_pass "Memory set to 512 MB"
    else
        print_fail "Memory incorrect"
    fi

    # Check autoscaling
    if echo "$RESPONSE" | grep -q '"autoscaling_enabled":false'; then
        print_pass "Autoscaling disabled for non-subscriber"
    else
        print_fail "Autoscaling should be disabled"
    fi

    # Check subdomain
    if echo "$RESPONSE" | grep -q '"subdomain"'; then
        SUBDOMAIN=$(echo "$RESPONSE" | grep -o '"subdomain":"[^"]*"' | cut -d'"' -f4)
        print_pass "Subdomain auto-generated: $SUBDOMAIN"
    else
        print_fail "Subdomain not generated"
    fi
}

# Test 3: Deploy with Medium Package
test_deploy_medium() {
    print_header "Test 3: Deploy Medium Package (Custom Subdomain)"

    print_test "POST /deploy (medium package)"
    RESPONSE=$(curl -s -X POST "${BASE_URL}/deploy" \
        -H "Content-Type: application/json" \
        -d '{
            "repo": "https://github.com/nodejs/node",
            "package": "medium",
            "port": 8080,
            "subdomain": "node-app-test"
        }')

    if echo "$RESPONSE" | grep -q '"package":"medium"'; then
        print_pass "Package set to medium"
    else
        print_fail "Package not set correctly"
    fi

    if echo "$RESPONSE" | grep -q '"cpu_cores":1'; then
        print_pass "CPU cores set to 1.0"
    else
        print_fail "CPU cores incorrect"
    fi

    if echo "$RESPONSE" | grep -q '"memory_mb":1024'; then
        print_pass "Memory set to 1024 MB"
    else
        print_fail "Memory incorrect"
    fi

    if echo "$RESPONSE" | grep -q '"subdomain":"node-app-test"'; then
        print_pass "Custom subdomain used"
    else
        print_fail "Custom subdomain not set"
    fi
}

# Test 4: Deploy with Large Package
test_deploy_large() {
    print_header "Test 4: Deploy Large Package"

    print_test "POST /deploy (large package)"
    RESPONSE=$(curl -s -X POST "${BASE_URL}/deploy" \
        -H "Content-Type: application/json" \
        -d '{
            "repo": "https://github.com/python/cpython",
            "package": "large",
            "port": 5000
        }')

    if echo "$RESPONSE" | grep -q '"package":"large"'; then
        print_pass "Package set to large"
    else
        print_fail "Package not set correctly"
    fi

    if echo "$RESPONSE" | grep -q '"cpu_cores":2'; then
        print_pass "CPU cores set to 2.0"
    else
        print_fail "CPU cores incorrect"
    fi

    if echo "$RESPONSE" | grep -q '"memory_mb":2048'; then
        print_pass "Memory set to 2048 MB"
    else
        print_fail "Memory incorrect"
    fi
}

# Test 5: Invalid Package
test_invalid_package() {
    print_header "Test 5: Invalid Package Validation"

    print_test "POST /deploy (invalid package)"
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/deploy" \
        -H "Content-Type: application/json" \
        -d '{
            "repo": "https://github.com/test/test",
            "package": "xlarge",
            "port": 3000
        }')

    if [ "$HTTP_CODE" = "400" ]; then
        print_pass "Invalid package rejected with 400"
    else
        print_fail "Invalid package should return 400, got $HTTP_CODE"
    fi
}

# Test 6: Default Package
test_default_package() {
    print_header "Test 6: Default Package (No Package Specified)"

    print_test "POST /deploy (no package)"
    RESPONSE=$(curl -s -X POST "${BASE_URL}/deploy" \
        -H "Content-Type: application/json" \
        -d '{
            "repo": "https://github.com/test/default",
            "port": 3000
        }')

    if echo "$RESPONSE" | grep -q '"package":"small"'; then
        print_pass "Defaults to small package"
    else
        print_fail "Should default to small package"
    fi
}

# Test 7: Subdomain Conflict
test_subdomain_conflict() {
    print_header "Test 7: Subdomain Conflict Detection"

    # First deployment
    print_test "POST /deploy (unique subdomain)"
    RESPONSE=$(curl -s -X POST "${BASE_URL}/deploy" \
        -H "Content-Type: application/json" \
        -d '{
            "repo": "https://github.com/test/app1",
            "subdomain": "conflict-test",
            "package": "small",
            "port": 3000
        }')

    if echo "$RESPONSE" | grep -q '"subdomain":"conflict-test"'; then
        print_pass "First deployment with subdomain created"
    else
        print_fail "First deployment failed"
        return
    fi

    # Second deployment with same subdomain
    print_test "POST /deploy (conflicting subdomain)"
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/deploy" \
        -H "Content-Type: application/json" \
        -d '{
            "repo": "https://github.com/test/app2",
            "subdomain": "conflict-test",
            "package": "small",
            "port": 3000
        }')

    if [ "$HTTP_CODE" = "400" ]; then
        print_pass "Subdomain conflict detected and rejected"
    else
        print_fail "Subdomain conflict should return 400, got $HTTP_CODE"
    fi
}

# Test 8: List Deployments
test_list_deployments() {
    print_header "Test 8: List Deployments"

    print_test "GET /deployments"
    RESPONSE=$(curl -s "${BASE_URL}/deployments")

    if echo "$RESPONSE" | grep -q '"deployments"'; then
        print_pass "Deployments list returned"
    else
        print_fail "Failed to get deployments list"
        return
    fi

    if echo "$RESPONSE" | grep -q '"package"'; then
        print_pass "Deployments include package field"
    else
        print_fail "Package field missing from deployments"
    fi
}

# Test 9: Build Logs
test_build_logs() {
    print_header "Test 9: Build Logs"

    if [ -z "$DEPLOYMENT_ID" ]; then
        print_info "Skipping build logs test (no deployment_id)"
        return
    fi

    print_test "GET /deployments/$DEPLOYMENT_ID/build-logs"
    RESPONSE=$(curl -s "${BASE_URL}/deployments/${DEPLOYMENT_ID}/build-logs")

    if echo "$RESPONSE" | grep -q '"build_logs"'; then
        print_pass "Build logs retrieved"
    else
        print_fail "Failed to get build logs"
    fi
}

# Test 10: Analytics API
test_analytics() {
    print_header "Test 10: Analytics API"

    if [ -z "$DEPLOYMENT_ID" ]; then
        print_info "Skipping analytics test (no deployment_id)"
        return
    fi

    # Wait for deployment to be running
    print_info "Waiting 10 seconds for deployment to initialize..."
    sleep 10

    print_test "GET /deployments/$DEPLOYMENT_ID/analytics"
    RESPONSE=$(curl -s "${BASE_URL}/deployments/${DEPLOYMENT_ID}/analytics")

    if echo "$RESPONSE" | grep -q '"metrics"'; then
        print_pass "Analytics endpoint returned metrics"
    else
        print_fail "Failed to get analytics"
        return
    fi

    # Check metrics structure
    if echo "$RESPONSE" | grep -q '"requests"'; then
        print_pass "Metrics include requests"
    else
        print_fail "Requests metrics missing"
    fi

    if echo "$RESPONSE" | grep -q '"latency"'; then
        print_pass "Metrics include latency"
    else
        print_fail "Latency metrics missing"
    fi

    if echo "$RESPONSE" | grep -q '"bandwidth"'; then
        print_pass "Metrics include bandwidth"
    else
        print_fail "Bandwidth metrics missing"
    fi

    if echo "$RESPONSE" | grep -q '"pods"'; then
        print_pass "Metrics include pod status"
    else
        print_fail "Pod metrics missing"
    fi

    # Check pod count for non-subscriber
    if echo "$RESPONSE" | grep -q '"desired":1'; then
        print_pass "Non-subscriber has desired pods = 1"
    else
        print_fail "Desired pods should be 1 for non-subscriber"
    fi
}

# Test 11: Analytics - Non-existent Deployment
test_analytics_404() {
    print_header "Test 11: Analytics for Non-Existent Deployment"

    print_test "GET /deployments/nonexistent/analytics"
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/deployments/nonexistent123/analytics")

    if [ "$HTTP_CODE" = "404" ]; then
        print_pass "Returns 404 for non-existent deployment"
    else
        print_fail "Should return 404, got $HTTP_CODE"
    fi
}

# Print summary
print_summary() {
    print_header "Test Summary"

    echo -e "Total Tests Run:    ${TESTS_RUN}"
    echo -e "${GREEN}Tests Passed:       ${TESTS_PASSED}${NC}"

    if [ $TESTS_FAILED -gt 0 ]; then
        echo -e "${RED}Tests Failed:       ${TESTS_FAILED}${NC}"
        echo -e "\n${RED}SOME TESTS FAILED${NC}"
        exit 1
    else
        echo -e "${RED}Tests Failed:       ${TESTS_FAILED}${NC}"
        echo -e "\n${GREEN}ALL TESTS PASSED!${NC}"
        exit 0
    fi
}

# Main execution
main() {
    echo -e "${BLUE}"
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║                                                              ║"
    echo "║          MeshVPN Phase 2 End-to-End Testing Suite           ║"
    echo "║                                                              ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo -e "${NC}\n"

    print_info "Testing against: $BASE_URL"

    # Run all tests
    check_control_plane
    test_health_metrics
    test_deploy_small
    test_deploy_medium
    test_deploy_large
    test_invalid_package
    test_default_package
    test_subdomain_conflict
    test_list_deployments
    test_build_logs
    test_analytics
    test_analytics_404

    # Print summary
    print_summary
}

# Run main function
main
