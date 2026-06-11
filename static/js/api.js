function apiFetch(url, opts) {
  opts = opts || {};
  opts.headers = opts.headers || {};
  if (authToken) opts.headers['Authorization'] = 'Bearer ' + authToken;
  return fetch(url, opts);
}