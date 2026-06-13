// ── Challenges ────────────────────────────────────────────────────────────────

var _activeChallengeArena = null; // { id, endsAt, questions }

// ── Seen-state helpers ────────────────────────────────────────────────────────

function _chSeenKey() {
  return 'ch_seen_' + (currentUser ? currentUser.id : 'anon');
}
function _getSeenChallenges() {
  try { return JSON.parse(localStorage.getItem(_chSeenKey()) || '[]'); } catch(e) { return []; }
}
function _markChallengesSeen(ids) {
  var seen = _getSeenChallenges();
  ids.forEach(function(id) { if (seen.indexOf(id) === -1) seen.push(id); });
  localStorage.setItem(_chSeenKey(), JSON.stringify(seen));
}

async function initChallengeBadge() {
  try {
    var res = await apiFetch('/api/challenges');
    if (!res.ok) return;
    var challenges = await res.json();
    if (!challenges.length) return;
    var seen = _getSeenChallenges();
    var hasUnseen = challenges.some(function(c) { return seen.indexOf(c.id) === -1; });
    if (hasUnseen) showNotifBadge();
  } catch(e) {}
}

async function initChallengesTab() {
  hideNotifBadge();
  // If there's an active arena open, show it instead of the list
  if (_activeChallengeArena) {
    _renderChallengeArenaView(_activeChallengeArena.id, _activeChallengeArena.questions, _activeChallengeArena.endsAt);
    return;
  }
  await loadChallengeList();
  try {
    var res2 = await apiFetch('/api/challenges');
    if (res2.ok) {
      var all = await res2.json();
      _markChallengesSeen(all.map(function(c) { return c.id; }));
    }
  } catch(e) {}
}

// ── Load & render challenge list ──────────────────────────────────────────────

async function loadChallengeList() {
  var container = document.getElementById('challenge-list');
  if (!container) return;
  container.innerHTML = '<p class="loading-text">Loading challenges…</p>';

  try {
    var res = await apiFetch('/api/challenges');
    if (!res.ok) { container.innerHTML = '<p class="error-text">Failed to load.</p>'; return; }
    var challenges = await res.json();

    if (challenges.length === 0) {
      container.innerHTML = '<p class="empty-text">No challenges yet. Challenge someone from the leaderboard!</p>';
      return;
    }

    container.innerHTML = challenges.map(renderChallengeCard).join('');
  } catch(e) {
    container.innerHTML = '<p class="error-text">Network error.</p>';
  }
}

function renderChallengeCard(c) {
  var me = currentUser ? currentUser.id : 0;
  var isChallenger = c.challenger_id === me;
  var opponent = isChallenger ? c.opponent_name : c.challenger_name;
  var role     = isChallenger ? 'You challenged' : 'Challenged by';
  var time     = new Date(c.scheduled_at).toLocaleString();
  var dur      = c.duration_minutes || 60;

  var statusBadge = '<span class="status-badge status-' + c.status + '">' + c.status + '</span>';

  var actions = '';
  if (c.status === 'pending' && !isChallenger) {
    actions =
      '<button class="btn btn-success btn-sm" onclick="acceptChallenge(' + c.id + ')">Accept</button> ' +
      '<button class="btn btn-danger  btn-sm" onclick="rejectChallenge(' + c.id + ')">Decline</button>';
  }
  if (c.status === 'active') {
    // Check if this user is already finished/disqualified for this challenge
    var iAmFinished = isChallenger
      ? (c.challenger_finished_at != null)
      : (c.opponent_finished_at != null);
    if (!iAmFinished) {
      actions = '<button class="btn btn-primary btn-sm" onclick="enterChallengeArena(' + c.id + ')">Enter Arena</button>';
    } else {
      actions = '<span class="muted-text">Awaiting opponent…</span>';
    }
  }
  if (c.status === 'completed') {
    actions = '<button class="btn btn-secondary btn-sm" onclick="viewChallengeResult(' + c.id + ')">View Result</button>';
  }

  return '<div class="challenge-card" id="challenge-' + c.id + '">' +
    '<div class="challenge-card-header">' +
      '<span class="challenge-vs">' + escHtml(role) + ' <strong>' + escHtml(opponent) + '</strong></span>' +
      statusBadge +
    '</div>' +
    '<div class="challenge-meta">📅 ' + escHtml(time) + ' &nbsp;·&nbsp; ⏱️ ' + dur + ' min</div>' +
    (c.status === 'completed' ?
      '<div class="challenge-scores">' +
        escHtml(c.challenger_name) + ': <strong>' + c.challenger_score + '</strong> &nbsp;|&nbsp; ' +
        escHtml(c.opponent_name)   + ': <strong>' + c.opponent_score   + '</strong>' +
        (c.winner_id ? ' &nbsp;🏆 ' + escHtml(c.winner_id === c.challenger_id ? c.challenger_name : c.opponent_name) : ' &nbsp;🤝 Tie') +
      '</div>' : '') +
    (actions ? '<div class="challenge-actions">' + actions + '</div>' : '') +
  '</div>';
}

// ── Send challenge ────────────────────────────────────────────────────────────

function openChallengeModal(opponentID, opponentName) {
  var modal = document.getElementById('challenge-modal');
  if (!modal) {
    modal = buildChallengeModal();
    document.body.appendChild(modal);
  }
  document.getElementById('challenge-opponent-id').value = opponentID;
  document.getElementById('challenge-opponent-name').textContent = escHtml(opponentName);

  var d = new Date(Date.now() + 24 * 3600 * 1000);
  d.setMinutes(Math.ceil(d.getMinutes() / 15) * 15, 0, 0);
  document.getElementById('challenge-time-input').value = toLocalDatetimeInput(d);

  modal.style.display = 'flex';
}

function buildChallengeModal() {
  var el = document.createElement('div');
  el.id = 'challenge-modal';
  el.className = 'modal-overlay';
  el.innerHTML =
    '<div class="modal-box">' +
      '<h3>⚔️ Challenge <span id="challenge-opponent-name"></span></h3>' +
      '<input type="hidden" id="challenge-opponent-id">' +
      '<label>Schedule date &amp; time (your local time)</label>' +
      '<input type="datetime-local" id="challenge-time-input" class="form-input">' +
      '<p class="modal-hint">The admin will set the challenge duration when assigning questions.</p>' +
      '<div id="challenge-modal-error" class="error-text" style="display:none"></div>' +
      '<div class="modal-actions">' +
        '<button class="btn btn-primary" onclick="sendChallenge()">Send Challenge</button>' +
        '<button class="btn btn-ghost"   onclick="closeChallengeModal()">Cancel</button>' +
      '</div>' +
    '</div>';
  return el;
}

function closeChallengeModal() {
  var modal = document.getElementById('challenge-modal');
  if (modal) modal.style.display = 'none';
}

async function sendChallenge() {
  var opponentID = parseInt(document.getElementById('challenge-opponent-id').value);
  var localDt    = document.getElementById('challenge-time-input').value;
  var errEl      = document.getElementById('challenge-modal-error');
  errEl.style.display = 'none';

  if (!localDt) {
    errEl.textContent = 'Please pick a date and time.';
    errEl.style.display = 'block';
    return;
  }

  var scheduledAt = new Date(localDt).toISOString();

  try {
    var res = await apiFetch('/api/challenges', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ opponent_id: opponentID, scheduled_at: scheduledAt })
    });
    if (!res.ok) {
      var errText = await res.text();
      errEl.textContent = errText.trim() || 'Failed to send challenge.';
      errEl.style.display = 'block';
      return;
    }
    closeChallengeModal();
    showToast('Challenge sent! ⚔️');
    loadChallengeList();
  } catch(e) {
    errEl.textContent = 'Network error.';
    errEl.style.display = 'block';
  }
}

// ── Accept / Reject ───────────────────────────────────────────────────────────

function acceptChallenge(id) {
  showConfirmModal(
    '⚔️ Accept Challenge?',
    'Are you sure you want to accept this challenge?',
    'Accept',
    async function() {
      try {
        var res = await apiFetch('/api/challenges/' + id + '/accept', { method: 'PATCH' });
        if (!res.ok) {
          var err = await res.text();
          showToast(err.trim() || 'Failed to accept.', 'error');
          return;
        }
        showToast('Challenge accepted! ⚔️');
        loadChallengeList();
      } catch(e) { showToast('Network error.', 'error'); }
    }
  );
}

function rejectChallenge(id) {
  showConfirmModal(
    'Decline Challenge?',
    'Are you sure you want to decline this challenge?',
    'Decline',
    async function() {
      try {
        var res = await apiFetch('/api/challenges/' + id + '/reject', { method: 'PATCH' });
        if (!res.ok) { showToast('Failed to decline.', 'error'); return; }
        showToast('Challenge declined.');
        loadChallengeList();
      } catch(e) { showToast('Network error.', 'error'); }
    }
  );
}

// ── Enter Arena ───────────────────────────────────────────────────────────────

async function enterChallengeArena(id) {
  try {
    var res = await apiFetch('/api/challenges/' + id + '/enter', { method: 'POST' });
    if (!res.ok) {
      var errText = await res.text();
      showToast(errText.trim() || 'Could not enter arena.', 'error');
      return;
    }
    var data = await res.json();
    // data: { ends_at, duration_minutes, questions: [...] }

    _activeChallengeArena = { id: id, endsAt: data.ends_at, questions: data.questions };

    _renderChallengeArenaView(id, data.questions, data.ends_at);
  } catch(e) { showToast('Network error.', 'error'); }
}

function _renderChallengeArenaView(challengeId, questions, endsAt) {
  var container = document.getElementById('challenge-list');
  if (!container) return;

  // Render the arena panel inside the challenges tab
  container.innerHTML = _buildArenaHTML('challenge', challengeId, questions, endsAt);

  // Start anti-cheat
  if (typeof AntiCheat !== 'undefined') AntiCheat.start('challenge', challengeId);

  // Start timer
  ContestTimer.resume('challenge', challengeId, endsAt, {
    onEnd: function() { _exitChallengeArena('Time\'s up! Challenge ended for you.'); }
  });

  // Wire up question selection inside arena
  _initArenaQuestionList('challenge', challengeId, questions);
}

function _exitChallengeArena(message) {
  if (typeof AntiCheat !== 'undefined') AntiCheat.stop();
  ContestTimer.clear();
  _activeChallengeArena = null;
  if (message) showToast(message);
  // Remove any DQ modal if open
  var dqModal = document.getElementById('dq-modal');
  if (dqModal) dqModal.remove();
  loadChallengeList();
}

// ── View result ───────────────────────────────────────────────────────────────

async function viewChallengeResult(id) {
  try {
    var res = await apiFetch('/api/challenges/' + id + '/result');
    if (!res.ok) { showToast('Could not load result.', 'error'); return; }
    var result = await res.json();
    showResultModal(result);
  } catch(e) { showToast('Network error.', 'error'); }
}

function showResultModal(r) {
  var existing = document.getElementById('result-modal');
  if (existing) existing.remove();

  var winnerLine = r.winner_id
    ? '<p class="result-winner">🏆 Winner: <strong>' + escHtml(r.winner_name) + '</strong></p>'
    : '<p class="result-winner">🤝 It\'s a Tie!</p>';

  var el = document.createElement('div');
  el.id = 'result-modal';
  el.className = 'modal-overlay';
  el.innerHTML =
    '<div class="modal-box">' +
      '<h3>Challenge Result</h3>' +
      '<div class="result-scores">' +
        '<div class="result-player">' +
          '<div class="result-name">' + escHtml(r.challenger_name) + '</div>' +
          '<div class="result-score">' + r.challenger_score + '</div>' +
        '</div>' +
        '<div class="result-vs">vs</div>' +
        '<div class="result-player">' +
          '<div class="result-name">' + escHtml(r.opponent_name) + '</div>' +
          '<div class="result-score">' + r.opponent_score + '</div>' +
        '</div>' +
      '</div>' +
      winnerLine +
      '<div class="modal-actions">' +
        '<button class="btn btn-primary" onclick="document.getElementById(\'result-modal\').remove()">Close</button>' +
      '</div>' +
    '</div>';
  document.body.appendChild(el);
}

// ── Notification handlers ─────────────────────────────────────────────────────

onChallengeReceived = function(p) {
  showNotifBadge();
  showToast('⚔️ ' + escHtml(p.challenger_name || 'Someone') + ' challenged you! Check the Challenges tab.');
};

onChallengeAccepted = function(p) {
  showNotifBadge();
  showToast('✅ ' + escHtml(p.opponent_name || 'Your opponent') + ' accepted your challenge!');
  loadChallengeList();
};

onChallengeRejected = function(p) {
  showToast('❌ ' + escHtml(p.opponent_name || 'Your opponent') + ' declined the challenge.');
  loadChallengeList();
};

onChallengeNeedsQuestions = function(p) {
  if (currentUser && currentUser.is_admin) {
    showToast('📋 Challenge #' + p.challenge_id + ' needs questions assigned.', 'info');
  }
};

onContestStart = function(p) {
  if (!p.challenge_id) return;
  showToast('🚀 Challenge #' + p.challenge_id + ' has started! Click Enter Arena when ready.');
  showNotifBadge();
  loadChallengeList();
};

onContestEnd = function(p) {
  if (_activeChallengeArena && _activeChallengeArena.id === p.challenge_id) {
    _exitChallengeArena('⏱️ Challenge over! Fetching results…');
  } else {
    ContestTimer.clear();
    showNotifBadge();
    loadChallengeList();
  }
};

// ── Admin ─────────────────────────────────────────────────────────────────────

async function loadAdminChallenges() {
  var container = document.getElementById('admin-challenges-list');
  if (!container) return;

  try {
    var res = await apiFetch('/api/admin/challenges');
    if (!res.ok) return;
    var challenges = await res.json();

    var needsAction = challenges.filter(function(c) {
      return c.status === 'accepted' || c.status === 'pending';
    });

    if (needsAction.length === 0) {
      container.innerHTML = '<p class="empty-text">No challenges need attention.</p>';
      return;
    }

    container.innerHTML = needsAction.map(function(c) {
      return '<div class="admin-challenge-row">' +
        '<span>' + escHtml(c.challenger_name) + ' vs ' + escHtml(c.opponent_name) + '</span>' +
        '<span class="status-badge status-' + c.status + '">' + c.status + '</span>' +
        '<span>📅 ' + new Date(c.scheduled_at).toLocaleString() + '</span>' +
        '<button class="btn btn-sm btn-primary" onclick="openAssignQuestionsModal(\'challenge\',' + c.id + ')">Assign Questions</button>' +
      '</div>';
    }).join('');
  } catch(e) {}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function toLocalDatetimeInput(date) {
  var pad = function(n) { return String(n).padStart(2,'0'); };
  return date.getFullYear() + '-' + pad(date.getMonth()+1) + '-' + pad(date.getDate()) +
    'T' + pad(date.getHours()) + ':' + pad(date.getMinutes());
}