// ── Arena shared view ─────────────────────────────────────────────────────────
// Builds and manages the isolated arena UI used by both challenges and tournaments.
// Questions appear only here — never in the general Questions tab.

var _arenaContestType = null;  // 'challenge' | 'tournament'
var _arenaContestId   = null;
var _arenaQuestions   = [];    // full Question objects
var _arenaCurrentQ    = null;  // currently selected question in arena
var _arenaSolvedIds   = new Set();

// Build the full arena HTML panel.
function _buildArenaHTML(type, contestId, questions, endsAt) {
  var typeLabel = type === 'challenge' ? 'Challenge' : 'Tournament';
  var qListHTML = questions.map(function(q, i) {
    return '<div class="arena-q-item" id="arena-q-' + q.id + '" data-qid="' + q.id + '" onclick="_arenaSelectQuestion(' + q.id + ')">' +
      '<span class="q-num">' + (i + 1) + '</span>' +
      '<div class="q-info">' +
        '<div class="q-title">' + escHtml(q.title) + '</div>' +
        '<div class="q-meta">' +
          '<span class="diff-badge diff-' + q.difficulty + '">' + q.difficulty + '</span>' +
          '<span class="cat-tag">' + escHtml(q.category) + '</span>' +
        '</div>' +
      '</div>' +
      '<span class="arena-q-status" id="arena-qs-' + q.id + '"></span>' +
    '</div>';
  }).join('');

  return '<div class="arena-panel">' +
    '<div class="arena-header">' +
      '<div class="arena-title">⚔️ ' + escHtml(typeLabel) + ' #' + contestId + ' — Arena</div>' +
      '<div id="contest-countdown" class="contest-countdown" style="display:none"></div>' +
      '<button class="btn btn-ghost btn-sm" onclick="_arenaExit()">Exit Arena</button>' +
    '</div>' +
    '<div class="arena-body">' +
      '<div class="arena-q-list">' + qListHTML + '</div>' +
      '<div class="arena-detail" id="arena-detail">' +
        '<p class="arena-hint-text">← Select a question to start solving.</p>' +
      '</div>' +
    '</div>' +
  '</div>';
}

// Called after HTML is injected — wires up contest-type/id globals.
function _initArenaQuestionList(type, contestId, questions) {
  _arenaContestType = type;
  _arenaContestId   = contestId;
  _arenaQuestions   = questions;
  _arenaCurrentQ    = null;
  _arenaSolvedIds   = new Set();
}

// Select a question inside the arena — renders full detail with editor, hints, AI hint.
function _arenaSelectQuestion(qid) {
  _arenaCurrentQ = _arenaQuestions.find(function(q) { return q.id === qid; });
  if (!_arenaCurrentQ) return;

  // Keep currentQuestion in sync so hint.js works correctly
  currentQuestion = _arenaCurrentQ;

  // Highlight active item in sidebar
  document.querySelectorAll('.arena-q-item').forEach(function(el) { el.classList.remove('active'); });
  var item = document.getElementById('arena-q-' + qid);
  if (item) item.classList.add('active');

  var q = _arenaCurrentQ;
  var isSolved = _arenaSolvedIds.has(qid);

  // Test info badge
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

  var detail = document.getElementById('arena-detail');
  detail.innerHTML =
    // ── Question card ──────────────────────────────────────────────────────
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
          '<button class="btn-action btn-hint" onclick="toggleArenaHint()">💡 Hint</button>' +
          '<button class="btn-action btn-ai-hint" id="ai-hint-btn" onclick="requestAiHint(' + q.id + ')">' +
            '🤖 AI Hint <span class="ai-hint-badge" id="ai-hint-badge">⌛</span>' +
          '</button>' +
        '</div>' +
      '</div>' +
      '<div class="detail-body">' +
        '<div class="detail-description">' + escHtml(q.description) + '</div>' +
        // Static hint (toggled by button)
        '<div class="detail-hint" id="hint-box" style="display:none">' +
          '<div class="hint-label">💡 Hint</div>' +
          '<div class="hint-text">' + escHtml(q.hint_text) + ' — ' +
            '<a class="hint-link" href="' + escHtml(q.hint_url) + '" target="_blank" rel="noopener">Open docs ↗</a>' +
          '</div>' +
        '</div>' +
        // AI hint output panel
        '<div class="ai-hint-panel" id="ai-hint-panel" style="display:none">' +
          '<div class="ai-hint-header">' +
            '<span class="ai-hint-label">🤖 AI Nudge</span>' +
            '<button class="ai-hint-close" onclick="document.getElementById(\'ai-hint-panel\').style.display=\'none\'">✕</button>' +
          '</div>' +
          '<div class="ai-hint-text" id="ai-hint-text"></div>' +
        '</div>' +
      '</div>' +
    '</div>' +

    // ── Code editor + submit ───────────────────────────────────────────────
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
          '<button class="btn-send" id="submit-btn" onclick="_arenaSubmit(' + q.id + ')" disabled style="opacity:.4;cursor:not-allowed">Submit ➜</button>' +
          testInfoHtml +
        '</div>' +
        '<div id="test-results-container"></div>' +
      '</div>' +
    '</div>';

  // Boot the CodeMirror editor
  initCodeEditor();

  // Load AI hint status for this question
  if (typeof initHintStatus === 'function') initHintStatus(q.id);
}

// Toggle static hint box in arena.
function toggleArenaHint() {
  var box = document.getElementById('hint-box');
  if (!box) return;
  box.style.display = (box.style.display === 'none' || box.style.display === '') ? 'block' : 'none';
}

// Called from the arena submit button.
async function _arenaSubmit(qid) {
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

  var endpoint = _arenaContestType === 'challenge'
    ? '/api/challenges/' + _arenaContestId + '/arena-submit'
    : '/api/tournaments/' + _arenaContestId + '/arena-submit';

  try {
    var res = await apiFetch(endpoint, {
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

    // Render test results
    var container = document.getElementById('test-results-container');
    if (container && data.run_result && typeof renderTestResults === 'function') {
      renderTestResults(data.run_result, container);
    }

    if (data.passed) {
      _arenaSolvedIds.add(qid);
      var statusEl = document.getElementById('arena-qs-' + qid);
      if (statusEl) { statusEl.textContent = '✓'; statusEl.style.color = 'var(--easy)'; }
      var listItem = document.getElementById('arena-q-' + qid);
      if (listItem) listItem.classList.add('completed');
      showToast('✅ Question solved!', 'success');
    } else {
      showToast('Tests did not all pass. Keep trying!', 'error');
      submitBtn.disabled = false;
      submitBtn.style.opacity = '';
      submitBtn.style.cursor = '';
      submitBtn.textContent = 'Submit ➜';
    }

    if (data.finished) {
      showToast('🎉 All questions solved! You\'ve completed this ' + _arenaContestType + '!', 'success');
      setTimeout(function() {
        if (_arenaContestType === 'challenge') {
          _exitChallengeArena('Well done! Waiting for the contest to fully conclude…');
        } else {
          _exitTournamentArena('Well done! Waiting for the contest to fully conclude…');
        }
      }, 2000);
    }

    if (data.disqualified) {
      if (_arenaContestType === 'challenge') {
        _exitChallengeArena('You have been disqualified.');
      } else {
        _exitTournamentArena('You have been disqualified.');
      }
    }

  } catch(e) {
    showToast('Network error during submission.', 'error');
    submitBtn.disabled = false;
    submitBtn.style.opacity = '';
    submitBtn.style.cursor = '';
    submitBtn.textContent = 'Submit ➜';
  }
}

// Called from the Exit Arena button.
function _arenaExit() {
  showConfirmModal(
    'Exit Arena?',
    'Exiting the arena does not end the challenge — you can re-enter while time remains. Your progress is saved.',
    'Exit',
    function() {
      currentQuestion = null;
      if (_arenaContestType === 'challenge') {
        _exitChallengeArena(null);
      } else {
        _exitTournamentArena(null);
      }
    }
  );
}