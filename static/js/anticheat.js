// ── Anti-cheat module ─────────────────────────────────────────────────────────
// Attached when a user enters an active challenge or tournament arena.
// Detached when the arena is exited or the contest ends.

var AntiCheat = (function () {

  var _contestType = null;  // 'challenge' | 'tournament'
  var _contestID   = null;
  var _active      = false;
  var _violations  = 0;     // local count, server is the source of truth

  // ── Handlers ────────────────────────────────────────────────────────────────

  function onClipboard(e) {
    if (!_active) return;
    e.preventDefault();
    handleViolation('copy_paste');
  }

  function onVisibilityChange() {
    if (!_active) return;
    if (document.visibilityState === 'hidden') {
      handleViolation('tab_switch');
    }
  }

  function onWindowBlur() {
    // Secondary signal — fires when the browser window loses focus (alt-tab, etc.)
    if (!_active) return;
    handleViolation('tab_switch');
  }

  // ── Core logic ───────────────────────────────────────────────────────────────

  function handleViolation(type) {
    if (!_active) return;

    _violations++;

    if (_violations === 1) {
      // First offence — warn but keep going
      showToast(
        '⚠️ Warning: ' + (type === 'copy_paste' ? 'Copying/pasting' : 'Switching tabs') +
        ' is not allowed. One more will disqualify you.',
        'error'
      );
      reportViolation(type, function (data) {
        // If server says already disqualified (e.g. reconnected after a prior session), lock now
        if (data && data.disqualified) {
          disqualifyLocally();
        }
      });
    } else {
      // Second offence — disqualify
      _active = false; // stop further detection immediately
      reportViolation(type, function () {
        disqualifyLocally();
      });
    }
  }

  function reportViolation(type, callback) {
    var endpoint = '/api/' + _contestType + 's/' + _contestID + '/violation';
    apiFetch(endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ type: type })
    })
    .then(function (res) { return res.json(); })
    .then(function (data) { if (callback) callback(data); })
    .catch(function () { if (callback) callback(null); });
  }

  function reportLogout() {
    // Use sendBeacon so the request survives page unload
    var endpoint = '/api/' + _contestType + 's/' + _contestID + '/violation';
    var token = localStorage.getItem('token') || '';
    var payload = JSON.stringify({ type: 'logout' });
    // sendBeacon doesn't support custom headers — send token as query param
    navigator.sendBeacon(endpoint + '?token=' + encodeURIComponent(token), new Blob([payload], { type: 'application/json' }));
  }

  function disqualifyLocally() {
    _active = false;
    detachListeners();

    // Lock the editor
    if (window._cmEditor) {
      window._cmEditor.setOption('readOnly', true);
    }

    // Stop the contest timer display
    if (window.ContestTimer && ContestTimer.clear) {
      ContestTimer.clear();
    }

    // Show the disqualification modal
    showDQModal();
  }

  function showDQModal() {
    var existing = document.getElementById('dq-modal');
    if (existing) return;

    var el = document.createElement('div');
    el.id = 'dq-modal';
    el.className = 'modal-overlay';
    el.innerHTML =
      '<div class="modal-box" style="text-align:center">' +
        '<div style="font-size:48px;margin-bottom:12px">🚫</div>' +
        '<h3 style="color:var(--color-danger,#e53)">Disqualified</h3>' +
        '<p style="margin:12px 0 4px">You have been removed from this contest for violating the rules.</p>' +
        '<p style="color:var(--muted);font-size:13px">Your score has been frozen at your current progress.<br>The contest continues for other participants.</p>' +
        '<div class="modal-actions" style="margin-top:20px">' +
          '<button class="btn btn-primary" onclick="' +
            'document.getElementById(\'dq-modal\').remove();' +
            'if(typeof exitContestMode===\'function\') exitContestMode();' +
          '">OK</button>' +
        '</div>' +
      '</div>';
    document.body.appendChild(el);
  }

  // ── Listener management ──────────────────────────────────────────────────────

  function attachListeners() {
    document.addEventListener('copy',  onClipboard);
    document.addEventListener('paste', onClipboard);
    document.addEventListener('cut',   onClipboard);
    document.addEventListener('visibilitychange', onVisibilityChange);
    window.addEventListener('blur', onWindowBlur);
    window.addEventListener('beforeunload', onBeforeUnload);
  }

  function detachListeners() {
    document.removeEventListener('copy',  onClipboard);
    document.removeEventListener('paste', onClipboard);
    document.removeEventListener('cut',   onClipboard);
    document.removeEventListener('visibilitychange', onVisibilityChange);
    window.removeEventListener('blur', onWindowBlur);
    window.removeEventListener('beforeunload', onBeforeUnload);
  }

  function onBeforeUnload() {
    if (!_active) return;
    reportLogout();
  }

  // ── Public API ───────────────────────────────────────────────────────────────

  function start(contestType, contestID) {
    if (_active) stop(); // clean up any prior session
    _contestType = contestType;
    _contestID   = contestID;
    _violations  = 0;
    _active      = true;
    attachListeners();
  }

  function stop() {
    _active = false;
    detachListeners();
    _contestType = null;
    _contestID   = null;
    _violations  = 0;
  }

  return { start: start, stop: stop };

})();