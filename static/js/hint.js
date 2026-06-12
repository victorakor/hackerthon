// ─── AI Hint Feature ──────────────────────────────────────────────────────────
// Fetches hint status on question load, streams hints from /api/hint,
// and enforces the 2-per-question-per-session limit in the UI.

var _hintAbortController = null;

// Called by selectQuestion() in questions.js as soon as the pane renders.
async function initHintStatus(questionId) {
  var badge = document.getElementById('ai-hint-badge');
  var btn   = document.getElementById('ai-hint-btn');
  if (!badge || !btn) return;

  try {
    var res = await apiFetch('/api/hint/status?question_id=' + questionId);
    if (!res.ok) {
      badge.textContent = '?';
      return;
    }
    var data = await res.json();
    updateHintBadge(data.remaining, data.limit);
    if (data.remaining <= 0) disableHintBtn(btn);
  } catch(e) {
    badge.textContent = '?';
  }
}

function updateHintBadge(remaining, limit) {
  var badge = document.getElementById('ai-hint-badge');
  if (!badge) return;
  badge.textContent = remaining + '/' + limit;
  badge.className = 'ai-hint-badge' + (remaining <= 0 ? ' exhausted' : remaining === 1 ? ' warn' : '');
}

function disableHintBtn(btn) {
  btn = btn || document.getElementById('ai-hint-btn');
  if (!btn) return;
  btn.disabled = true;
  btn.title = 'No AI hints remaining for this question. Log out and back in to reset.';
  btn.style.opacity = '0.45';
  btn.style.cursor = 'not-allowed';
}

// Called by the AI Hint button onclick in questions.js
async function requestAiHint(questionId) {
  var q = currentQuestion;
  if (!q || q.id !== questionId) return;

  var btn    = document.getElementById('ai-hint-btn');
  var panel  = document.getElementById('ai-hint-panel');
  var output = document.getElementById('ai-hint-text');
  if (!btn || !panel || !output) return;

  // Abort any previous in-flight hint stream
  if (_hintAbortController) {
    _hintAbortController.abort();
  }
  _hintAbortController = new AbortController();

  // Show panel and loading state
  panel.style.display = 'block';
  output.innerHTML = '<span class="ai-hint-loading">&#x1F916; Thinking\u2026</span>';
  btn.disabled = true;
  btn.style.opacity = '0.6';

  var langEl = document.getElementById('sub-lang');
  var code   = getEditorCode();
  var lang   = langEl ? langEl.value : 'go';

  try {
    var res = await apiFetch('/api/hint', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        question_id:          q.id,
        question_title:       q.title,
        question_description: q.description,
        user_code:            code,
        language:             lang
      }),
      signal: _hintAbortController.signal
    });

    if (res.status === 429) {
      // Hint limit reached
      var errData = await res.json();
      output.innerHTML = '<span class="ai-hint-limit">\u26A0\uFE0F ' + escHtml(errData.message) + '</span>';
      updateHintBadge(0, 2);
      disableHintBtn(btn);
      return;
    }

    if (!res.ok) {
      var errText = await res.text();
      output.innerHTML = '<span class="ai-hint-error">\u2717 ' + escHtml(errText) + '</span>';
      btn.disabled = false;
      btn.style.opacity = '';
      return;
    }

    // ── Stream the SSE response ──────────────────────────────────────────────
    output.innerHTML = '';
    var reader  = res.body.getReader();
    var decoder = new TextDecoder();
    var buffer  = '';

    while (true) {
      var _a = await reader.read();
      var done  = _a.done;
      var value = _a.value;
      if (done) break;

      buffer += decoder.decode(value, {stream: true});
      var lines = buffer.split('\n');
      buffer = lines.pop(); // keep incomplete last line

      for (var i = 0; i < lines.length; i++) {
        var line = lines[i].trim();

        if (line.startsWith('event: meta')) {
          // next line is the data; peek ahead
          continue;
        }
        if (line.startsWith('event: done')) {
          break;
        }

        if (line.startsWith('data: ')) {
          var raw = line.slice(6);

          // meta event data: update badge
          if (raw.startsWith('{') && raw.includes('"remaining"')) {
            try {
              var meta = JSON.parse(raw);
              if (typeof meta.remaining !== 'undefined') {
                updateHintBadge(meta.remaining, meta.limit || 2);
                if (meta.remaining <= 0) disableHintBtn();
              }
            } catch(e) {}
            continue;
          }

          // token event data: append text
          if (raw !== '{}') {
            try {
              var token = JSON.parse(raw);
              // Append as text node to prevent XSS while preserving whitespace
              output.appendChild(document.createTextNode(token));
            } catch(e) {}
          }
        }
      }
    }

    // Re-enable button only if hints remain
    var badge = document.getElementById('ai-hint-badge');
    var remaining = badge ? parseInt(badge.textContent) : 0;
    if (remaining > 0) {
      btn.disabled = false;
      btn.style.opacity = '';
    }

  } catch(e) {
    if (e.name === 'AbortError') return;
    output.innerHTML = '<span class="ai-hint-error">\u2717 Connection error \u2014 could not reach AI service.</span>';
    btn.disabled = false;
    btn.style.opacity = '';
  }
}