async function loadAdminPanel() {
  try {
    var results = await Promise.all([apiFetch('/api/admin/users'), apiFetch('/api/questions')]);
    var usersRes = results[0], qRes = results[1];
    if (!usersRes.ok || !qRes.ok) return;
    var users = await usersRes.json();
    var qs = await qRes.json();

    document.getElementById('admin-user-list').innerHTML = users.map(function(u) {
      return '<div class="admin-item">' +
        '<div class="admin-item-info">' +
          '<div class="admin-item-name">' + escHtml(u.name) +
            (u.is_admin ? ' <span style="font-size:10px;color:var(--accent3)">[ADMIN]</span>' : '') +
          '</div>' +
          '<div class="admin-item-sub">' + escHtml(u.email) + '</div>' +
        '</div>' +
        '<div class="admin-item-actions">' +
          (u.id !== currentUser.id ?
            '<button class="btn-sm ' + (u.is_admin ? 'btn-sm-muted' : 'btn-sm-success') + '" onclick="adminToggleAdmin(' + u.id + ',' + !u.is_admin + ')">' +
              (u.is_admin ? 'Demote' : 'Make Admin') +
            '</button>' +
            '<button class="btn-sm btn-sm-danger" onclick="adminDeleteUser(' + u.id + ')">Delete</button>'
          : '<span style="font-family:var(--font-mono);font-size:10px;color:var(--muted)">You</span>') +
        '</div>' +
      '</div>';
    }).join('');

    document.getElementById('admin-q-list').innerHTML = qs.map(function(q) {
      var testLabel = q.test_file && q.test_file.trim() !== '' ? 'Go test' :
        (function() {
          try { var tc = JSON.parse(q.test_cases || '[]'); return Array.isArray(tc) && tc.length ? tc.length + ' case(s)' : 'no tests'; } catch(e) { return 'no tests'; }
        })();
      return '<div class="admin-item">' +
        '<div class="admin-item-info">' +
          '<div class="admin-item-name">' + escHtml(q.title) + '</div>' +
          '<div class="admin-item-sub">' + q.difficulty + ' \u00B7 ' + escHtml(q.category) + ' \u00B7 ' + testLabel + '</div>' +
        '</div>' +
        '<div class="admin-item-actions">' +
          '<button class="btn-sm ' + (q.visible ? 'btn-sm-success' : 'btn-sm-muted') + '" onclick="adminToggleVisibilityFromPanel(' + q.id + ',' + !q.visible + ',this)">' +
            (q.visible ? '&#x1F441; Visible' : '&#x1F441; Hidden') +
          '</button>' +
          '<button class="btn-sm btn-sm-danger" onclick="adminDeleteQuestion(' + q.id + ')">Delete</button>' +
        '</div>' +
      '</div>';
    }).join('');
    loadAdminContestSections();
  } catch(e) {}
}

async function adminDeleteUser(id) {
  if (!confirm('Delete this user? This cannot be undone.')) return;
  try {
    var res = await apiFetch('/api/admin/users', {
      method: 'DELETE',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({id: id})
    });
    if (res.ok) { showToast('User deleted', 'success'); loadAdminPanel(); }
    else { showToast(await res.text(), 'error'); }
  } catch(e) { showToast('Error deleting user', 'error'); }
}

async function adminToggleAdmin(id, makeAdmin) {
  try {
    var res = await apiFetch('/api/admin/promote', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({id: id, is_admin: makeAdmin})
    });
    if (res.ok) { showToast(makeAdmin ? 'Promoted to admin' : 'Demoted', 'success'); loadAdminPanel(); }
    else { showToast(await res.text(), 'error'); }
  } catch(e) {}
}

async function adminToggleVisibility(qid) {
  var q = questions.find(function(x) { return x.id === qid; });
  if (!q) return;
  var newVis = !q.visible;
  try {
    var res = await apiFetch('/api/questions/visibility', {
      method: 'PATCH',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({id: qid, visible: newVis})
    });
    if (res.ok) {
      q.visible = newVis;
      if (newVis) assignQuestionTimers();
      showToast(newVis ? 'Question is now visible' : 'Question hidden', 'success');
      var btn = document.getElementById('vis-btn');
      if (btn) {
        btn.innerHTML = newVis ? '&#x1F441; Visible' : '&#x1F441; Hidden';
        btn.className = 'btn-action ' + (newVis ? 'btn-vis' : 'btn-vis hidden');
      }
      renderQuestionList();
    } else { showToast(await res.text(), 'error'); }
  } catch(e) {}
}

async function adminToggleVisibilityFromPanel(qid, newVis, btn) {
  try {
    var res = await apiFetch('/api/questions/visibility', {
      method: 'PATCH',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({id: qid, visible: newVis})
    });
    if (res.ok) {
      var q = questions.find(function(x) { return x.id === qid; });
      if (q) { q.visible = newVis; if (newVis) assignQuestionTimers(); }
      btn.innerHTML = newVis ? '&#x1F441; Visible' : '&#x1F441; Hidden';
      btn.className = 'btn-sm ' + (newVis ? 'btn-sm-success' : 'btn-sm-muted');
      showToast(newVis ? 'Question visible' : 'Question hidden', 'success');
      renderQuestionList();
    } else { showToast(await res.text(), 'error'); }
  } catch(e) {}
}

async function adminDeleteQuestion(qid) {
  if (!confirm('Delete this question? All submissions will also be deleted.')) return;
  try {
    var res = await apiFetch('/api/questions/delete', {
      method: 'DELETE',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({id: qid})
    });
    if (res.ok) {
      questions = questions.filter(function(q) { return q.id !== qid; });
      delete questionTimers[qid];
      showToast('Question deleted', 'success');
      if (currentQuestion && currentQuestion.id === qid) {
        document.getElementById('detail-pane').innerHTML =
          '<div class="empty-state"><div class="empty-state-icon">{_}</div><div class="empty-state-text">Question deleted</div></div>';
        currentQuestion = null;
      }
      renderQuestionList();
      updateProgress();
      var adminTab = document.getElementById('tab-admin');
      if (adminTab && adminTab.style.display !== 'none') loadAdminPanel();
    } else { showToast(await res.text(), 'error'); }
  } catch(e) { showToast('Error deleting question', 'error'); }
}

async function adminAddQuestion() {
  var title = document.getElementById('aq-title').value.trim();
  var description = document.getElementById('aq-desc').value.trim();
  var difficulty = document.getElementById('aq-diff').value;
  var category = document.getElementById('aq-cat').value.trim() || 'General';
  var hint_url = document.getElementById('aq-hurl').value.trim() || 'https://pkg.go.dev';
  var hint_text = document.getElementById('aq-htxt').value.trim() || 'Go documentation';
  var testCasesRaw = document.getElementById('aq-tests').value.trim() || '[]';
  var testFile = document.getElementById('aq-testfile').value.trim();

  if (!title || !description) { showToast('Title and description required', 'error'); return; }

  if (testCasesRaw !== '[]') {
    try {
      var parsed = JSON.parse(testCasesRaw);
      if (!Array.isArray(parsed)) throw new Error('not an array');
    } catch(e) {
      showToast('Test cases must be a valid JSON array, e.g. [{"input":"hi","expected":"HI"}]', 'error');
      return;
    }
  }

  var body = {
    title: title, description: description, difficulty: difficulty,
    category: category, hint_url: hint_url, hint_text: hint_text,
    test_cases: testCasesRaw
  };
  if (testFile) body.test_file = testFile;

  try {
    var res = await apiFetch('/api/questions', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body)
    });
    if (res.ok) {
      var q = await res.json();
      questions.push(q);
      showToast('Question added', 'success');
      ['aq-title','aq-desc','aq-cat','aq-hurl','aq-htxt','aq-tests','aq-testfile'].forEach(function(id) {
        document.getElementById(id).value = '';
      });
      renderQuestionList();
      loadAdminPanel();
    } else { showToast(await res.text(), 'error'); }
  } catch(e) { showToast('Error adding question', 'error'); }
}

// Called at end of loadAdminPanel — load pending challenges and tournaments
async function loadAdminContestSections() {
  // Challenges needing question assignment
  loadAdminChallenges();

  // Tournament list with assign-questions buttons
  var container = document.getElementById('admin-tournaments-list');
  if (!container) return;
  try {
    var res = await apiFetch('/api/tournaments');
    if (!res.ok) return;
    var tournaments = await res.json();
    if (!tournaments || tournaments.length === 0) {
      container.innerHTML = '<p class="empty-text">No tournaments yet.</p>';
      return;
    }
    container.innerHTML = tournaments.map(function(t) {
      return '<div class="admin-item">' +
        '<span>' + escHtml(t.title) + '</span>' +
        '<span class="status-badge status-' + t.status + '">' + t.status + '</span>' +
        '<span>' + new Date(t.scheduled_at).toLocaleString() + '</span>' +
        '<span>' + t.participant_count + '/' + t.max_participants + '</span>' +
        (t.status === 'upcoming' ?
          '<button class="btn btn-sm btn-primary" onclick="openAssignQuestionsModal(\'tournament\',' + t.id + ')">Assign Questions</button>' : '') +
      '</div>';
    }).join('');
  } catch(e) {}
}