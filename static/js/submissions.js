async function loadSubmissions(qid) {
  try {
    var res = await apiFetch('/api/submissions?question_id=' + qid);
    if (!res.ok) throw new Error('failed');
    var subs = await res.json();
    var list = document.getElementById('submissions-list');
    if (!list) return;
    if (!subs || !subs.length) {
      list.innerHTML = '<div style="font-family:var(--font-mono);font-size:12px;color:var(--muted);text-align:center;padding:20px">No solutions yet. Be the first!</div>';
      return;
    }
    list.innerHTML = subs.map(function(s) {
      return '<div class="sub-card">' +
        '<div class="sub-header" onclick="toggleSub(' + s.id + ')">' +
          '<span class="sub-author">' + escHtml(s.author_name) + '</span>' +
          '<div class="sub-meta">' +
            '<span class="sub-lang">' + escHtml(s.language) + '</span>' +
            '<span class="stars">' + starsHtml(s.avg_rating) + '</span>' +
            '<span class="sub-count">' + s.review_count + ' review' + (s.review_count !== 1 ? 's' : '') + '</span>' +
          '</div>' +
        '</div>' +
        '<div class="sub-body" id="sub-body-' + s.id + '">' +
          '<pre class="code-block">' + escHtml(s.code) + '</pre>' +
          (s.notes ? '<div class="sub-notes">&#x1F4AC; ' + escHtml(s.notes) + '</div>' : '') +
          (function() {
            var isOwn = currentUser && s.user_id && s.user_id === currentUser.id;
            if (isOwn) return '';
            return '<div id="review-form-' + s.id + '" class="review-form">' +
              '<div class="form-label">Rate this solution</div>' +
              '<div class="stars-input" id="stars-' + s.id + '">' +
                [1,2,3,4,5].map(function(n) {
                  return '<button class="star-btn" onclick="setStar(' + s.id + ',' + n + ')">&#x2606;</button>';
                }).join('') +
              '</div>' +
              '<input class="form-input" id="comment-' + s.id + '" placeholder="Leave a comment (required)\u2026" style="font-size:12px"/>' +
              '<div><button class="btn-send" onclick="submitReview(' + s.id + ')">Post Review</button></div>' +
            '</div>';
          })() +
          '<div class="reviews-list" id="reviews-' + s.id + '">' +
            '<div style="font-family:var(--font-mono);font-size:11px;color:var(--muted)">Loading reviews\u2026</div>' +
          '</div>' +
        '</div>' +
      '</div>';
    }).join('');
  } catch(e) {
    var list2 = document.getElementById('submissions-list');
    if (list2) list2.innerHTML = '<div style="font-family:var(--font-mono);font-size:12px;color:var(--hard);text-align:center;padding:20px">Failed to load submissions.</div>';
  }
}

function toggleSub(id) {
  var body = document.getElementById('sub-body-' + id);
  if (!body) return;
  var isOpen = body.classList.contains('open');
  document.querySelectorAll('.sub-body.open').forEach(function(b) { b.classList.remove('open'); });
  if (!isOpen) { body.classList.add('open'); loadReviews(id); }
}

async function submitSolution(qid) {
  var submitBtn = document.getElementById('submit-btn');
  if (!submitBtn || submitBtn.disabled) { showToast('Run tests first and pass them all', 'error'); return; }

  var langEl = document.getElementById('sub-lang');
  var notesEl = document.getElementById('sub-notes');
  var code = getEditorCode().trim();
  var language = langEl ? langEl.value : 'go';
  var notes = notesEl ? notesEl.value.trim() : '';
  if (!code) { showToast('Code is required', 'error'); return; }

  submitBtn.disabled = true;
  submitBtn.style.opacity = '.4';
  submitBtn.style.cursor = 'not-allowed';

  try {
    var res = await apiFetch('/api/submissions', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({question_id: qid, code: code, language: language, notes: notes})
    });
    if (!res.ok) { showToast(await res.text(), 'error'); return; }
    showToast('Solution submitted!', 'success');
    
    completedIds.add(qid);
    saveCompleted();
    updateProgress();
    renderQuestionList();
    var btn = document.getElementById('complete-btn');
    if (btn) { btn.innerHTML = '&#x2713; Completed'; btn.classList.add('done'); }
    // Clear the editor after successful submit
    if (_cmEditor) { _cmEditor.setValue(''); } 
    if (notesEl) notesEl.value = '';
    loadSubmissions(qid);
  } finally {
    // Keep locked after submit — user must re-run tests for a new submission
    submitBtn.disabled = true;
    submitBtn.style.opacity = '.4';
    submitBtn.style.cursor = 'not-allowed';
  }
}