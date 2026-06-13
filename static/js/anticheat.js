// ── Anti-cheat module ─────────────────────────────────────────────────────────

var AntiCheat = (function () {

  var _contestType = null;
  var _contestID   = null;
  var _active      = false;
  var _violations  = 0;

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
    if (!_active) return;
    handleViolation('tab_switch');
  }

  function handleViolation(type) {
    if (!_active) return;

    _violations++;

    if (_violations === 1) {
      showToast(
        '⚠️ Warning: ' + (type === 'copy_paste' ? 'Copying/pasting' : 'Switching tabs') +
        ' is not allowed. One more will disqualify you.',
        'error'
      );
      reportViolation(type, function(data) {
        if (data && data.disqualified) {
          disqualifyLocally();
        }
      });
    } else {
      _active = false;
      reportViolation(type, function() {
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
    .then(function(res) { return res.json(); })
    .then(function(data) { if (callback) callback(data); })
    .catch(function() { if (callback) callback(null); });
  }

  function reportLogout() {
    var endpoint = '/api/' + _contestType + 's/' + _contestID + '/violation';
    var token = localStorage.getItem('token') || '';
    var payload = JSON.stringify({ type: 'logout' });
    navigator.sendBeacon(endpoint + '?token=' + encodeURIComponent(token), new Blob([payload], { type: 'application/json' }));
  }

  function disqualifyLocally() {
    _active = false;
    detachListeners();

    if (window._cmEditor) {
      window._cmEditor.setOption('readOnly', true);
    }

    if (window.ContestTimer && ContestTimer.clear) {
      ContestTimer.clear();
    }

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
            '_arenaExit();' +
          '">OK</button>' +
        '</div>' +
      '</div>';
    document.body.appendChild(el);
  }

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

  function start(contestType, contestID) {
    if (_active) stop();
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