// ── Tournaments ───────────────────────────────────────────────────────────────

var _activeTournamentArena = null; // { id, endsAt, questions }

// ── Badge helpers ─────────────────────────────────────────────────────────────

function showTournamentBadge() {
  var b = document.getElementById('tournament-badge');
  if (b) b.style.display = 'inline-block';
}
function hideTournamentBadge() {
  var b = document.getElementById('tournament-badge');
  if (b) b.style.display = 'none';
}

// ── Seen-state helpers ────────────────────────────────────────────────────────

function _tSeenKey() {
  return 't_seen_' + (currentUser ? currentUser.id : 'anon');
}
function _getSeenTournaments() {
  try { return JSON.parse(localStorage.getItem(_tSeenKey()) || '[]'); } catch(e) { return []; }
}
function _markTournamentsSeen(ids) {
  var seen = _getSeenTournaments();
  ids.forEach(function(id) { if (seen.indexOf(id) === -1) seen.push(id); });
  localStorage.setItem(_tSeenKey(), JSON.stringify(seen));
}

async function initTournamentBadge() {
  try {
    var res = await apiFetch('/api/tournaments');
    if (!res.ok) return;
    var tournaments = await res.json();
    if (!tournaments.length) return;
    var seen = _getSeenTournaments();
    var hasUnseen = tournaments.some(function(t) {
      return seen.indexOf(t.id) === -1 && (t.is_joined || t.status === 'upcoming');
    });
    if (hasUnseen) showTournamentBadge();
  } catch(e) {}
}

async function initTournamentsTab() {
  hideTournamentBadge();
  if (_activeTournamentArena) {
    _renderTournamentArenaView(_activeTournamentArena.id, _activeTournamentArena.questions, _activeTournamentArena.endsAt);
    return;
  }
  await loadTournamentList();
  try {
    var res2 = await apiFetch('/api/tournaments');
    if (res2.ok) {
      var all = await res2.json();
      _markTournamentsSeen(all.map(function(t) { return t.id; }));
    }
  } catch(e) {}
}

// ── Load & render tournament list ─────────────────────────────────────────────

async function loadTournamentList() {
  var container = document.getElementById('tournament-list');
  if (!container) return;
  container.innerHTML = '<p class="loading-text">Loading tournaments…</p>';

  try {
    var res = await apiFetch('/api/tournaments');
    if (!res.ok) { container.innerHTML = '<p class="error-text">Failed to load.</p>'; return; }
    var tournaments = await res.json();

    if (tournaments.length === 0) {
      container.innerHTML = '<p class="empty-text">No tournaments yet. Check back soon!</p>';
      return;
    }

    var upcoming  = tournaments.filter(function(t) { return t.status === 'upcoming'; });
    var active    = tournaments.filter(function(t) { return t.status === 'active'; });
    var completed = tournaments.filter(function(t) { return t.status === 'completed'; });

    var html = '';
    if (active.length)    html += '<h4 class="section-label">🔴 Live Now</h4>'    + active.map(renderTournamentCard).join('');
    if (upcoming.length)  html += '<h4 class="section-label">📅 Upcoming</h4>'    + upcoming.map(renderTournamentCard).join('');
    if (completed.length) html += '<h4 class="section-label">✅ Completed</h4>'   + completed.map(renderTournamentCard).join('');

    container.innerHTML = html;
  } catch(e) {
    container.innerHTML = '<p class="error-text">Network error.</p>';
  }
}

function renderTournamentCard(t) {
  var time      = new Date(t.scheduled_at).toLocaleString();
  var spotsLeft = t.max_participants - t.participant_count;
  var full      = spotsLeft <= 0;
  var dur       = t.duration_minutes || 60;

  var statusBadge = '<span class="status-badge status-' + t.status + '">' + t.status + '</span>';

  var participantBar =
    '<div class="participant-bar">' +
      '<div class="participant-fill" style="width:' +
        Math.min(100, Math.round(t.participant_count / t.max_participants * 100)) + '%"></div>' +
    '</div>' +
    '<span class="participant-count">' + t.participant_count + '/' + t.max_participants + '</span>';

  var actions = '';
  if (t.status === 'upcoming') {
    if (t.is_joined) {
      actions = '<button class="btn btn-ghost btn-sm" onclick="leaveTournament(' + t.id + ')">Leave</button>';
    } else if (!full) {
      actions = '<button class="btn btn-primary btn-sm" onclick="joinTournament(' + t.id + ')">Join</button>';
    } else {
      actions = '<button class="btn btn-ghost btn-sm" disabled>Full</button>';
    }
  }
  if (t.status === 'active' && t.is_joined) {
    actions = '<button class="btn btn-primary btn-sm" onclick="enterTournamentArena(' + t.id + ')">Enter Arena</button>';
  }
  if (t.status === 'completed') {
    actions = '<button class="btn btn-secondary btn-sm" onclick="viewTournamentLeaderboard(' + t.id + ')">Leaderboard</button>';
  }

  return '<div class="tournament-card" id="tournament-' + t.id + '">' +
    '<div class="tournament-card-header">' +
      '<span class="tournament-title">' + escHtml(t.title) + '</span>' +
      statusBadge +
    '</div>' +
    (t.description ? '<p class="tournament-desc">' + escHtml(t.description) + '</p>' : '') +
    '<div class="tournament-meta">📅 ' + escHtml(time) + ' &nbsp;·&nbsp; ⏱️ ' + dur + ' min</div>' +
    participantBar +
    (actions ? '<div class="tournament-actions">' + actions + '</div>' : '') +
  '</div>';
}

// ── Join / Leave ──────────────────────────────────────────────────────────────

function joinTournament(id) {
  showConfirmModal(
    '📅 Join Tournament?',
    'Make sure you have no other contests scheduled at that time.',
    'Join',
    async function() {
      try {
        var res = await apiFetch('/api/tournaments/' + id + '/join', { method: 'POST' });
        if (!res.ok) {
          var err = await res.text();
          showToast(err.trim() || 'Failed to join.', 'error');
          return;
        }
        showToast('You\'re registered! ✅');
        loadTournamentList();
      } catch(e) { showToast('Network error.', 'error'); }
    }
  );
}

async function leaveTournament(id) {
  try {
    var res = await apiFetch('/api/tournaments/' + id + '/leave', { method: 'DELETE' });
    if (!res.ok) { showToast('Failed to leave.', 'error'); return; }
    showToast('You\'ve left the tournament.');
    loadTournamentList();
  } catch(e) { showToast('Network error.', 'error'); }
}

// ── Enter Arena ───────────────────────────────────────────────────────────────

async function enterTournamentArena(id) {
  try {
    var res = await apiFetch('/api/tournaments/' + id + '/enter', { method: 'POST' });
    if (!res.ok) {
      var errText = await res.text();
      showToast(errText.trim() || 'Could not enter arena.', 'error');
      return;
    }
    var data = await res.json();

    _activeTournamentArena = { id: id, endsAt: data.ends_at, questions: data.questions };

    _renderTournamentArenaView(id, data.questions, data.ends_at);
  } catch(e) { showToast('Network error.', 'error'); }
}

function _renderTournamentArenaView(tournamentId, questions, endsAt) {
  var container = document.getElementById('tournament-list');
  if (!container) return;

  container.innerHTML = _buildArenaHTML('tournament', tournamentId, questions, endsAt);

  if (typeof AntiCheat !== 'undefined') AntiCheat.start('tournament', tournamentId);

  ContestTimer.resume('tournament', tournamentId, endsAt, {
    onEnd: function() { _exitTournamentArena('Time\'s up! Tournament ended for you.'); }
  });

  _initArenaQuestionList('tournament', tournamentId, questions);
}

function _exitTournamentArena(message) {
  if (typeof AntiCheat !== 'undefined') AntiCheat.stop();
  ContestTimer.clear();
  _activeTournamentArena = null;
  if (message) showToast(message);
  var dqModal = document.getElementById('dq-modal');
  if (dqModal) dqModal.remove();
  loadTournamentList();
}

// ── Leaderboard ───────────────────────────────────────────────────────────────

async function viewTournamentLeaderboard(id) {
  try {
    var res = await apiFetch('/api/tournaments/' + id + '/leaderboard');
    if (!res.ok) { showToast('Could not load leaderboard.', 'error'); return; }
    var data = await res.json();
    showLeaderboardModal(data);
  } catch(e) { showToast('Network error.', 'error'); }
}

function showLeaderboardModal(data) {
  var existing = document.getElementById('tournament-lb-modal');
  if (existing) existing.remove();

  var me = currentUser ? currentUser.id : 0;
  var rows = (data.ranks || []).map(function(r) {
    var medal = r.rank === 1 ? '🥇' : r.rank === 2 ? '🥈' : r.rank === 3 ? '🥉' : r.rank + '.';
    var isMe  = r.user_id === me ? ' lb-row-me' : '';
    var dqBadge = r.disqualified ? ' <span class="badge badge-danger" title="Disqualified">DQ</span>' : '';
    var scoreCell = r.disqualified
      ? '<td class="lb-score" style="color:var(--color-danger,#e53);text-decoration:line-through">' + r.score + ' solved</td>'
      : '<td class="lb-score">' + r.score + ' solved</td>';
    return '<tr class="lb-row' + isMe + (r.disqualified ? ' lb-row-dq' : '') + '">' +
      '<td class="lb-rank">'  + medal + '</td>' +
      '<td class="lb-name">'  + escHtml(r.name) + dqBadge + '</td>' +
      scoreCell +
    '</tr>';
  }).join('');

  var el = document.createElement('div');
  el.id = 'tournament-lb-modal';
  el.className = 'modal-overlay';
  el.innerHTML =
    '<div class="modal-box modal-wide">' +
      '<h3>🏆 ' + escHtml(data.title || 'Tournament') + ' — Results</h3>' +
      '<table class="lb-table">' +
        '<thead><tr><th>Rank</th><th>Player</th><th>Score</th></tr></thead>' +
        '<tbody>' + rows + '</tbody>' +
      '</table>' +
      '<div class="modal-actions">' +
        '<button class="btn btn-primary" onclick="document.getElementById(\'tournament-lb-modal\').remove()">Close</button>' +
      '</div>' +
    '</div>';
  document.body.appendChild(el);
}

// ── Notification handlers ─────────────────────────────────────────────────────

onTournamentStart = function(p) {
  showToast('🚀 Tournament #' + p.tournament_id + ' has started! Click Enter Arena when ready.');
  showTournamentBadge();
  loadTournamentList();
};

onTournamentEnd = function(p) {
  if (_activeTournamentArena && _activeTournamentArena.id === p.tournament_id) {
    _exitTournamentArena('⏱️ Tournament over! Loading results…');
  } else {
    ContestTimer.clear();
    showTournamentBadge();
    loadTournamentList();
  }
  setTimeout(function() {
    if (p.tournament_id) viewTournamentLeaderboard(p.tournament_id);
  }, 1500);
};

// ── Admin: create tournament panel ────────────────────────────────────────────

function openCreateTournamentModal() {
  var existing = document.getElementById('create-tournament-modal');
  if (existing) existing.remove();

  var el = document.createElement('div');
  el.id = 'create-tournament-modal';
  el.className = 'modal-overlay';
  el.innerHTML =
    '<div class="modal-box">' +
      '<h3>Create Tournament</h3>' +
      '<label>Title</label>' +
      '<input type="text" id="t-title" class="form-input" placeholder="e.g. Weekly Go Cup">' +
      '<label>Description (optional)</label>' +
      '<textarea id="t-desc" class="form-input" rows="2"></textarea>' +
      '<label>Start date &amp; time (your local time)</label>' +
      '<input type="datetime-local" id="t-scheduled" class="form-input">' +
      '<label>Max participants</label>' +
      '<input type="number" id="t-max" class="form-input" value="16" min="2" max="256">' +
      '<div id="t-error" class="error-text" style="display:none"></div>' +
      '<div class="modal-actions">' +
        '<button class="btn btn-primary" onclick="submitCreateTournament()">Create</button>' +
        '<button class="btn btn-ghost"   onclick="document.getElementById(\'create-tournament-modal\').remove()">Cancel</button>' +
      '</div>' +
    '</div>';
  document.body.appendChild(el);
}

async function submitCreateTournament() {
  var title       = document.getElementById('t-title').value.trim();
  var desc        = document.getElementById('t-desc').value.trim();
  var scheduledAt = new Date(document.getElementById('t-scheduled').value).toISOString();
  var maxP        = parseInt(document.getElementById('t-max').value) || 16;
  var errEl       = document.getElementById('t-error');
  errEl.style.display = 'none';

  if (!title) {
    errEl.textContent = 'Title is required.';
    errEl.style.display = 'block';
    return;
  }

  try {
    var res = await apiFetch('/api/tournaments', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        title:            title,
        description:      desc,
        scheduled_at:     scheduledAt,
        max_participants: maxP
      })
    });
    var data = await res.json();
    if (!res.ok) {
      errEl.textContent = data || 'Failed to create tournament.';
      errEl.style.display = 'block';
      return;
    }
    document.getElementById('create-tournament-modal').remove();
    showToast('Tournament created! ✅ Now assign questions.');
    loadTournamentList();
    openAssignQuestionsModal('tournament', data.id);
  } catch(e) {
    errEl.textContent = 'Network error.';
    errEl.style.display = 'block';
  }
}

// ── Shared assign-questions modal ─────────────────────────────────────────────

async function openAssignQuestionsModal(type, id) {
  var existing = document.getElementById('assign-q-modal');
  if (existing) existing.remove();

  var res = await apiFetch('/api/questions');
  if (!res.ok) { showToast('Could not load questions.', 'error'); return; }
  var questions = await res.json();

  var checkboxes = questions.map(function(q) {
    return '<label class="assign-q-row">' +
      '<input type="checkbox" class="assign-q-cb" value="' + q.id + '"> ' +
      escHtml(q.title) +
      ' <span class="difficulty-badge diff-' + q.difficulty + '">' + q.difficulty + '</span>' +
    '</label>';
  }).join('');

  var el = document.createElement('div');
  el.id = 'assign-q-modal';
  el.className = 'modal-overlay';
  el.innerHTML =
    '<div class="modal-box modal-wide">' +
      '<h3>Assign Questions — ' + (type === 'challenge' ? 'Challenge' : 'Tournament') + ' #' + id + '</h3>' +
      '<p class="modal-hint">Select the questions participants will solve. Order follows selection sequence.</p>' +
      '<div class="assign-q-list">' + checkboxes + '</div>' +
      '<div style="margin-top:16px">' +
        '<label class="form-label">Duration (minutes)</label>' +
        '<input type="number" id="assign-duration" class="form-input" value="60" min="5" max="480" style="width:120px">' +
      '</div>' +
      '<div id="assign-q-error" class="error-text" style="display:none"></div>' +
      '<div class="modal-actions">' +
        '<button class="btn btn-primary" onclick="submitAssignQuestions(\'' + type + '\',' + id + ')">Save</button>' +
        '<button class="btn btn-ghost"   onclick="document.getElementById(\'assign-q-modal\').remove()">Cancel</button>' +
      '</div>' +
    '</div>';
  document.body.appendChild(el);
}

async function submitAssignQuestions(type, id) {
  var checked = Array.from(document.querySelectorAll('.assign-q-cb:checked'))
                     .map(function(cb) { return parseInt(cb.value); });
  var duration = parseInt(document.getElementById('assign-duration').value) || 60;
  var errEl = document.getElementById('assign-q-error');
  errEl.style.display = 'none';

  if (checked.length === 0) {
    errEl.textContent = 'Select at least one question.';
    errEl.style.display = 'block';
    return;
  }

  var endpoint = type === 'challenge'
    ? '/api/admin/challenges/' + id + '/questions'
    : '/api/admin/tournaments/' + id + '/questions';

  try {
    var res = await apiFetch(endpoint, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ question_ids: checked, duration_minutes: duration })
    });
    if (!res.ok) {
      errEl.textContent = 'Failed to assign questions.';
      errEl.style.display = 'block';
      return;
    }
    document.getElementById('assign-q-modal').remove();
    showToast('Questions assigned! ✅ Duration: ' + duration + ' min');
  } catch(e) {
    errEl.textContent = 'Network error.';
    errEl.style.display = 'block';
  }
}