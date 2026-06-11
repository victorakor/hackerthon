function switchTab(name) {
  document.querySelectorAll('[id^="tab-"]').forEach(function(el) { el.style.display = 'none'; });
  document.querySelectorAll('.nav-tab').forEach(function(t) { t.classList.remove('active'); });
  document.getElementById('tab-' + name).style.display = '';
  document.querySelector('[data-tab="' + name + '"]').classList.add('active');
  if (name === 'leaderboard') loadLeaderboard();
  if (name === 'notifications') loadNotifications();
  if (name === 'admin') loadAdminPanel();
}