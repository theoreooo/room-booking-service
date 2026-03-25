# Load Test Results

Date: 2026-03-25T04:47:48.186Z

Target:
- Hot endpoint `GET /rooms/{roomId}/slots/list?date=YYYY-MM-DD`
- Service base URL `http://app:8080`

Method:
- Real HTTP load test with `k6` against the running application.
- Request path includes router, middleware, service layer, PostgreSQL access, and JSON serialization.

Dataset:
- Namespace: `20260325044748172`
- Rooms created for this run: `50`
- Schedule per room: `09:00-19:00 UTC`
- Slots per room per day: `20`
- Total slots per day across the target profile: `1000`
- Future slots prepared across `7` days: `7000`
- Active bookings pre-created: `3500`

Load profile:
- Rate: `100 RPS`
- Duration: `30s`
- Pre-allocated VUs: `50`
- Max VUs: `200`
- Request timeout: `2s`
- Target room/date combinations: `350`

Results:
- Total requests: `3000`
- Successful responses (200): `3000`
- Non-200 responses: `0`
- Success rate: `100.00%`
- Achieved throughput: `100.00 RPS`
- Latency avg: `1.01 ms`
- Latency p50: `0.96 ms`
- Latency p95: `1.42 ms`
- Latency p99: `1.62 ms`
- Latency max: `6.63 ms`

Takeaway:
- The hot endpoint met the target profile for this run: success rate >= 99.9% and p99 <= 200 ms at 100 RPS.
- This run is a reproducible black-box benchmark of the deployed service, not an in-process benchmark.
