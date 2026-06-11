// CodeMirror editor instance (global so runner.js and submissions.js can read it)
var _cmEditor = null;

function initCodeEditor() {
  // Null out the reference first so getEditorCode() falls back cleanly
  // while we wait for the setTimeout
  _cmEditor = null;

  // Wipe the container directly — avoids calling toTextArea() on a
  // potentially stale instance which was causing the TypeError
  var existing = document.getElementById('sub-code-editor');
  if (existing) existing.innerHTML = '';

  // Defer mount to next paint so the freshly injected DOM has dimensions
  setTimeout(function() {
    var target = document.getElementById('sub-code-editor');
    if (!target || !window.CodeMirror) return;

    // Guard: if another question was clicked before this timeout fired,
    // the target div will have been wiped again — bail out safely
    if (target.closest('#detail-pane') === null) return;

    _cmEditor = CodeMirror(target, {
      value: '',
      mode: 'text/x-go',
      theme: 'dracula',
      lineNumbers: true,
      indentUnit: 4,
      tabSize: 4,
      indentWithTabs: true,
      matchBrackets: true,
      autoCloseBrackets: true,
      lineWrapping: false,
      autofocus: true,
      extraKeys: {
        Tab: function(cm) {
          if (cm.somethingSelected()) {
            cm.indentSelection('add');
          } else {
            cm.replaceSelection('    ', 'end');
          }
        },
        'Shift-Tab': function(cm) {
          cm.indentSelection('subtract');
        }
      }
    });

    _cmEditor.setSize('100%', '340px');

    // Force a refresh so CodeMirror redraws correctly in the new container
    _cmEditor.refresh();

    // Sync language dropdown to editor highlight mode
    var langEl = document.getElementById('sub-lang');
    if (langEl) {
      _cmEditor.setOption('mode', langToMode(langEl.value));

      langEl.addEventListener('change', function() {
        if (_cmEditor) {
          _cmEditor.setOption('mode', langToMode(langEl.value));
        }
      });
    }
  }, 0);
}

function langToMode(lang) {
  var modes = {
    go:         'text/x-go',
    python:     'text/x-python',
    javascript: 'text/javascript',
    typescript: 'text/typescript',
    java:       'text/x-java',
    c:          'text/x-csrc',
    cpp:        'text/x-c++src',
    rust:       'text/x-rustsrc',
    bash:       'text/x-sh'
  };
  return modes[lang] || 'text/plain';
}

// Helper used by runner.js and submissions.js to safely get the current code
function getEditorCode() {
  if (_cmEditor) return _cmEditor.getValue();
  // Fallback to plain textarea if CodeMirror failed to load
  var el = document.getElementById('sub-code');
  return el ? el.value : '';
}