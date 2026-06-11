// CodeMirror editor instance (global so runner.js and submissions.js can read it)
var _cmEditor = null;

function initCodeEditor() {
  // Destroy previous instance if navigating between questions
  if (_cmEditor) {
    _cmEditor.toTextArea();
    _cmEditor = null;
  }

  // Defer mount to next paint so the freshly injected DOM has dimensions
  setTimeout(function() {
    var target = document.getElementById('sub-code-editor');
    if (!target || !window.CodeMirror) return;

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

    // Sync language dropdown to editor highlight mode
    var langEl = document.getElementById('sub-lang');
    if (langEl) {
      // Set initial mode from whatever language is currently selected
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
