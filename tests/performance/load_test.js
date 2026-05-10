import http from 'k6/http';
import { sleep, check } from 'k6';
import { SharedArray } from 'k6/data';
import { scenario } from 'k6/execution';

const BASE_URL = __ENV.BASE_URL || 'http://server:8080';
const MOCK_URL = __ENV.MOCK_URL || 'http://mockwebhook:8888';
const BATCH_ENDPOINT = `${BASE_URL}/api/v1/notifications/batches`;

const CHANNELS = ['sms', 'email', 'push'];
const PRIORITIES = ['high', 'normal', 'low'];

const idempotencyKeys = new SharedArray('idempotency-keys', function () {
  const keys = [];
  for (let i = 0; i < 500; i++) {
    keys.push(randomUUID());
  }
  return keys;
});

function randomUUID() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

function randomItem(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

function generateRecipient(channel) {
  if (channel === 'sms') return `+9055${Math.floor(Math.random() * 10000000).toString().padStart(8, '0')}`;
  if (channel === 'email') return `user${Math.floor(Math.random() * 10000)}@test.com`;
  return `device-token-${Math.floor(Math.random() * 100000)}`;
}

function generateBatch() {
  const count = Math.floor(Math.random() * 41) + 10;
  const notifications = [];
  for (let i = 0; i < count; i++) {
    const channel = randomItem(CHANNELS);
    notifications.push({
      recipient: generateRecipient(channel),
      channel: channel,
      content: `Load test notification ${Date.now()}-${i}`,
      priority: randomItem(PRIORITIES),
    });
  }
  return { notifications: notifications };
}

function getIdempotencyKey(iter) {
  if (iter > 0 && Math.random() < 0.1) {
    return idempotencyKeys[Math.floor(Math.random() * idempotencyKeys.length)];
  }
  return randomUUID();
}

function setMockStatus(status) {
  const res = http.post(
    `${MOCK_URL}/__config`,
    JSON.stringify({ status: status, response_body: '' }),
    { headers: { 'Content-Type': 'application/json' } }
  );
  console.log(`[MOCK] Set webhook status to ${status}: response=${res.status}`);
}

export const options = {
  scenarios: {
    warmup: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '15s', target: 20 },
      ],
      gracefulStop: '5s',
      exec: 'batchRequest',
      tags: { phase: 'warmup' },
    },
    sustained: {
      executor: 'constant-vus',
      vUs: 20,
      duration: '1m',
      startTime: '15s',
      gracefulStop: '5s',
      exec: 'batchRequest',
      tags: { phase: 'sustained' },
    },
    inject_failure: {
      executor: 'shared-iterations',
      iterations: 1,
      vUs: 1,
      startTime: '1m15s',
      maxDuration: '5s',
      exec: 'setMockFailure',
    },
    spike: {
      executor: 'ramping-vus',
      startVUs: 20,
      stages: [
        { duration: '10s', target: 60 },
        { duration: '15s', target: 60 },
        { duration: '10s', target: 0 },
      ],
      startTime: '1m20s',
      gracefulStop: '5s',
      exec: 'batchRequest',
      tags: { phase: 'spike' },
    },
    inject_recovery: {
      executor: 'shared-iterations',
      iterations: 1,
      vUs: 1,
      startTime: '1m45s',
      maxDuration: '5s',
      exec: 'setMockRecovery',
    },
  },
  thresholds: {
    http_req_duration: [{ threshold: 'p(95)<2000', abortOnFail: false }],
    http_req_failed: [{ threshold: 'rate<0.05', abortOnFail: false }],
    checks: [{ threshold: 'rate>0.95', abortOnFail: false }],
  },
};

export function setup() {
  setMockStatus(200);

  const checkRes = http.get(`${BASE_URL}/api/v1/notifications?page=1&limit=1`);
  console.log(`[SETUP] Server health check: status=${checkRes.status}`);

  return { startTime: Date.now() };
}

export function batchRequest(data) {
  const iter = scenario.iterationInTest;
  const key = getIdempotencyKey(iter);
  const payload = JSON.stringify(generateBatch());

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': key,
    },
    tags: { endpoint: 'batches' },
  };

  const res = http.post(BATCH_ENDPOINT, payload, params);

  const passed = check(res, {
    'status is 202': (r) => r.status === 202,
    'has batch_id': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.batch_id !== undefined && body.batch_id !== '';
      } catch {
        return false;
      }
    },
  });

  if (!passed) {
    console.error(`[FAIL] iter=${iter} status=${res.status} body=${res.body}`);
  }

  sleep(0.5);
}

export function setMockFailure() {
  console.log('[PHASE] Injecting webhook failure (500)');
  setMockStatus(500);
}

export function setMockRecovery() {
  console.log('[PHASE] Recovering webhook to 200');
  setMockStatus(200);
}

export function teardown(data) {
  const elapsed = ((Date.now() - data.startTime) / 1000).toFixed(1);
  console.log(`[TEARDOWN] Test completed in ${elapsed}s. Mock webhook reset to 200.`);
  setMockStatus(200);
}