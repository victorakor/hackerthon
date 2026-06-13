// ── Raid Arena ───────────────────────────────────────────────────────────────
// Live multi-clan coding battle. Reuses CodeMirror editor + test runner UI
// patterns from arena.js, but adds a live clan scoreboard and per-clan scoring.

var _raidArenaId       = null;
var _raidQuestions     = [];
var _raidCurrentQ      = null;
var _raidSolvedIds     = new Set();
var _raidEndsAt        = null;
var _raidCountdownHandle = null;
var _raidScorePollHandle = null;

// Entry point — called from raids.js "Enter Arena" button
async function initRaidArena(raidID) {
  try {
    var res = await apiFetch('/api/raids/' + raidID + '/enter', { method: 'POST' });
    var data = await res.json();
    if (!res.ok) {
      // If user is disqualified or finished, show results view instead
      if (res.status === 403 &&
          (data.error === 'you have been disqualified from this raid' ||
           data.error === 'you have already finished this raid')) {
        _renderRaidResultsView(raidID, data.error);
        return;
      }
      showToast((data && data.error) || 'Could not enter raid arena.', 'error');
      return;
    }

    _raidArenaId   = raidID;
    _raidQuestions = data.questions || [];
    _raidCurrentQ  = null;
    _raidSolvedIds = new Set();
    _raidEndsAt    = data.ends_at;

    _renderRaidArena(data);

  // Enable contest-mode anti-cheat (blocks copy/paste, tab-switch)
  if (typeof AntiCheat !== 'undefined') {
    AntiCheat.start('raid', raidID);
  }
  } catch (e) {
    showToast('Network error entering raid arena.', 'error');
  }
}

function _renderRaidArena(data) {
  var container = document.getElementById('clan-view');
  if (!container) return;

  var qListHTML = _raidQuestions.map(function(q, i) {
    return '<div class="arena-q-item" id="raid-q-' + q.id + '" data-qid="' + q.id + '" onclick="_raidSelectQuestion(' + q.id + ')">' +
      '<span class="q-num">' + (i + 1) + '</span>' +
      '<div class="q-info">' +
        '<div class="q-title">' + escHtml(q.title) + '</div>' +
        '<div class="q-meta">' +
          '<span class="diff-badge diff-' + q.difficulty + '">' + q.difficulty + '</span>' +
          '<span class="cat-tag">' + escHtml(q.category) + '</span>' +
        '</div>' +
      '</div>' +
      '<span class="arena-q-status" id="raid-qs-' + q.id + '"></span>' +
    '</div>';
  }).join('');

  container.innerHTML =
    '<div class="arena-panel raid-arena-panel">' +
      '<div class="arena-header">' +
        '<div class="arena-title">⚔️ Raid #' + _raidArenaId + ' — Arena</div>' +
        '<div id="raid-countdown" class="contest-countdown"></div>' +
        '<button class="btn btn-ghost btn-sm" onclick="_raidArenaExit()">Exit Arena</button>' +
      '</div>' +
      '<div class="raid-scoreboard" id="raid-scoreboard"></div>' +
      '<div class="arena-body">' +
        '<div class="arena-q-list">' + qListHTML + '</div>' +
        '<div class="arena-detail" id="raid-arena-detail">' +
          '<p class="arena-hint-text">← Select a question to start solving.</p>' +
        '</div>' +
      '</div>' +
    '</div>';

  _renderRaidScoreboard(data.clan_scores || []);
  _startRaidCountdown();

  // Poll scoreboard every 5s so members see live clan progress
  if (_raidScorePollHandle) clearInterval(_raidScorePollHandle);
  _raidScorePollHandle = setInterval(_refreshRaidScoreboard, 5000);
}

function _renderRaidScoreboard(scores) {
  var board = document.getElementById('raid-scoreboard');
  if (!board) return;

  var myClanID = window._myClan ? window._myClan.id : null;

  board.innerHTML = '<div class="raid-scoreboard-title">🏆 Live Clan Scores</div>' +
    '<div class="raid-scoreboard-rows">' +
    scores.map(function(s) {
      var mineClass = s.clan_id === myClanID ? ' raid-clan-mine' : '';
      return '<div class="raid-scoreboard-row' + mineClass + '">' +
        '<span class="raid-clan-rank">#' + s.rank + '</span>' +
        '<span class="raid-clan-name">' + escHtml(s.clan_name) + '</span>' +
        '<span class="raid-clan-score">' + s.score + ' pts</span>' +
      '</div>';
    }).join('') +
    '</div>';
}

function _refreshRaidScoreboard() {
  if (!_raidArenaId) return;
  apiFetch('/api/raids/' + _raidArenaId + '/leaderboard')
    .then(function(r) { return r.json(); })
    .then(function(scores) {
      _renderRaidScoreboard(scores || []);
    })
    .catch(function() {});
}

function _startRaidCountdown() {
  if (_raidCountdownHandle) clearInterval(_raidCountdownHandle);
  if (!_raidEndsAt) return;

  var target = new Date(_raidEndsAt).getTime();

  function tick() {
    var el = document.getElementById('raid-countdown');
    if (!el) {
      clearInterval(_raidCountdownHandle);
      return;
    }
    var diff = target - Date.now();
    if (diff <= 0) {
      el.textContent = '⏱ Raid ended';
      el.style.display = '';
      clearInterval(_raidCountdownHandle);
      showToast('Raid has ended!', 'success');
      if (typeof AntiCheat !== 'undefined') AntiCheat.stop();
      // Exit directly — no confirm modal needed since the raid is already over.
      setTimeout(function() {
        currentQuestion = null;
        if (_raidScorePollHandle) clearInterval(_raidScorePollHandle);
        _raidArenaId = null;
        _raidQuestions = [];
        _raidCurrentQ = null;
        renderClanTab();
      }, 2500);
      return;
    }
    var mins = Math.floor(diff / 60000);
    var secs = Math.floor((diff % 60000) / 1000);
    el.textContent = '⏱ ' + mins + 'm ' + (secs < 10 ? '0' : '') + secs + 's remaining';
    el.style.display = '';
  }
  tick();
  _raidCountdownHandle = setInterval(tick, 1000);
}

function _raidSelectQuestion(qid) {
  _raidCurrentQ = _raidQuestions.find(function(q) { return q.id === qid; });
  if (!_raidCurrentQ) return;

  currentQuestion = _raidCurrentQ;

  document.querySelectorAll('.arena-q-item').forEach(function(el) { el.classList.remove('active'); });
  var item = document.getElementById('raid-q-' + qid);
  if (item) item.classList.add('active');

  var q = _raidCurrentQ;
  var isSolved = _raidSolvedIds.has(qid);

  var testInfoHtml = '';
  if (q.test_file && q.test_file.trim() !== '') {
    testInfoHtml = '<span class="go-test-badge">🧪 Go test file</span>';
  } else {
    var tcCount = 0;
    try { var tc = JSON.parse(q.test_cases || '[]'); tcCount = Array.isArray(tc) ? tc.length : 0; } catch(e) {}
    testInfoHtml = tcCount > 0
      ? '<span style="font-family:var(--font-mono);font-size:10px;color:var(--muted)">' + tcCount + ' test case(s)</span>'
      : '<span class="no-tests-note" style="margin-top:0">no test cases — run shows raw output</span>';
  }

  var detail = document.getElementById('raid-arena-detail');
  detail.innerHTML =
    '<div class="detail-card">' +
      '<div class="detail-header">' +
        '<div class="detail-title-group">' +
          '<div class="detail-number">Question #' + q.id + '</div>' +
          '<div class="detail-title">' + escHtml(q.title) + '</div>' +
          '<div class="detail-badges">' +
            '<span class="badge badge-' + q.difficulty + '">' + q.difficulty + '</span>' +
            '<span class="badge badge-cat">' + escHtml(q.category) + '</span>' +
            (isSolved ? '<span class="badge badge-solved">✓ Solved</span>' : '') +
          '</div>' +
        '</div>' +
        '<div class="detail-actions">' +
          '<button class="btn-action btn-hint" onclick="toggleRaidHint()">💡 Hint</button>' +
          '<button class="btn-action btn-ai-hint" id="ai-hint-btn" onclick="requestAiHint(' + q.id + ')">' +
            '🤖 AI Hint <span class="ai-hint-badge" id="ai-hint-badge">⌛</span>' +
          '</button>' +
        '</div>' +
      '</div>' +
      '<div class="detail-body">' +
        '<div class="detail-description">' + escHtml(q.description) + '</div>' +
        // Static hint box (toggled by Hint button)
        '<div class="detail-hint" id="hint-box" style="display:none">' +
          '<div class="hint-label">💡 Hint</div>' +
          '<div class="hint-text">' + escHtml(q.hint_text || '') +
            (q.hint_url ? ' — <a class="hint-link" href="' + escHtml(q.hint_url) + '" target="_blank" rel="noopener">Open docs ↗</a>' : '') +
          '</div>' +
        '</div>' +
        // AI hint streaming panel
        '<div class="ai-hint-panel" id="ai-hint-panel" style="display:none">' +
          '<div class="ai-hint-header">' +
            '<span class="ai-hint-label">🤖 AI Nudge</span>' +
            '<button class="ai-hint-close" onclick="document.getElementById(\'ai-hint-panel\').style.display=\'none\'">✕</button>' +
          '</div>' +
          '<div class="ai-hint-text" id="ai-hint-text"></div>' +
        '</div>' +
      '</div>' +
    '</div>' +

    '<div>' +
      '<div class="section-title">Submit Solution</div>' +
      '<div class="submit-form">' +
        '<div class="form-row">' +
          '<div>' +
            '<label class="form-label">Language</label>' +
            '<select class="form-select" id="sub-lang">' +
              '<option value="go">Go</option>' +
              '<option value="python">Python</option>' +
              '<option value="javascript">JavaScript</option>' +
              '<option value="bash">Bash</option>' +
            '</select>' +
          '</div>' +
        '</div>' +
        '<div>' +
          '<label class="form-label">Code</label>' +
          '<div id="sub-code-editor"></div>' +
        '</div>' +
        '<div style="margin-top:12px;display:flex;gap:10px;align-items:center;flex-wrap:wrap">' +
          '<button class="btn-run" id="run-btn" onclick="runTests(' + q.id + ')">▶ Run Tests</button>' +
          '<button class="btn-send" id="submit-btn" onclick="_raidArenaSubmit(' + q.id + ')" disabled style="opacity:.4;cursor:not-allowed">Submit ➜</button>' +
          testInfoHtml +
        '</div>' +
        '<div id="test-results-container"></div>' +
      '</div>' +
    '</div>';

  initCodeEditor();

  if (typeof initHintStatus === 'function') initHintStatus(q.id);
}

function toggleRaidHint() {
  var box = document.getElementById('hint-box');
  if (!box) return;
  box.style.display = (box.style.display === 'none' || box.style.display === '') ? 'block' : 'none';
}

async function _raidArenaSubmit(qid) {
  var submitBtn = document.getElementById('submit-btn');
  if (!submitBtn || submitBtn.disabled) {
    showToast('Run tests first and pass them all', 'error');
    return;
  }

  var code = (typeof getEditorCode === 'function') ? getEditorCode().trim() : '';
  var langEl = document.getElementById('sub-lang');
  var language = langEl ? langEl.value : 'go';

  if (!code) { showToast('Code is required', 'error'); return; }

  submitBtn.disabled = true;
  submitBtn.style.opacity = '.4';
  submitBtn.style.cursor = 'not-allowed';
  submitBtn.textContent = 'Submitting…';

  try {
    var res = await apiFetch('/api/raids/' + _raidArenaId + '/arena-submit', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ question_id: qid, code: code, language: language })
    });

    var data = await res.json();

    if (!res.ok) {
      showToast((data && data.error) || 'Submission failed.', 'error');
      submitBtn.disabled = false;
      submitBtn.style.opacity = '';
      submitBtn.style.cursor = '';
      submitBtn.textContent = 'Submit ➜';
      return;
    }

    var container = document.getElementById('test-results-container');
    if (container && data.run_result && typeof renderTestResults === 'function') {
      renderTestResults(data.run_result, container);
    }

    // Update scoreboard immediately from response
    if (data.clan_scores) _renderRaidScoreboard(data.clan_scores);

    if (data.passed) {
      _raidSolvedIds.add(qid);
      var statusEl = document.getElementById('raid-qs-' + qid);
      if (statusEl) { statusEl.textContent = '✓'; statusEl.style.color = 'var(--easy)'; }
      var listItem = document.getElementById('raid-q-' + qid);
      if (listItem) listItem.classList.add('completed');
      showToast('✅ Question solved! Your clan score went up.', 'success');

      // Mark solved badge in detail header
      var badges = document.querySelector('.detail-badges');
      if (badges && !badges.querySelector('.badge-solved')) {
        badges.insertAdjacentHTML('beforeend', '<span class="badge badge-solved">✓ Solved</span>');
      }

      // If all questions are solved, mark the user as finished and show results
      if (_raidSolvedIds.size >= _raidQuestions.length) {
        if (typeof AntiCheat !== 'undefined') AntiCheat.stop();
        try {
          await apiFetch('/api/raids/' + _raidArenaId + '/finish', { method: 'POST' });
        } catch(e) {}
        showToast('🎉 You solved all questions! Waiting for raid to end…', 'success');
        var finishedRaidId = _raidArenaId;
        if (_raidCountdownHandle) clearInterval(_raidCountdownHandle);
        if (_raidScorePollHandle) clearInterval(_raidScorePollHandle);
        _raidArenaId = null;
        _raidQuestions = [];
        _raidCurrentQ = null;
        setTimeout(function() {
          _renderRaidResultsView(finishedRaidId, 'you have already finished this raid');
        }, 1500);
        return;
      }
    } else {
      showToast('Tests did not all pass. Keep trying!', 'error');
    }

    submitBtn.disabled = false;
    submitBtn.style.opacity = '';
    submitBtn.style.cursor = '';
    submitBtn.textContent = 'Submit ➜';

  } catch (e) {
    showToast('Network error during submission.', 'error');
    submitBtn.disabled = false;
    submitBtn.style.opacity = '';
    submitBtn.style.cursor = '';
    submitBtn.textContent = 'Submit ➜';
  }
}

function _raidArenaExit() {
  showConfirmModal(
    'Exit Raid Arena?',
    'Exiting does not end the raid — you can re-enter while time remains. Your clan\'s score is saved.',
    'Exit',
    function() {
      currentQuestion = null;
      if (_raidCountdownHandle) clearInterval(_raidCountdownHandle);
      if (_raidScorePollHandle) clearInterval(_raidScorePollHandle);
      if (typeof AntiCheat !== 'undefined') AntiCheat.stop();
      _raidArenaId = null;
      _raidQuestions = [];
      _raidCurrentQ = null;
      // Return to clan tab raids view
      renderClanTab();
    }
  );
}

// Called when user is disqualified or has finished — shows a locked results view
async function _renderRaidResultsView(raidID, reason) {
  var container = document.getElementById('clan-view');
  if (!container) return;

  // Fetch current scores to display
  var scores = [];
  try {
    var r = await apiFetch('/api/raids/' + raidID + '/leaderboard');
    scores = await r.json();
  } catch(e) {}

  var isDisq = reason && reason.indexOf('disqualified') !== -1;
  var headline = isDisq
    ? '🚫 You have been disqualified from this raid.'
    : '✅ You have finished this raid.';
  var subtitle = 'The raid is still in progress. Final results will be available when it ends.';

  var scoreRows = (scores || []).map(function(s) {
    return '<div class="raid-scoreboard-row">' +
      '<span class="raid-clan-rank">#' + s.rank + '</span>' +
      '<span class="raid-clan-name">' + escHtml(s.clan_name) + '</span>' +
      '<span class="raid-clan-score">' + s.score + ' pts</span>' +
    '</div>';
  }).join('');

  container.innerHTML =
    '<div class="arena-panel raid-arena-panel" style="text-align:center;padding:40px 20px">' +
      '<div style="font-size:2rem;margin-bottom:12px">' + headline + '</div>' +
      '<div style="color:var(--muted);margin-bottom:32px">' + subtitle + '</div>' +
      '<div class="raid-scoreboard" style="max-width:400px;margin:0 auto 32px">' +
        '<div class="raid-scoreboard-title">🏆 Current Clan Scores</div>' +
        '<div class="raid-scoreboard-rows" id="raid-results-scores">' + scoreRows + '</div>' +
      '</div>' +
      '<button class="btn btn-ghost" onclick="renderClanTab()">← Back to Raids</button>' +
    '</div>';

  // Keep polling scores so they see live updates even from the waiting room
  if (_raidScorePollHandle) clearInterval(_raidScorePollHandle);
  _raidScorePollHandle = setInterval(function() {
    apiFetch('/api/raids/' + raidID + '/leaderboard')
      .then(function(r) { return r.json(); })
      .then(function(s) {
        var el = document.getElementById('raid-results-scores');
        if (!el) { clearInterval(_raidScorePollHandle); return; }
        el.innerHTML = (s || []).map(function(sc) {
          return '<div class="raid-scoreboard-row">' +
            '<span class="raid-clan-rank">#' + sc.rank + '</span>' +
            '<span class="raid-clan-name">' + escHtml(sc.clan_name) + '</span>' +
            '<span class="raid-clan-score">' + sc.score + ' pts</span>' +
          '</div>';
        }).join('');
      }).catch(function() {});
  }, 5000);
}