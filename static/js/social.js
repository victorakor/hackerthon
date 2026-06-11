async function loadFollowing() {
  try {
    var res = await apiFetch('/api/follows');
    if (!res.ok) return;
    var ids = await res.json();
    followingIds = new Set(ids);
  } catch(e) {}
}

async function toggleFollow(uid, btn) {
  var isFollowing = followingIds.has(uid);
  try {
    var res = await apiFetch('/api/follows', {
      method: isFollowing ? 'DELETE' : 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({followee_id: uid})
    });
    if (!res.ok) return;
    if (isFollowing) {
      followingIds.delete(uid);
      btn.textContent = '+ Follow';
      btn.classList.remove('following');
    } else {
      followingIds.add(uid);
      btn.textContent = '\u2713 Following';
      btn.classList.add('following');
    }
  } catch(e) {}
}

async function doSearch() {
  var name = document.getElementById('search-input').value.trim();
  if (!name) return;
  try {
    var res = await apiFetch('/api/search?name=' + encodeURIComponent(name));
    if (!res.ok) return;
    var results = await res.json();
    var el = document.getElementById('search-results');
    if (!results || !results.length) {
      el.innerHTML = '<div style="font-family:var(--font-mono);font-size:13px;color:var(--muted);text-align:center;padding:40px">No users found</div>';
      return;
    }
    el.innerHTML = results.map(function(r) {
      var subsHtml = r.submissions && r.submissions.length
        ? r.submissions.map(function(s) {
            var q = questions.find(function(x) { return x.id === s.question_id; });
            return '<div class="mini-sub">' +
              '<div>' +
                '<div class="mini-sub-title">' + (q ? escHtml(q.title) : 'Question #' + s.question_id) + '</div>' +
                '<span class="sub-lang">' + escHtml(s.language) + '</span>' +
              '</div>' +
              '<span class="mini-sub-rating">' + starsHtml(s.avg_rating) + ' ' + s.avg_rating.toFixed(1) + '</span>' +
            '</div>';
          }).join('')
        : '<div style="font-family:var(--font-mono);font-size:12px;color:var(--muted)">No submissions yet</div>';

      return '<div class="user-result">' +
        '<div class="user-result-header">' +
          '<div>' +
            '<div class="user-result-name">' + escHtml(r.user.name) + '</div>' +
            '<div class="user-result-meta">' + r.submissions.length + ' solution' + (r.submissions.length !== 1 ? 's' : '') + ' submitted</div>' +
          '</div>' +
          (r.user.id !== currentUser.id ?
            '<button class="follow-btn ' + (followingIds.has(r.user.id) ? 'following' : '') + '" onclick="toggleFollow(' + r.user.id + ',this)">' +
              (followingIds.has(r.user.id) ? '\u2713 Following' : '+ Follow') +
            '</button>' : '') +
        '</div>' +
        '<div class="user-result-subs">' + subsHtml + '</div>' +
      '</div>';
    }).join('');
  } catch(e) {}
}

async function checkNotifications() {
  try {
    var res = await apiFetch('/api/notifications');
    if (!res.ok) return;
    var notifs = await res.json();
    if (notifs && notifs.length) document.getElementById('notif-dot').style.display = 'inline-block';
  } catch(e) {}
}

async function loadNotifications() {
  try {
    var res = await apiFetch('/api/notifications');
    if (!res.ok) return;
    var notifs = await res.json();
    document.getElementById('notif-dot').style.display = 'none';
    var el = document.getElementById('notif-list');
    if (!notifs || !notifs.length) {
      el.innerHTML = '<div style="font-family:var(--font-mono);font-size:13px;color:var(--muted);text-align:center;padding:60px">Follow other hackers to see their activity here</div>';
      return;
    }
    el.innerHTML = notifs.map(function(n) {
      return '<div class="notif-item">' +
        '<div class="notif-top">' +
          '<span class="notif-who">' + escHtml(n.submission.author_name) + '</span>' +
          '<span class="notif-when">' + timeAgo(n.submission.created_at) + '</span>' +
        '</div>' +
        '<div class="notif-q">' + escHtml(n.question.title) + '</div>' +
        '<span class="notif-lang">' + escHtml(n.submission.language) + '</span>' +
        '<span style="font-family:var(--font-mono);font-size:10px;color:var(--muted);margin-left:8px">' + n.question.difficulty + '</span>' +
      '</div>';
    }).join('');
  } catch(e) {}
}