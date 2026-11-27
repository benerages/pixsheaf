// Load test for batch mode (k6). Started by tests/load/run.sh, which
// brings up the stack with relaxed client limits and checks the memory of
// every container afterwards.
//
// Each iteration is one batch: create a job with FILES files, upload them
// (every tenth file is broken), wait until the job is done, check that
// broken files failed individually and all others succeeded, and download
// the ZIP archive.
import http from 'k6/http';
import { check, fail, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const BASE = __ENV.BASE || 'http://127.0.0.1:8080';
const API = `${BASE}/api/v1`;
const FILES = parseInt(__ENV.FILES || '50', 10);
const BATCHES = parseInt(__ENV.BATCHES || '4', 10);
const VUS = parseInt(__ENV.VUS || '2', 10);

const large = open('/data/large.png', 'b');
const broken = open('/tests/e2e/fixtures/broken.png', 'b');

const batchSeconds = new Trend('batch_seconds');
const filesDone = new Counter('files_done');
const filesFailed = new Counter('files_failed');
const busy = new Counter('upload_busy');

export const options = {
  scenarios: {
    batches: {
      executor: 'shared-iterations',
      vus: VUS,
      iterations: BATCHES,
      maxDuration: '20m',
    },
  },
  thresholds: {
    checks: ['rate==1'],
    // Busy answers (503) are retried and count as expected responses.
    http_req_failed: ['rate==0'],
  },
};

const pipeline = {
  version: 1,
  steps: [
    { op: 'autoOrient' },
    { op: 'resize', params: { longestEdge: 1600 } },
    { op: 'watermark', params: { text: 'Pixsheaf load test', size: 30, opacity: 0.4 } },
    { op: 'convert', params: { format: 'webp', quality: 80 } },
  ],
};

function json(res) {
  try {
    return res.json();
  } catch (_) {
    return null;
  }
}

// upload retries while the server is busy (503 with Retry-After).
function upload(id, auth, name, data) {
  for (let attempt = 0; attempt < 120; attempt++) {
    const res = http.post(
      `${API}/jobs/${id}/files`,
      { file: http.file(data, name, 'image/png') },
      { headers: auth, tags: { name: 'upload' }, responseCallback: http.expectedStatuses(201, 503) },
    );
    if (res.status === 201) {
      return;
    }
    busy.add(1);
    sleep(parseInt(res.headers['Retry-After'] || '2', 10));
  }
  fail(`upload of ${name} kept failing`);
}

export default function () {
  const start = Date.now();
  const created = http.post(`${API}/jobs`, JSON.stringify({ pipeline, expectedFiles: FILES, naming: { pattern: '{index}-{name}' } }), {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'create' },
  });
  if (!check(created, { 'job created': (r) => r.status === 201 })) {
    fail(`create: ${created.status} ${created.body}`);
  }
  const { job, token } = json(created);
  const auth = { Authorization: `Bearer ${token}` };

  let brokenCount = 0;
  for (let i = 0; i < FILES; i++) {
    if (i % 10 === 9) {
      brokenCount++;
      upload(job.id, auth, `broken-${i}.png`, broken);
    } else {
      upload(job.id, auth, `photo-${i}.png`, large);
    }
  }

  let state = null;
  for (let i = 0; i < 600; i++) {
    state = json(http.get(`${API}/jobs/${job.id}`, { headers: auth, tags: { name: 'poll' } }));
    if (state && state.status === 'done') {
      break;
    }
    sleep(1);
  }
  const done = state ? state.files.filter((f) => f.status === 'done').length : 0;
  const failed = state ? state.files.filter((f) => f.status === 'failed').length : 0;
  filesDone.add(done);
  filesFailed.add(failed);
  check(state, {
    'job done': (s) => s && s.status === 'done',
    'every valid file processed': () => done === FILES - brokenCount,
    'only broken files failed': () =>
      failed === brokenCount && state.files.filter((f) => f.status === 'failed').every((f) => f.error.code === 'decode_failed'),
  });

  const zip = http.get(`${API}/jobs/${job.id}/download`, {
    headers: auth,
    responseType: 'none',
    tags: { name: 'download' },
  });
  check(zip, {
    'ZIP downloaded': (r) => r.status === 200 && r.headers['Content-Type'] === 'application/zip',
  });
  batchSeconds.add((Date.now() - start) / 1000);
}
