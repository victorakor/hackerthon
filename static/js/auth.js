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