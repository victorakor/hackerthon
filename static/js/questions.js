async function loadQuestions() {
  var res = await apiFetch('/api/questions');
  if (!res.ok) return;
  questions = await res.json();
  var saved = [];
  try { saved = JSON.parse(localStorage.getItem('completed_' + currentUser.id) || '[]'); } catch(e) {}
  completedIds = new Set(saved);
  assignQuestionTimers();
  renderQuestionList();
  updateProgress();
}

function renderQuestionList() {
  var list = document.getElementById('question-list');
  var visible = currentUser.is_admin ? questions : questions.filter(function(q) { return q.visible; });
  var filtered = currentFilter === 'all' ? visible : visible.filter(function(q) { return q.difficulty === currentFilter; });
  list.innerHTML = filtered.map(function(q, i) {
    var isActive = currentQuestion && currentQuestion.id === q.id;
    var isComplete = completedIds.has(q.id);
    return '<div class="q-item' + (isComplete ? ' completed' : '') + (isActive ? ' active' : '') +
      '" data-id="' + q.id + '" onclick="selectQuestion(' + q.id + ')">' +
      '<span class="q-num">' + (i + 1) + '</span>' +
      '<div class="q-info">' +
        '<div class="q-title">' + escHtml(q.title) + '</div>' +
        '<div class="q-meta">' +
          '<span class="diff-badge diff-' + q.difficulty + '">' + q.difficulty + '</span>' +
          '<span class="cat-tag">' + escHtml(q.category) + '</span>' +
          (currentUser.is_admin && !q.visible ? '<span class="hidden-tag">hidden</span>' : '') +
        '</div>' +
        '<div class="q-timer" data-id="' + q.id + '">' + qTimerStr(q.id) + '</div>' +
      '</div></div>';
  }).join('');
}

function setFilter(f, btn) {
  currentFilter = f;
  document.querySelectorAll('.filter-chip').forEach(function(c) { c.classList.remove('active'); });
  btn.classList.add('active');
  renderQuestionList();
}

function updateProgress() {
  var visible = questions.filter(function(q) { return q.visible; });
  var total = visible.length;
  var done = visible.filter(function(q) { return completedIds.has(q.id); }).length;
  document.getElementById('progress-count').textContent = done + ' / ' + total + ' solved';
  document.getElementById('progress-bar').style.width = total > 0 ? ((done / total) * 100) + '%' : '0%';
  document.getElementById('easy-count').textContent = questions.filter(function(q) { return q.difficulty === 'easy' && q.visible; }).length + ' Easy';
  document.getElementById('medium-count').textContent = questions.filter(function(q) { return q.difficulty === 'medium' && q.visible; }).length + ' Medium';
  document.getElementById('hard-count').textContent = questions.filter(function(q) { return q.difficulty === 'hard' && q.visible; }).length + ' Hard';
  document.getElementById('header-answered').textContent = done + ' solved';
}

function testInfoHtml(q) {
  if (q.test_file && q.test_file.trim() !== '') {
    return '<span class="go-test-badge">&#x1F9EA; Go test file</span>';
  }
  var testCaseCount = 0;
  try {
    var tc = JSON.parse(q.test_cases || '[]');
    testCaseCount = Array.isArray(tc) ? tc.length : 0;
  } catch(e) {}
  if (testCaseCount > 0) {
    return '<span style="font-family:var(--font-mono);font-size:10px;color:var(--muted)">' + testCaseCount + ' test case(s) available</span>';
  }
  return '<span class="no-tests-note" style="margin-top:0">no test cases \u2014 run shows raw output</span>';
}

async function selectQuestion(id) {
  currentQuestion = questions.find(function(q) { return q.id === id; });
  if (!currentQuestion) return;
  renderQuestionList();

  var q = currentQuestion;
  var isComplete = completedIds.has(id);
  var isAdmin = currentUser.is_admin;

  var pane = document.getElementById('detail-pane');
  pane.innerHTML =
    '<div class="detail-card">' +
      '<div class="detail-header">' +
        '<div class="detail-title-group">' +
          '<div class="detail-number">Question #' + id + '</div>' +
          '<div class="detail-title">' + escHtml(q.title) + '</div>' +
          '<div class="detail-badges">' +
            '<span class="badge badge-' + q.difficulty + '">' + q.difficulty + '</span>' +
            '<span class="badge badge-cat">' + escHtml(q.category) + '</span>' +
            (!q.visible ? '<span class="badge badge-hidden">Hidden from users</span>' : '') +
          '</div>' +
        '</div>' +
        '<div class="detail-actions">' +
          '<button class="btn-action btn-hint" onclick="toggleHint()">&#x1F4A1; Hint</button>' +
          '<button class="btn-action btn-complete' + (isComplete ? ' done' : '') + '" id="complete-btn" onclick="toggleComplete(' + id + ')">' +
            (isComplete ? '&#x2713; Completed' : 'Mark Complete') +
          '</button>' +
          (isAdmin ?
            '<button class="btn-action ' + (q.visible ? 'btn-vis' : 'btn-vis hidden') + '" id="vis-btn" onclick="adminToggleVisibility(' + id + ')">' +
              (q.visible ? '&#x1F441; Visible' : '&#x1F441; Hidden') +
            '</button>' +
            '<button class="btn-action btn-danger" onclick="adminDeleteQuestion(' + id + ')">&#x1F5D1; Delete</button>'
          : '') +
        '</div>' +
      '</div>' +
      '<div class="detail-body">' +
        '<div class="detail-description">' + escHtml(q.description) + '</div>' +
        '<div class="detail-hint" id="hint-box">' +
          '<div class="hint-label">&#x1F4A1; Hint</div>' +
          '<div class="hint-text">' + escHtml(q.hint_text) + ' &mdash; ' +
            '<a class="hint-link" href="' + escHtml(q.hint_url) + '" target="_blank" rel="noopener">Open docs &#x2197;</a>' +
          '</div>' +
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
              '<option value="typescript">TypeScript</option>' +
              '<option value="java">Java</option>' +
              '<option value="c">C</option>' +
              '<option value="cpp">C++</option>' +
              '<option value="rust">Rust</option>' +
              '<option value="bash">Bash</option>' +
            '</select>' +
          '</div>' +
          '<div>' +
            '<label class="form-label">Notes (optional)</label>' +
            '<input class="form-input" id="sub-notes" placeholder="Approach, time complexity\u2026"/>' +
          '</div>' +
        '</div>' +
        '<div>' +
          '<label class="form-label">Code</label>' +
          '<div id="sub-code-editor"></div>' +
        '</div>' +
        '<div style="margin-top:12px;display:flex;gap:10px;align-items:center;flex-wrap:wrap">' +
          '<button class="btn-run" id="run-btn" onclick="runTests(' + id + ')">&#x25BA; Run Tests</button>' +
          '<button class="btn-send" id="submit-btn" onclick="submitSolution(' + id + ')" disabled style="opacity:.4;cursor:not-allowed">Submit Solution &#x2192;</button>' +
          testInfoHtml(q) +
        '</div>' +
        '<div id="test-results-container"></div>' +
      '</div>' +
    '</div>' +

    '<div>' +
      '<div class="section-title">Solutions</div>' +
      '<div class="submissions-list" id="submissions-list">' +
        '<div style="font-family:var(--font-mono);font-size:12px;color:var(--muted);text-align:center;padding:20px">Loading\u2026</div>' +
      '</div>' +
    '</div>';

  // Boot the CodeMirror editor now that the DOM is ready
  initCodeEditor();

  loadSubmissions(id);
}

function toggleHint() {
  var box = document.getElementById('hint-box');
  if (box) box.classList.toggle('visible');
}

function toggleComplete(id) {
  if (completedIds.has(id)) return;
  completedIds.add(id);
  saveCompleted();
  updateProgress();
  renderQuestionList();
  var btn = document.getElementById('complete-btn');
  if (btn) { btn.innerHTML = '&#x2713; Completed'; btn.classList.add('done'); }
  showToast('Marked as complete!', 'success');
}

function saveCompleted() {
  localStorage.setItem('completed_' + currentUser.id, JSON.stringify(Array.from(completedIds)));
}