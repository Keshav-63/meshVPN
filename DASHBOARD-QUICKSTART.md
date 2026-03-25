# 🚀 MeshVPN Analytics Dashboard - Quick Start

## Start Everything (3 Commands)

### In WSL Terminal:
```bash
# 1. Start Prometheus
cd /mnt/c/Users/Keshav\ suthar/Desktop/MeshVPN-slef-hosting/infra/observability
prometheus --config.file=prometheus.yml

# Or use the automated script:
cd /mnt/c/Users/Keshav\ suthar/Desktop/MeshVPN-slef-hosting
./scripts/start-analytics.sh
```

### In Another Terminal:
```bash
# 2. Start Grafana
cd /mnt/c/Users/Keshav\ suthar/Desktop/MeshVPN-slef-hosting/infra/observability
docker-compose up -d grafana
```

### In Your Browser:
```
# 3. Open Dashboard
http://localhost:3001
```

## ✨ That's It!

The comprehensive analytics dashboard will load automatically with:
- ✅ Worker status and capacity
- ✅ Request rate and latency
- ✅ CPU and memory usage
- ✅ Bandwidth monitoring
- ✅ Deployment analytics
- ✅ Per-worker resource tracking

## 📊 Dashboard Overview

### What You'll See:

**Top Row (Overview):**
- 6 stat panels showing workers, deployments, pods, requests at a glance

**Section 1: Worker Capacity**
- Live capacity utilization chart
- Worker status distribution over time

**Section 2: Performance**
- Request rate (requests/second)
- Latency percentiles (p50, p90, p95, p99)

**Section 3: Bandwidth**
- Total platform bandwidth (upload/download)
- Top 10 bandwidth consumers

**Section 4: Resources**
- CPU usage (average & peak) with color thresholds
- Memory usage (average & peak)
- Top 10 resource consumers

**Section 5: Deployments**
- Deployment status distribution
- Pod counts (current vs desired)
- Top active deployments
- Job completion rates

**Section 6: Worker Details**
- Complete worker inventory table
- CPU/Memory allocation per worker
- Pods deployed per worker
- Active jobs per worker

## 🎯 Access Points

| Service | URL | Purpose |
|---------|-----|---------|
| **Grafana Dashboard** | http://localhost:3001 | Main analytics interface |
| **Prometheus UI** | http://localhost:9090 | Metrics database |
| **Prometheus Targets** | http://localhost:9090/targets | Check scraping status |
| **Control-Plane Metrics** | http://localhost:8080/metrics | Raw metrics endpoint |

## ⚡ Quick Actions

### View Metrics
```bash
# Check control-plane metrics
curl http://localhost:8080/metrics

# Check Prometheus health
curl http://localhost:9090/-/healthy

# Check Grafana health
curl http://localhost:3001/api/health
```

### Stop Services
```bash
# Stop Prometheus (in WSL)
pkill prometheus

# Stop Grafana
cd infra/observability
docker-compose stop grafana
```

### Restart Services
```bash
# Restart Grafana
docker-compose restart grafana

# Restart Prometheus
# Just kill and re-run the prometheus command above
```

## 🔍 Key Metrics to Watch

| Metric | Good | Warning | Critical |
|--------|------|---------|----------|
| **CPU Usage** | < 70% | 70-90% | > 90% |
| **Memory Usage** | < 512 MB | 512-900 MB | > 900 MB |
| **p99 Latency** | < 0.5s | 0.5-1s | > 1s |
| **Worker Utilization** | < 80% | 80-95% | > 95% |
| **Failed Deployments** | 0 | 1-2 | > 2 |

## 🎨 Dashboard Features

- **Auto-Refresh**: Updates every 5 seconds (configurable)
- **Time Range**: Last 1 hour default (change top-right)
- **Interactive**: Hover for details, click legends to toggle
- **Zoom**: Click and drag on any chart to zoom into time range
- **No Login**: Anonymous access enabled for local development
- **Color-Coded**: Green = good, Yellow = warning, Red = critical

## 📖 Full Documentation

- **Complete Guide**: [GRAFANA-DASHBOARD-GUIDE.md](GRAFANA-DASHBOARD-GUIDE.md)
- **Implementation Summary**: [ANALYTICS-SUMMARY.md](ANALYTICS-SUMMARY.md)
- **Existing Grafana Docs**: [docs/GRAFANA-SETUP.md](docs/GRAFANA-SETUP.md)

## 🐛 Troubleshooting

**No data in dashboard?**
1. Check Prometheus is running: `curl http://localhost:9090/-/healthy`
2. Check control-plane is running: `curl http://localhost:8080/metrics`
3. Check Prometheus targets: http://localhost:9090/targets (all should be "UP")

**Grafana won't start?**
1. Check Docker is running
2. Check port 3001 is free
3. View logs: `docker-compose logs grafana`

**Can't connect to Prometheus?**
1. Verify Prometheus is on localhost:9090 in WSL
2. Check Grafana datasource: Configuration → Data Sources → Prometheus
3. Click "Save & Test" - should say "Data source is working"

## 💡 Pro Tips

1. **Pin the dashboard** - Click star icon to favorite it
2. **Share views** - Click share icon to get a shareable link
3. **Export data** - Panel menu → Inspect → Data → Download CSV
4. **Set alerts** - Panel menu → Edit → Alert tab
5. **Compare time periods** - Use time shift in query options

---

**Quick Reference Card**
Version 1.0 | Created 2026-03-25
