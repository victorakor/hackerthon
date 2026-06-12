// ── Challenges ────────────────────────────────────────────────────────────────

var _activeChallengeID = null;  // challenge we are currently competing in

// ── Entry point ───────────────────────────────────────────────────────────────

async function initChallengesTab() {
  await loadChallengeList();
  await checkForActiveChallenge();
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

  var statusBadge = '<span class="status-badge status-' + c.status + '">' + c.status + '</span>';

  var actions = '';
  if (c.status === 'pending' && !isChallenger) {
    actions =
      '<button class="btn btn-success btn-sm" onclick="acceptChallenge(' + c.id + ')">Accept</button> ' +
      '<button class="btn btn-danger  btn-sm" onclick="rejectChallenge(' + c.id + ')">Decline</button>';
  }
  if (c.status === 'active') {
    actions = '<button class="btn btn-primary btn-sm" onclick="enterChallengeArena(' + c.id + ')">Enter Arena</button>';
  }
  if (c.status === 'completed') {
    actions = '<button class="btn btn-secondary btn-sm" onclick="viewChallengeResult(' + c.id + ')">View Result</button>';
  }

  return '<div class="challenge-card" id="challenge-' + c.id + '">' +
    '<div class="challenge-card-header">' +
      '<span class="challenge-vs">' + escHtml(role) + ' <strong>' + escHtml(opponent) + '</strong></span>' +
      statusBadge +
    '</div>' +
    '<div class="challenge-meta">📅 ' + escHtml(time) + '</div>' +
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
  document.getElementById('challenge-opponent-id').value   = opponentID;
  document.getElementById('challenge-opponent-name').textContent = escHtml(opponentName);

  // Default to tomorrow at current time (rounded to nearest 15 min)
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
      '<p class="modal-hint">The challenge lasts exactly 1 hour from this time.</p>' +
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

  // Convert local datetime-local input to UTC ISO string
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
  // Find the challenge data already rendered so we can show the scheduled time in the confirm dialog
  var card = document.getElementById('challenge-' + id);
  var timeText = card ? card.querySelector('.challenge-meta') : null;
  var scheduledLabel = timeText ? timeText.textContent.replace('📅', '').trim() : '';

  showConfirmModal(
    '⚔️ Accept Challenge?',
    scheduledLabel ? 'This challenge is scheduled for <strong>' + escHtml(scheduledLabel) + '</strong>.<br>Make sure you have no other contests at that time — accepting will lock the slot.' : 'Make sure you have no other contests at that time — accepting will lock the slot.',
    'Accept',
    async function() {
      try {
        var res = await apiFetch('/api/challenges/' + id + '/accept', { method: 'PATCH' });
        if (!res.ok) {
          var errText = await res.text();
          showToast((errText.trim() || 'Failed to accept.'), 'error');
          return;
        }
        showToast('Challenge accepted! The admin will assign questions.');
        loadChallengeList();
      } catch(e) { showToast('Network error.', 'error'); }
    }
  );
}

async function rejectChallenge(id) {
  try {
    var res = await apiFetch('/api/challenges/' + id + '/reject', { method: 'PATCH' });
    if (!res.ok) { showToast('Failed to decline.', 'error'); return; }
    showToast('Challenge declined.');
    loadChallengeList();
  } catch(e) { showToast('Network error.', 'error'); }
}

// ── Check for active challenge on load ────────────────────────────────────────

async function checkForActiveChallenge() {
  try {
    var res = await apiFetch('/api/challenges');
    if (!res.ok) return;
    var challenges = await res.json();
    var active = challenges.find(function(c) { return c.status === 'active'; });
    if (!active) return;

    // Resume the timer — contest is already running
    var detail = await (await apiFetch('/api/challenges/' + active.id)).json();
    _activeChallengeID = active.id;
    ContestTimer.resume('challenge', active.id, active.scheduled_at,
      detail.question_ids || [],
      {
        onStart: function() {},   // already active
        onEnd:   function() { onContestEnd({ challenge_id: active.id }); }
      }
    );
    showChallengeArena(active.id, detail.question_ids || [], active.scheduled_at);
  } catch(e) {}
}

// ── Enter / show arena ────────────────────────────────────────────────────────

async function enterChallengeArena(id) {
  try {
    var res = await apiFetch('/api/challenges/' + id);
    if (!res.ok) { showToast('Could not load challenge.', 'error'); return; }
    var detail = await res.json();
    _activeChallengeID = id;

    if (detail.status === 'active') {
      showChallengeArena(id, detail.question_ids || [], detail.scheduled_at);
    } else {
      // Not started yet — schedule the timer
      ContestTimer.schedule('challenge', id, detail.scheduled_at, detail.question_ids || [], {
        onStart: function(qids) { showChallengeArena(id, qids, detail.scheduled_at); },
        onEnd:   function()     { onContestEnd({ challenge_id: id }); }
      });
      showToast('Challenge starts at ' + new Date(detail.scheduled_at).toLocaleTimeString());
    }
  } catch(e) { showToast('Network error.', 'error'); }
}

function showChallengeArena(challengeID, questionIDs, scheduledAt) {
  // Switch to the Questions tab in contest mode
  if (typeof switchTab === 'function') switchTab('questions');
  if (typeof enterContestMode === 'function') {
    enterContestMode('challenge', challengeID, questionIDs, scheduledAt);
  }
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

  var winnerLine = '';
  if (r.winner_id) {
    winnerLine = '<p class="result-winner">🏆 Winner: <strong>' + escHtml(r.winner_name) + '</strong></p>';
  } else {
    winnerLine = '<p class="result-winner">🤝 It\'s a Tie!</p>';
  }

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

// ── Notification handlers (overrides stubs in contest_timer.js) ───────────────

onChallengeReceived = function(p) {
  showNotifBadge();
  showToast('⚔️ ' + escHtml(p.challenger_name || 'Someone') + ' challenged you! Check the Challenges tab.');
  loadChallengeList();
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
  // Admin-only toast
  if (currentUser && currentUser.is_admin) {
    showToast('📋 Challenge #' + p.challenge_id + ' needs questions assigned.', 'info');
  }
};

onContestStart = function(p) {
  if (!p.challenge_id) return;
  showToast('🚀 Challenge #' + p.challenge_id + ' has started!');
  enterChallengeArena(p.challenge_id);
};

onContestEnd = function(p) {
  ContestTimer.clear();
  showToast('⏱️ Challenge over! Fetching results…');
  if (typeof exitContestMode === 'function') exitContestMode();
  setTimeout(function() {
    if (p.challenge_id) viewChallengeResult(p.challenge_id);
    loadChallengeList();
  }, 1500);
};

// ── Admin: assign questions panel ─────────────────────────────────────────────

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
  // Returns "YYYY-MM-DDTHH:MM" in local time for datetime-local inputs
  var pad = function(n) { return String(n).padStart(2,'0'); };
  return date.getFullYear() + '-' + pad(date.getMonth()+1) + '-' + pad(date.getDate()) +
    'T' + pad(date.getHours()) + ':' + pad(date.getMinutes());
}