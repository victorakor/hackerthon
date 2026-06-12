function showAuthTab(tab) {
  document.getElementById('auth-error').textContent = '';
  document.querySelectorAll('.auth-tab').forEach(function(t, i) {
    t.classList.toggle('active', (i === 0 && tab === 'login') || (i === 1 && tab === 'register'));
  });
  document.getElementById('login-form').style.display = tab === 'login' ? '' : 'none';
  document.getElementById('register-form').style.display = tab === 'register' ? '' : 'none';
}

function showAuthError(msg) {
  document.getElementById('auth-error').textContent = msg;
}

async function doLogin() {
  var email = document.getElementById('login-email').value.trim();
  var password = document.getElementById('login-password').value;
  if (!email || !password) { showAuthError('Email and password required'); return; }
  try {
    var res = await fetch('/api/auth/login', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({email: email, password: password})
    });
    if (!res.ok) { showAuthError(await res.text()); return; }
    var data = await res.json();
    authToken = data.token;
    currentUser = data.user;
    localStorage.setItem('authToken', authToken);
    localStorage.setItem('currentUser', JSON.stringify(currentUser));
    enterApp();
  } catch(e) { showAuthError('Connection error'); }
}

async function doRegister() {
  var name = document.getElementById('reg-name').value.trim();
  var email = document.getElementById('reg-email').value.trim();
  var password = document.getElementById('reg-password').value;
  if (!name || !email || !password) { showAuthError('All fields required'); return; }
  try {
    var res = await fetch('/api/auth/register', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({name: name, email: email, password: password})
    });
    if (!res.ok) { showAuthError(await res.text()); return; }
    var data = await res.json();
    authToken = data.token;
    currentUser = data.user;
    localStorage.setItem('authToken', authToken);
    localStorage.setItem('currentUser', JSON.stringify(currentUser));
    enterApp();
  } catch(e) { showAuthError('Connection error'); }
}

async function doLogout() {
  try { await apiFetch('/api/auth/logout', {method: 'POST'}); } catch(e) {}
  authToken = null;
  currentUser = null;
  localStorage.removeItem('authToken');
  localStorage.removeItem('currentUser');
  stopAllTimers();
  document.getElementById('auth-page').style.display = 'flex';
  document.getElementById('main-app').style.display = 'none';
}

function showForgotForm() {
  document.getElementById('auth-error').textContent = '';
  var successEl = document.getElementById('auth-success');
  if (successEl) { successEl.style.display = 'none'; successEl.textContent = ''; }
  document.querySelectorAll('.auth-tab').forEach(function(t) { t.classList.remove('active'); });
  document.getElementById('login-form').style.display = 'none';
  document.getElementById('register-form').style.display = 'none';
  document.getElementById('forgot-form').style.display = '';
}

// Override showAuthTab to also hide forgot-form
var _origShowAuthTab = showAuthTab;
showAuthTab = function(tab) {
  document.getElementById('forgot-form').style.display = 'none';
  var successEl = document.getElementById('auth-success');
  if (successEl) { successEl.style.display = 'none'; successEl.textContent = ''; }
  _origShowAuthTab(tab);
};

async function doForgotPassword() {
  var email = document.getElementById('forgot-email').value.trim();
  var errEl = document.getElementById('auth-error');
  var successEl = document.getElementById('auth-success');
  errEl.textContent = '';
  successEl.style.display = 'none';

  if (!email) { errEl.textContent = 'Please enter your email.'; return; }

  try {
    var res = await fetch('/api/auth/forgot-password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: email })
    });
    var data = await res.json();
    successEl.textContent = data.message || 'Check your inbox for a reset link.';
    successEl.style.display = 'block';
    document.getElementById('forgot-email').value = '';
  } catch(e) {
    errEl.textContent = 'Connection error. Please try again.';
  }
}

// ── Reset password page (shown when URL has ?token=) ──────────────────────────

async function initResetPage() {
  var params = new URLSearchParams(window.location.search);
  var token = params.get('token');
  if (!token) return;

  // Show the reset page, hide auth page
  document.getElementById('auth-page').style.display = 'none';
  var resetPage = document.getElementById('reset-page');
  resetPage.style.display = 'flex';

  // Validate the token with the server
  try {
    var res = await fetch('/api/auth/validate-reset-token?token=' + encodeURIComponent(token));
    if (!res.ok) {
      document.getElementById('reset-form-inner').style.display = 'none';
      var errEl = document.getElementById('reset-token-error');
      errEl.textContent = 'This reset link is invalid or has expired. Please request a new one.';
      errEl.style.display = 'block';
      var btn = document.createElement('button');
      btn.className = 'btn-primary';
      btn.style.marginTop = '12px';
      btn.textContent = 'Request new link';
      btn.onclick = function() {
        resetPage.style.display = 'none';
        document.getElementById('auth-page').style.display = 'flex';
        showForgotForm();
        // Clear the token from the URL without reloading
        history.replaceState({}, '', window.location.pathname);
      };
      errEl.parentNode.appendChild(btn);
    }
  } catch(e) {
    document.getElementById('reset-token-error').textContent = 'Could not verify reset link.';
    document.getElementById('reset-token-error').style.display = 'block';
  }
}

async function doResetPassword() {
  var params = new URLSearchParams(window.location.search);
  var token = params.get('token');
  var password = document.getElementById('reset-password').value;
  var password2 = document.getElementById('reset-password2').value;
  var errEl = document.getElementById('reset-error');
  var successEl = document.getElementById('reset-success');
  errEl.textContent = '';
  successEl.style.display = 'none';

  if (!password || password.length < 6) { errEl.textContent = 'Password must be at least 6 characters.'; return; }
  if (password !== password2) { errEl.textContent = 'Passwords do not match.'; return; }

  try {
    var res = await fetch('/api/auth/reset-password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token: token, password: password })
    });
    var data = await res.json();
    if (!res.ok) { errEl.textContent = data || 'Failed to reset password.'; return; }

    document.getElementById('reset-form-inner').style.display = 'none';
    successEl.textContent = 'Password updated! Redirecting to login…';
    successEl.style.display = 'block';

    setTimeout(function() {
      document.getElementById('reset-page').style.display = 'none';
      document.getElementById('auth-page').style.display = 'flex';
      showAuthTab('login');
      history.replaceState({}, '', window.location.pathname);
    }, 2000);
  } catch(e) {
    errEl.textContent = 'Connection error. Please try again.';
  }
}

// Run on page load
initResetPage();