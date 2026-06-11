async function runTests(qid) {
  var codeEl = document.getElementById('sub-code');
  var langEl = document.getElementById('sub-lang');
  var code = codeEl ? codeEl.value.trim() : '';
  var language = langEl ? langEl.value : 'go';
  if (!code) { showToast('Paste your code first', 'error'); return; }

  var runBtn = document.getElementById('run-btn');
  var container = document.getElementById('test-results-container');
  if (runBtn) { runBtn.disabled = true; runBtn.innerHTML = '<span class="run-spinner"></span>Running\u2026'; }
  if (container) container.innerHTML = '';

  try {
    var res = await apiFetch('/api/run', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({question_id: qid, code: code, language: language})
    });
    if (!res.ok) {
      var errText = await res.text();
      if (container) container.innerHTML = '<div style="font-family:var(--font-mono);font-size:12px;color:var(--hard);margin-top:12px">' + escHtml(errText) + '</div>';
      return;
    }
    var result = await res.json();
    if (container) renderTestResults(result, container);
  } catch(e) {
    if (container) container.innerHTML = '<div style="font-family:var(--font-mono);font-size:12px;color:var(--hard);margin-top:12px">Connection error \u2014 could not reach the runner</div>';
  } finally {
    if (runBtn) { runBtn.disabled = false; runBtn.innerHTML = '\u25BA Run Tests'; }
  }
}

function renderTestResults(result, container) {
  var submitBtn = document.getElementById('submit-btn');

  function lockSubmit() {
    if (submitBtn) { submitBtn.disabled = true; submitBtn.style.opacity = '.4'; submitBtn.style.cursor = 'not-allowed'; }
  }
  function unlockSubmit() {
    if (submitBtn) { submitBtn.disabled = false; submitBtn.style.opacity = ''; submitBtn.style.cursor = ''; }
  }

  if (!result.results || result.results.length === 0) {
    container.innerHTML = '<div class="no-tests-note" style="margin-top:12px">No output returned.</div>';
    lockSubmit();
    return;
  }

  if (result.total === 0) {
    var r = result.results[0];
    var isErr = r.error && r.error !== 'no test cases defined for this question \u2014 output shown above';
    container.innerHTML =
      '<div class="test-results">' +
        '<div class="test-results-header">' +
          '<span class="test-results-summary ' + (isErr ? 'all-fail' : 'all-pass') + '">' +
            (isErr ? '\u2717 Runtime Error' : '\u2713 Code ran successfully') +
          '</span>' +
        '</div>' +
        '<div class="test-case ' + (isErr ? 'fail' : 'pass') + '">' +
          '<div class="test-case-body open" style="grid-template-columns:1fr">' +
            '<div>' +
              '<div class="test-field-label">' + (isErr ? 'Error' : 'Output') + '</div>' +
              '<div class="test-field-val' + (isErr ? ' error' : '') + '">' + escHtml(isErr ? r.error : r.got) + '</div>' +
            '</div>' +
          '</div>' +
        '</div>' +
        '<div class="no-tests-note">No test cases defined \u2014 add them in the admin panel to enable pass/fail checking.</div>' +
      '</div>';
    isErr ? lockSubmit() : unlockSubmit();
    return;
  }

  var isGoTestRun = result.results.length > 0 &&
    !result.results[0].input &&
    !result.results[0].expected &&
    result.results[0].got;

  var summaryClass, summaryText;
  if (result.all_passed) {
    summaryClass = 'all-pass';
    summaryText = '\u2713 All ' + result.total + ' test(s) passed';
  } else if (result.passed === 0) {
    summaryClass = 'all-fail';
    summaryText = '\u2717 0 / ' + result.total + ' passed';
  } else {
    summaryClass = 'partial';
    summaryText = '\u26A0 ' + result.passed + ' / ' + result.total + ' passed';
  }

  var casesHtml = result.results.map(function(r) {
    var cls = r.passed ? 'pass' : 'fail';
    var badgeText = r.passed ? '\u2713 Pass' : '\u2717 Fail';
    var label = isGoTestRun ? escHtml(r.got || ('Test ' + r.index)) : ('Test ' + r.index);
    var bodyContent;

    if (isGoTestRun) {
      if (r.passed) {
        bodyContent = '<div style="grid-column:1/-1"><div class="test-field-label">Function</div>' +
          '<div class="test-field-val">' + escHtml(r.got) + '</div></div>';
      } else {
        bodyContent =
          '<div><div class="test-field-label">Function</div>' +
          '<div class="test-field-val">' + escHtml(r.got) + '</div></div>' +
          '<div><div class="test-field-label">Failure Detail</div>' +
          '<div class="test-field-val error">' + escHtml(r.error || '(no detail)') + '</div></div>';
      }
    } else if (r.error) {
      bodyContent =
        '<div><div class="test-field-label">Error</div><div class="test-field-val error">' + escHtml(r.error) + '</div></div>' +
        '<div><div class="test-field-label">Input</div><div class="test-field-val">' + escHtml(r.input || '(none)') + '</div></div>';
    } else {
      bodyContent =
        '<div><div class="test-field-label">Input</div><div class="test-field-val">' + escHtml(r.input || '(none)') + '</div></div>' +
        '<div><div class="test-field-label">Expected</div><div class="test-field-val">' + escHtml(r.expected) + '</div></div>' +
        (!r.passed ? '<div style="grid-column:1/-1"><div class="test-field-label">Got</div><div class="test-field-val error">' + escHtml(r.got || '(no output)') + '</div></div>' : '');
    }

    return '<div class="test-case ' + cls + '">' +
      '<div class="test-case-header" onclick="toggleTestCase(this)">' +
        '<span class="test-case-label">' + label + '</span>' +
        '<span class="test-case-badge ' + cls + '">' + badgeText + '</span>' +
      '</div>' +
      '<div class="test-case-body' + (!r.passed ? ' open' : '') + '">' + bodyContent + '</div>' +
    '</div>';
  }).join('');

  container.innerHTML =
    '<div class="test-results">' +
      '<div class="test-results-header">' +
        '<span class="test-results-summary ' + summaryClass + '">' + summaryText + '</span>' +
      '</div>' +
      casesHtml +
    '</div>';

  result.all_passed ? unlockSubmit() : lockSubmit();
}

function toggleTestCase(header) {
  var body = header.nextElementSibling;
  if (body) body.classList.toggle('open');
}