window.addEventListener('DOMContentLoaded', function() {
  var saved = localStorage.getItem('authToken');
  var savedUser = localStorage.getItem('currentUser');
  if (saved && savedUser) {
    authToken = saved;
    try { currentUser = JSON.parse(savedUser); } catch(e) { return; }
    enterApp();
  }
});

async function enterApp() {
  document.getElementById('auth-page').style.display = 'none';
  document.getElementById('main-app').style.display = 'block';

  try {
    var meRes = await apiFetch('/api/me');
    if (meRes.ok) {
      var meData = await meRes.json();
      currentUser = meData.user;
      localStorage.setItem('currentUser', JSON.stringify(currentUser));
      document.getElementById('header-answered').textContent = meData.answered + ' solved';
    }
  } catch(e) {}

  document.getElementById('header-username').textContent = currentUser.name;
  if (currentUser.is_admin) {
    document.getElementById('header-admin-badge').style.display = 'inline';
    document.getElementById('admin-tab').style.display = 'inline-block';
  }

  await loadFollowing();
  await loadQuestions();
  checkNotifications();
  startTimer();
  startNotificationPolling();
  // Resume any active contest if page was refreshed mid-contest
  checkForActiveChallenge();
  checkForActiveTournament();
}