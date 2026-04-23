from locust import HttpUser, constant, task, between
import random

class MeshVPNUser(HttpUser):
    """Load test for MeshVPN edge deployments"""
    wait_time = constant(0)
    
    def on_start(self):
        """Called when a user starts"""
        print(f"User started: {self.host}")
    
    @task
    def index(self):
        """Test root endpoint"""
        self.client.get("/", name="GET /")
    
    @task
    def health_check(self):
        """Test health endpoint"""
        self.client.get("/health", name="GET /health")
    
    @task
    def api_request(self):
        """Test API endpoint"""
        endpoint = random.choice(["/api/status", "/api/metrics", "/api/health"])
        self.client.get(endpoint, name=f"GET {endpoint}")
    

# Configuration:
# Run with: locust -f locustfile.py -H http://localhost:8080 -u 10 -r 2 --run-time 5m
# Replace http://localhost:8080 with your actual service URL