import http from 'k6/http';
import { check, fail, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const baseURL = __ENV.BASE_URL || 'http://app:8080';
const reportPath = __ENV.REPORT_PATH || '/work/loadtest/results.md';

const rate = Number(__ENV.LOADTEST_RATE || 100);
const duration = __ENV.LOADTEST_DURATION || '30s';
const preAllocatedVUs = Number(__ENV.LOADTEST_PRE_ALLOCATED_VUS || 50);
const maxVUs = Number(__ENV.LOADTEST_MAX_VUS || 200);
const readyTimeoutSeconds = Number(__ENV.LOADTEST_READY_TIMEOUT_SECONDS || 120);
const requestTimeout = __ENV.LOADTEST_REQUEST_TIMEOUT || '2s';

const roomsCount = Number(__ENV.LOADTEST_ROOMS || 50);
const windowDays = Number(__ENV.LOADTEST_WINDOW_DAYS || 7);
const startHour = Number(__ENV.LOADTEST_START_HOUR || 9);
const endHour = Number(__ENV.LOADTEST_END_HOUR || 19);
const bookedSlotsPerRoomDay = Number(__ENV.LOADTEST_BOOKED_SLOTS_PER_ROOM_DAY || 10);
const namespace = __ENV.LOADTEST_NAMESPACE || makeNamespace();

const slotsPerRoomDay = (endHour - startHour) * 2;

const loadRequests = new Counter('load_requests');
const loadStatus200 = new Counter('load_status_200');
const loadStatusOther = new Counter('load_status_other');
const loadSuccess = new Rate('load_success');
const loadDuration = new Trend('load_duration', true);
const preparedRooms = new Counter('prepared_rooms');
const preparedFutureSlots = new Counter('prepared_future_slots');
const preparedActiveBookings = new Counter('prepared_active_bookings');
const preparedTargetCombinations = new Counter('prepared_target_combinations');

export const options = {
  scenarios: {
    list_slots: {
      executor: 'constant-arrival-rate',
      rate: rate,
      timeUnit: '1s',
      duration: duration,
      preAllocatedVUs: preAllocatedVUs,
      maxVUs: maxVUs,
      gracefulStop: '0s',
    },
  },
  thresholds: {
    load_success: ['rate>0.999'],
    load_duration: ['p(99)<200'],
  },
  summaryTrendStats: ['avg', 'min', 'med', 'p(95)', 'p(99)', 'max'],
};

export function setup() {
  validateConfig();
  waitForReady();

  const adminToken = dummyLogin('admin');
  const userToken = dummyLogin('user');
  const rooms = [];

  for (let i = 1; i <= roomsCount; i += 1) {
    const roomName = `loadtest-${namespace}-room-${pad3(i)}`;
    const roomResponse = request('POST', '/rooms/create', adminToken, {
      name: roomName,
      description: 'k6 load test fixture',
      capacity: 8,
    });

    ensureStatus(roomResponse, 201, `create room ${roomName}`);

    const roomId = roomResponse.json('room.id');
    rooms.push({ id: roomId, name: roomName });
    preparedRooms.add(1);

    const scheduleResponse = request(
      'POST',
      `/rooms/${roomId}/schedule/create`,
      adminToken,
      {
        daysOfWeek: [1, 2, 3, 4, 5, 6, 7],
        startTime: `${pad2(startHour)}:00`,
        endTime: `${pad2(endHour)}:00`,
      },
    );

    ensureStatus(scheduleResponse, 201, `create schedule for ${roomName}`);
  }

  const now = new Date();
  const startDate = startOfUTCDay(now);
  const targets = [];
  for (const room of rooms) {
    for (let dayOffset = 0; dayOffset < windowDays; dayOffset += 1) {
      const date = addDaysUTC(startDate, dayOffset);
      const dateString = formatDateUTC(date);
      const slotsResponse = request(
        'GET',
        `/rooms/${room.id}/slots/list?date=${dateString}`,
        userToken,
      );

      ensureStatus(slotsResponse, 200, `list slots for ${room.name} on ${dateString}`);

      const slots = slotsResponse.json('slots');
      if (!Array.isArray(slots)) {
        fail(`expected slots array for ${room.name} on ${dateString}`);
      }
      if (slots.length !== slotsPerRoomDay) {
        fail(
          `expected ${slotsPerRoomDay} slots for ${room.name} on ${dateString}, got ${slots.length}`,
        );
      }

      preparedFutureSlots.add(slots.length);
      preparedTargetCombinations.add(1);
      targets.push({ roomId: room.id, date: dateString });

      let bookedForDay = 0;
      for (const slot of slots) {
        if (bookedForDay >= bookedSlotsPerRoomDay) {
          break;
        }

        if (new Date(slot.start) <= now) {
          continue;
        }

        const bookingResponse = request('POST', '/bookings/create', userToken, {
          slotId: slot.id,
        });

        ensureStatus(bookingResponse, 201, `create booking for slot ${slot.id}`);
        bookedForDay += 1;
        preparedActiveBookings.add(1);
      }
    }
  }

  return {
    userToken: userToken,
    targets: targets,
  };
}

export default function (data) {
  const target = data.targets[(__VU + __ITER) % data.targets.length];
  const response = request(
    'GET',
    `/rooms/${target.roomId}/slots/list?date=${target.date}`,
    data.userToken,
    null,
    { tags: { endpoint: 'slots_list' } },
  );

  loadRequests.add(1);
  loadDuration.add(response.timings.duration);

  const ok = response.status === 200;
  loadSuccess.add(ok);

  if (ok) {
    loadStatus200.add(1);
  } else {
    loadStatusOther.add(1);
  }

  check(response, {
    'slots list returns 200': (res) => res.status === 200,
  });
}

export function handleSummary(data) {
  const report = buildReport(data);
  return {
    [reportPath]: report,
    stdout: report,
  };
}

function request(method, path, token, body, params) {
  const requestParams = params || {};
  requestParams.timeout = requestTimeout;
  requestParams.headers = requestParams.headers || {};

  let payload = body;
  if (body !== undefined && body !== null) {
    requestParams.headers['Content-Type'] = 'application/json';
    payload = JSON.stringify(body);
  }

  if (token) {
    requestParams.headers.Authorization = `Bearer ${token}`;
  }

  return http.request(method, `${baseURL}${path}`, payload, requestParams);
}

function waitForReady() {
  for (let attempt = 0; attempt < readyTimeoutSeconds; attempt += 1) {
    const response = http.get(`${baseURL}/_info`, { timeout: requestTimeout });
    if (response.status === 200) {
      return;
    }

    sleep(1);
  }

  fail(`service ${baseURL} was not ready within ${readyTimeoutSeconds}s`);
}

function dummyLogin(role) {
  const response = request('POST', '/dummyLogin', '', { role: role });
  ensureStatus(response, 200, `dummyLogin ${role}`);

  const token = response.json('token');
  if (!token) {
    fail(`dummyLogin ${role} returned empty token`);
  }

  return token;
}

function ensureStatus(response, expectedStatus, operation) {
  if (response.status !== expectedStatus) {
    fail(
      `${operation} returned ${response.status}, expected ${expectedStatus}: ${response.body}`,
    );
  }
}

function validateConfig() {
  if (Number.isNaN(rate) || rate < 1) {
    fail('LOADTEST_RATE must be a positive integer');
  }
  if (Number.isNaN(roomsCount) || roomsCount < 1) {
    fail('LOADTEST_ROOMS must be a positive integer');
  }
  if (Number.isNaN(windowDays) || windowDays < 1) {
    fail('LOADTEST_WINDOW_DAYS must be a positive integer');
  }
  if (Number.isNaN(startHour) || startHour < 0 || startHour > 23) {
    fail('LOADTEST_START_HOUR must be between 0 and 23');
  }
  if (Number.isNaN(endHour) || endHour < 1 || endHour > 24 || endHour <= startHour) {
    fail('LOADTEST_END_HOUR must be between 1 and 24 and greater than LOADTEST_START_HOUR');
  }
  if (
    Number.isNaN(bookedSlotsPerRoomDay) ||
    bookedSlotsPerRoomDay < 0 ||
    bookedSlotsPerRoomDay > slotsPerRoomDay
  ) {
    fail(`LOADTEST_BOOKED_SLOTS_PER_ROOM_DAY must be between 0 and ${slotsPerRoomDay}`);
  }
}

function buildReport(data) {
  const loadRequestsMetric = data.metrics.load_requests;
  const loadDurationMetric = data.metrics.load_duration;
  const loadSuccessMetric = data.metrics.load_success;
  const loadStatus200Metric = data.metrics.load_status_200;
  const loadStatusOtherMetric = data.metrics.load_status_other;
  const preparedRoomsMetric = data.metrics.prepared_rooms;
  const preparedFutureSlotsMetric = data.metrics.prepared_future_slots;
  const preparedActiveBookingsMetric = data.metrics.prepared_active_bookings;
  const preparedTargetCombinationsMetric = data.metrics.prepared_target_combinations;

  const totalRequests = getMetricValue(loadRequestsMetric, 'count');
  const achievedRPS = totalRequests / parseDurationSeconds(duration);
  const successRate = getMetricValue(loadSuccessMetric, 'rate') * 100;
  const status200 = getMetricValue(loadStatus200Metric, 'count');
  const statusOther = getMetricValue(loadStatusOtherMetric, 'count');
  const roomsCreated = getMetricValue(preparedRoomsMetric, 'count');
  const futureSlotsPrepared = getMetricValue(preparedFutureSlotsMetric, 'count');
  const activeBookingsMade = getMetricValue(preparedActiveBookingsMetric, 'count');
  const targetCombinations = getMetricValue(preparedTargetCombinationsMetric, 'count');
  const avgLatency = getMetricValue(loadDurationMetric, 'avg');
  const p50Latency = getMetricValue(loadDurationMetric, 'med');
  const p95Latency = getMetricValue(loadDurationMetric, 'p(95)');
  const p99Latency = getMetricValue(loadDurationMetric, 'p(99)');
  const maxLatency = getMetricValue(loadDurationMetric, 'max');

  const meetsSuccess = successRate >= 99.9;
  const meetsLatency = p99Latency <= 200;

  const lines = [
    '# Load Test Results',
    '',
    `Date: ${new Date().toISOString()}`,
    '',
    'Target:',
    '- Hot endpoint `GET /rooms/{roomId}/slots/list?date=YYYY-MM-DD`',
    `- Service base URL \`${baseURL}\``,
    '',
    'Method:',
    '- Real HTTP load test with `k6` against the running application.',
    '- Request path includes router, middleware, service layer, PostgreSQL access, and JSON serialization.',
    '',
    'Dataset:',
    `- Namespace: \`${namespace}\``,
    `- Rooms created for this run: \`${formatCount(roomsCreated)}\``,
    `- Schedule per room: \`${pad2(startHour)}:00-${pad2(endHour)}:00 UTC\``,
    `- Slots per room per day: \`${slotsPerRoomDay}\``,
    `- Total slots per day across the target profile: \`${roomsCount * slotsPerRoomDay}\``,
    `- Future slots prepared across \`${windowDays}\` days: \`${formatCount(futureSlotsPrepared)}\``,
    `- Active bookings pre-created: \`${formatCount(activeBookingsMade)}\``,
    '',
    'Load profile:',
    `- Rate: \`${rate} RPS\``,
    `- Duration: \`${duration}\``,
    `- Pre-allocated VUs: \`${preAllocatedVUs}\``,
    `- Max VUs: \`${maxVUs}\``,
    `- Request timeout: \`${requestTimeout}\``,
    `- Target room/date combinations: \`${formatCount(targetCombinations)}\``,
    '',
    'Results:',
    `- Total requests: \`${formatCount(totalRequests)}\``,
    `- Successful responses (200): \`${formatCount(status200)}\``,
    `- Non-200 responses: \`${formatCount(statusOther)}\``,
    `- Success rate: \`${successRate.toFixed(2)}%\``,
    `- Achieved throughput: \`${achievedRPS.toFixed(2)} RPS\``,
    `- Latency avg: \`${formatMilliseconds(avgLatency)}\``,
    `- Latency p50: \`${formatMilliseconds(p50Latency)}\``,
    `- Latency p95: \`${formatMilliseconds(p95Latency)}\``,
    `- Latency p99: \`${formatMilliseconds(p99Latency)}\``,
    `- Latency max: \`${formatMilliseconds(maxLatency)}\``,
    '',
    'Takeaway:',
    takeawayLine(meetsSuccess, meetsLatency),
    '- This run is a reproducible black-box benchmark of the deployed service, not an in-process benchmark.',
    '',
  ];

  return lines.join('\n');
}

function takeawayLine(meetsSuccess, meetsLatency) {
  if (meetsSuccess && meetsLatency) {
    return `- The hot endpoint met the target profile for this run: success rate >= 99.9% and p99 <= 200 ms at ${rate} RPS.`;
  }
  if (!meetsSuccess && !meetsLatency) {
    return '- The hot endpoint missed both the success-rate target and the 200 ms p99 latency budget.';
  }
  if (!meetsSuccess) {
    return '- The hot endpoint stayed within the latency budget, but missed the success-rate target.';
  }

  return '- The hot endpoint met the success-rate target, but missed the 200 ms p99 latency budget.';
}

function getMetricValue(metric, key) {
  if (!metric || !metric.values || metric.values[key] === undefined) {
    return 0;
  }

  return metric.values[key];
}

function formatMilliseconds(value) {
  return `${value.toFixed(2)} ms`;
}

function formatCount(value) {
  return Math.round(value).toString();
}

function parseDurationSeconds(value) {
  const matches = String(value).matchAll(/(\d+)(ms|s|m|h)/g);
  let totalSeconds = 0;

  for (const match of matches) {
    const amount = Number(match[1]);
    const unit = match[2];

    if (unit === 'ms') {
      totalSeconds += amount / 1000;
    } else if (unit === 's') {
      totalSeconds += amount;
    } else if (unit === 'm') {
      totalSeconds += amount * 60;
    } else if (unit === 'h') {
      totalSeconds += amount * 3600;
    }
  }

  if (totalSeconds <= 0) {
    fail(`unsupported duration format: ${value}`);
  }

  return totalSeconds;
}

function makeNamespace() {
  return new Date().toISOString().replace(/[-:.TZ]/g, '');
}

function pad2(value) {
  return String(value).padStart(2, '0');
}

function pad3(value) {
  return String(value).padStart(3, '0');
}

function startOfUTCDay(date) {
  return new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()));
}

function addDaysUTC(date, days) {
  return new Date(
    Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate() + days),
  );
}

function formatDateUTC(date) {
  return [
    date.getUTCFullYear(),
    pad2(date.getUTCMonth() + 1),
    pad2(date.getUTCDate()),
  ].join('-');
}
