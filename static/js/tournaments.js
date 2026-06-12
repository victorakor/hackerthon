// ── Tournaments ───────────────────────────────────────────────────────────────

// ── Entry point ───────────────────────────────────────────────────────────────

var _activeTournamentID = null;

// Badge helpers for the Tournaments tab
function showTournamentBadge() {
  var b = document.getElementById('tournament-badge');
  if (b) b.style.display = 'inline-block';
}
function hideTournamentBadge() {
  var b = document.getElementById('tournament-badge');
  if (b) b.style.display = 'none';
}

// localStorage seen-state for tournaments
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

// Called on enterApp — lights the badge if user has unseen tournaments they're joined in,
// and silently resumes an active arena only if it was already open this session
async function initTournamentBadge() {
  try {
    var res = await apiFetch('/api/tournaments');
    if (!res.ok) return;
    var tournaments = await res.json();
    if (!tournaments.length) return;

    // Only show badge for tournaments the user joined (or any new tournament for non-participants)
    var joined = tournaments.filter(function(t) { return t.is_joined; });
    var seen = _getSeenTournaments();

    // Badge for participants: any joined tournament they haven't seen
    var hasUnseen = joined.some(function(t) { return seen.indexOf(t.id) === -1; });

    // Badge for non-participants: any brand-new (upcoming) tournament they haven't seen
    if (!hasUnseen) {
      var upcoming = tournaments.filter(function(t) { return t.status === 'upcoming' && seen.indexOf(t.id) === -1; });
      hasUnseen = upcoming.length > 0;
    }

    if (hasUnseen) showTournamentBadge();

    // Only auto-resume active arena if it was already open in this session
    var active = joined.find(function(t) { return t.status === 'active'; });
    if (!active) return;

    var arenaKey = 't_arena_' + (currentUser ? currentUser.id : 'anon');
    var arenaActive = sessionStorage.getItem(arenaKey);
    if (!arenaActive) return;  // fresh login — don't auto-popup

    var detail = await (await apiFetch('/api/tournaments/' + active.id)).json();
    _activeTournamentID = active.id;
    ContestTimer.resume('tournament', active.id, active.scheduled_at,
      detail.question_ids || [],
      {
        onStart: function() {},
        onEnd:   function() { onTournamentEnd({ tournament_id: active.id }); }
      }
    );
    showTournamentArena(active.id, detail.question_ids || [], active.scheduled_at);
  } catch(e) {}
}

async function initTournamentsTab() {
  // Clear the alert badge — user has now opened the tab
  hideTournamentBadge();
  await loadTournamentList();
  // Mark all currently-visible tournaments as seen
  try {
    var res2 = await apiFetch('/api/tournaments');
    if (res2.ok) {
      var all = await res2.json();
      _markTournamentsSeen(all.map(function(t) { return t.id; }));
    }
  } catch(e) {}
  await checkForActiveTournament();
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

    // Group by status
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
  var time     = new Date(t.scheduled_at).toLocaleString();
  var spotsLeft = t.max_participants - t.participant_count;
  var full      = spotsLeft <= 0;

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
    '<div class="tournament-meta">📅 ' + escHtml(time) + ' &nbsp;·&nbsp; ⏱️ 1 hour</div>' +
    participantBar +
    (actions ? '<div class="tournament-actions">' + actions + '</div>' : '') +
  '</div>';
}

// ── Join / Leave ──────────────────────────────────────────────────────────────

function joinTournament(id) {
  // Find the card already rendered so we can show the time in the confirm
  var card = document.getElementById('tournament-' + id);
  var timeText = card ? card.querySelector('.tournament-meta') : null;
  var scheduledLabel = timeText ? timeText.textContent.replace('📅', '').replace('⏱️ 1 hour', '').replace('·', '').trim() : '';

  showConfirmModal(
    '📅 Join Tournament?',
    scheduledLabel ? 'This tournament starts on <strong>' + escHtml(scheduledLabel) + '</strong> and lasts 1 hour.<br>Make sure you have no challenges or other tournaments scheduled at that time.' : 'Make sure you have no other contests scheduled at that time.',
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

// ── Check for active tournament on load ───────────────────────────────────────

async function checkForActiveTournament() {
  try {
    var res = await apiFetch('/api/tournaments');
    if (!res.ok) return;
    var tournaments = await res.json();

    // Joined and active
    var active = tournaments.find(function(t) {
      return t.status === 'active' && t.is_joined;
    });
    if (!active) return;

    // Mark session so reload within the same session resumes the arena
    var arenaKey = 't_arena_' + (currentUser ? currentUser.id : 'anon');
    sessionStorage.setItem(arenaKey, '1');

    var detail = await (await apiFetch('/api/tournaments/' + active.id)).json();
    _activeTournamentID = active.id;
    ContestTimer.resume('tournament', active.id, active.scheduled_at,
      detail.question_ids || [],
      {
        onStart: function() {},
        onEnd:   function() { onTournamentEnd({ tournament_id: active.id }); }
      }
    );
    showTournamentArena(active.id, detail.question_ids || [], active.scheduled_at);
  } catch(e) {}
}

// ── Enter arena ───────────────────────────────────────────────────────────────

async function enterTournamentArena(id) {
  try {
    var res = await apiFetch('/api/tournaments/' + id);
    if (!res.ok) { showToast('Could not load tournament.', 'error'); return; }
    var detail = await res.json();
    _activeTournamentID = id;

    if (detail.status === 'active') {
      showTournamentArena(id, detail.question_ids || [], detail.scheduled_at);
    } else {
      ContestTimer.schedule('tournament', id, detail.scheduled_at, detail.question_ids || [], {
        onStart: function(qids) { showTournamentArena(id, qids, detail.scheduled_at); },
        onEnd:   function()     { onTournamentEnd({ tournament_id: id }); }
      });
      showToast('Tournament starts at ' + new Date(detail.scheduled_at).toLocaleTimeString());
    }
  } catch(e) { showToast('Network error.', 'error'); }
}

function showTournamentArena(tournamentID, questionIDs, scheduledAt) {
  if (typeof switchTab === 'function') switchTab('questions');
  if (typeof enterContestMode === 'function') {
    enterContestMode('tournament', tournamentID, questionIDs, scheduledAt);
  }
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
  var arenaKey = 't_arena_' + (currentUser ? currentUser.id : 'anon');
  sessionStorage.setItem(arenaKey, '1');
  showToast('🚀 Tournament #' + p.tournament_id + ' has started! Get ready!');
  enterTournamentArena(p.tournament_id);
};

onTournamentEnd = function(p) {
  ContestTimer.clear();
  var arenaKey = 't_arena_' + (currentUser ? currentUser.id : 'anon');
  sessionStorage.removeItem(arenaKey);
  showToast('⏱️ Tournament over! Loading results…');
  if (typeof exitContestMode === 'function') exitContestMode();
  // Show badge so user knows there's a new result to view
  showTournamentBadge();
  setTimeout(function() {
    if (p.tournament_id) viewTournamentLeaderboard(p.tournament_id);
    loadTournamentList();
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

    // Immediately open question assignment for this tournament
    openAssignQuestionsModal('tournament', data.id);
  } catch(e) {
    errEl.textContent = 'Network error.';
    errEl.style.display = 'block';
  }
}

// ── Shared assign-questions modal (used by both challenges and tournaments) ───

async function openAssignQuestionsModal(type, id) {
  var existing = document.getElementById('assign-q-modal');
  if (existing) existing.remove();

  // Load all available questions
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
      body: JSON.stringify({ question_ids: checked })
    });
    if (!res.ok) {
      errEl.textContent = 'Failed to assign questions.';
      errEl.style.display = 'block';
      return;
    }
    document.getElementById('assign-q-modal').remove();
    showToast('Questions assigned! ✅');
  } catch(e) {
    errEl.textContent = 'Network error.';
    errEl.style.display = 'block';
  }
}