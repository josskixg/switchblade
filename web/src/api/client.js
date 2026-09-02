import axios from 'axios';

export const client = axios.create({
  baseURL: '',
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
});

/** Dispatched on `window` when the gateway refuses a request for billing reasons. */
export const BILLING_BLOCK_EVENT = 'sb:billing-block';

/**
 * The 402 contract with the shell. `detail` is the gateway's body verbatim
 * ({ error, code }) — a listener must be able to read the code the server sent
 * without knowing how this module reshapes it for BILLING_BLOCK_EVENT.
 */
export const PAYMENT_REQUIRED_EVENT = 'switchblade:payment-required';

// A 401 from these is the answer to the question the caller asked, not a
// session that expired mid-session, so the caller renders it.
const AUTH_ENDPOINTS = ['/api/auth/login', '/api/auth/refresh'];

function isAuthEndpoint(url) {
  return AUTH_ENDPOINTS.some((p) => String(url || '').startsWith(p));
}

/**
 * The server's own words. Axios otherwise leaves `message` as "Request failed
 * with status code 500", which tells a user nothing and hides the reason the
 * handler already wrote into the body.
 */
function serverMessage(err) {
  const data = err.response?.data;
  if (typeof data === 'string') return data.trim() || null;
  return data?.error || data?.message || null;
}

// Token is read from localStorage on each request rather than cached, so a
// logout in one tab takes effect in the others.
client.interceptors.request.use((config) => {
  const token = localStorage.getItem('sb_token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

client.interceptors.response.use(
  (res) => res,
  (err) => {
    const status = err.response?.status;

    const msg = serverMessage(err);
    if (msg) err.message = msg;

    if (status === 401 && !isAuthEndpoint(err.config?.url)) {
      localStorage.removeItem('sb_token');
      // replace(), and only from off the login page: assign() stacks a history
      // entry per bounced request, so Back walks the user into the same 401,
      // and redirecting while already on /login reloads the page under the
      // form that is trying to report the failure.
      if (window.location.pathname !== '/login') {
        window.location.replace('/login');
      }
      return Promise.reject(err);
    }

    if (status === 402) {
      window.dispatchEvent(
        new CustomEvent(PAYMENT_REQUIRED_EVENT, { detail: err.response?.data })
      );
    }

    // BillingGate answers 402 with a machine-readable `code`; a 403 carries
    // `tenant_inactive`. Plain 403s from role checks have no code and must not
    // raise a billing banner.
    const code = err.response?.data?.code;
    if ((status === 402 || status === 403) && code) {
      window.dispatchEvent(
        new CustomEvent(BILLING_BLOCK_EVENT, {
          detail: { status, code, message: err.response?.data?.error },
        })
      );
    }

    return Promise.reject(err);
  }
);
