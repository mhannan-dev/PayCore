import http from 'k6/http';
import { check, group, sleep } from 'k6';
import exec from 'k6/execution';
import { Counter, Rate, Trend } from 'k6/metrics';

const BASE_URL = (__ENV.BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const EMAIL = __ENV.EMAIL || 'operator@yopmail.com';
const AUDITOR_EMAIL = __ENV.AUDITOR_EMAIL || 'auditor@yopmail.com';
const PASSWORD = __ENV.PAYCORE_LOAD_PASSWORD || '';
const CURRENCY = __ENV.CURRENCY || 'JPY';
const DEBIT_ACCOUNT = __ENV.DEBIT_ACCOUNT || '1001';
const CREDIT_ACCOUNT = __ENV.CREDIT_ACCOUNT || '2003';
const VUS = Number(__ENV.VUS || '20');
const DURATION = __ENV.DURATION || '30s';
const SLEEP_SECONDS = Number(__ENV.SLEEP_SECONDS || '0');
const AMOUNT = Number(__ENV.AMOUNT || '1');
const RUN_ID = __ENV.RUN_ID || `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
const MAX_BUSY = Number(__ENV.MAX_BUSY || '0');
const MAX_P95_MS = Number(__ENV.MAX_P95_MS || '2000');
const MAX_FAILURE_RATE = Number(__ENV.MAX_FAILURE_RATE || '0.01');

const transferDuration = new Trend('transfer_duration_ms', true);
const transferFailures = new Rate('transfer_failures');
const duplicatePosts = new Counter('duplicate_posts');
const overdrafts = new Counter('overdrafts');
const ledgerBusy = new Counter('ledger_busy');
const invariantFailures = new Counter('invariant_failures');
const idempotencyReplays = new Counter('idempotency_replays');

export const options = {
  vus: VUS,
  duration: DURATION,
  thresholds: {
    http_req_failed: [`rate<${MAX_FAILURE_RATE}`],
    transfer_failures: [`rate<${MAX_FAILURE_RATE}`],
    transfer_duration_ms: [`p(95)<${MAX_P95_MS}`],
    duplicate_posts: ['count<1'],
    overdrafts: ['count<1'],
    ledger_busy: [`count<${MAX_BUSY + 1}`],
    invariant_failures: ['count<1'],
  },
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
};

function jsonPath(response, path) {
  try {
    return response.json(path);
  } catch (error) {
    return undefined;
  }
}

function shortError(response) {
  const code = jsonPath(response, 'error') || '';
  return `${response.status}${code ? ` ${code}` : ''}`;
}

function requireCheck(response, conditions, message) {
  if (!check(response, conditions)) {
    throw new Error(`${message}: ${shortError(response)}`);
  }
  return response;
}

function captchaAnswer(svg) {
  const chars = Array.from(svg.matchAll(/<text[^>]*>([^<]*)<\/text>/g))
    .map((match) => match[1].trim())
    .filter((char) => char !== '' && char !== '=' && char !== '?');
  const expression = chars.join('').replace(/×/g, '*');
  if (!/^\d+(?:[+\-*]\d+)+$/.test(expression)) {
    throw new Error('Could not parse the CAPTCHA answer from the challenge image');
  }
  const parts = expression.split(/([+\-*])/);
  let result = Number(parts[0]);
  for (let i = 1; i < parts.length; i += 2) {
    const operator = parts[i];
    const value = Number(parts[i + 1]);
    if (operator === '+') result += value;
    else if (operator === '-') result -= value;
    else result *= value;
  }
  return String(result);
}

function login(email) {
  const challenge = http.get(`${BASE_URL}/api/v1/auth/captcha`);
  requireCheck(challenge, { 'captcha issued': (r) => r.status === 200 }, 'CAPTCHA request failed');
  const loginResponse = http.post(
    `${BASE_URL}/api/v1/auth/login`,
    JSON.stringify({
      email,
      password: PASSWORD,
      captcha_id: jsonPath(challenge, 'captcha_id'),
      captcha_answer: captchaAnswer(jsonPath(challenge, 'image')),
    }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  requireCheck(
    loginResponse,
    {
      'login accepted': (r) => r.status === 200,
      'login returned a token': (r) => typeof jsonPath(r, 'token') === 'string',
    },
    'login failed',
  );
  return jsonPath(loginResponse, 'token');
}

function authHeaders(token) {
  return { Authorization: `Bearer ${token}` };
}

export function setup() {
  if (!PASSWORD) {
    throw new Error('PAYCORE_LOAD_PASSWORD must be set before running the load test');
  }
  const token = login(EMAIL);
  const auditorToken = login(AUDITOR_EMAIL);
  const headers = authHeaders(token);
  const accounts = http.get(`${BASE_URL}/api/v1/accounts?currency=${CURRENCY}&limit=200`, { headers });
  requireCheck(accounts, { 'accounts listed': (r) => r.status === 200 }, 'account discovery failed');
  const items = jsonPath(accounts, 'items') || [];
  const debit = items.find((a) => a.account_number === DEBIT_ACCOUNT && a.currency === CURRENCY && a.is_active);
  const credit = items.find((a) => a.account_number === CREDIT_ACCOUNT && a.currency === CURRENCY && a.is_active);
  if (!debit || !credit) {
    throw new Error(`Load-test accounts ${DEBIT_ACCOUNT}/${CREDIT_ACCOUNT} were not found or are inactive in ${CURRENCY}`);
  }
  const zeroSum = http.get(`${BASE_URL}/api/v1/audit/zero-sum?currency=${CURRENCY}`, {
    headers: authHeaders(auditorToken),
  });
  requireCheck(
    zeroSum,
    {
      'initial zero-sum available': (r) => r.status === 200,
      'initial zero-sum balanced': (r) => jsonPath(r, 'balanced') === true && jsonPath(r, 'difference') === '0',
    },
    'initial zero-sum check failed',
  );
  return {
    token,
    auditorToken,
    debitId: debit.id,
    creditId: credit.id,
    initialJournals: Number(jsonPath(zeroSum, 'journals')),
  };
}

export default function (data) {
  const key = `load-${RUN_ID}-${exec.vu.idInTest}-${__ITER}`;
  const body = JSON.stringify({
    reference_id: `LOAD-${RUN_ID}`,
    description: 'PayCore k6 concurrency load test',
    currency: CURRENCY,
    legs: [
      { account_id: data.debitId, direction: 'DEBIT', amount: AMOUNT },
      { account_id: data.creditId, direction: 'CREDIT', amount: AMOUNT },
    ],
  });
  const params = {
    headers: {
      ...authHeaders(data.token),
      'Content-Type': 'application/json',
      'Idempotency-Key': key,
    },
  };
  let first;
  let second;
  const startedAt = Date.now();
  group('idempotent transfer', () => {
    first = http.post(`${BASE_URL}/api/v1/transfers`, body, params);
    second = http.post(`${BASE_URL}/api/v1/transfers`, body, params);
  });
  transferDuration.add(Date.now() - startedAt);

  const firstId = jsonPath(first, 'id');
  const secondId = jsonPath(second, 'id');
  const firstOk = check(first, {
    'transfer accepted': (r) => r.status === 201,
    'transfer returned a journal id': (r) => typeof jsonPath(r, 'id') === 'string',
  });
  const secondOk = check(second, {
    'replay accepted': (r) => r.status === 201,
    'replay returned the same journal id': (r) => typeof secondId === 'string' && secondId === firstId,
  });
  if (typeof firstId === 'string' && typeof secondId === 'string' && firstId !== secondId) {
    duplicatePosts.add(1);
  }
  if (typeof firstId === 'string' && firstId === secondId) {
    idempotencyReplays.add(1);
  }
  for (const response of [first, second]) {
    const code = jsonPath(response, 'error');
    if (code === 'insufficient_funds') overdrafts.add(1);
    if (code === 'ledger_busy') ledgerBusy.add(1);
  }
  transferFailures.add(firstOk && secondOk ? 0 : 1);
  if (SLEEP_SECONDS > 0) sleep(SLEEP_SECONDS);
}

export function teardown(data) {
  const zeroSum = http.get(`${BASE_URL}/api/v1/audit/zero-sum?currency=${CURRENCY}`, {
    headers: authHeaders(data.auditorToken),
  });
  const ok = check(zeroSum, {
    'final zero-sum available': (r) => r.status === 200,
    'final zero-sum balanced': (r) => jsonPath(r, 'balanced') === true && jsonPath(r, 'difference') === '0',
    'final zero-sum has no findings': (r) => Array.isArray(jsonPath(r, 'unbalanced_journals')) && jsonPath(r, 'unbalanced_journals').length === 0,
    'final zero-sum advanced': (r) => Number(jsonPath(r, 'journals')) > data.initialJournals,
  });
  if (!ok) invariantFailures.add(1);
}
