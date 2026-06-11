// CodeMirror editor instance (global so runner.js and submissions.js can read it)
var _cmEditor = null;

// ─── Per-language keyword lists ───────────────────────────────────────────────
var LANG_KEYWORDS = {
  go: [
    // keywords
    'break','case','chan','const','continue','default','defer','else','fallthrough',
    'for','func','go','goto','if','import','interface','map','package','range',
    'return','select','struct','switch','type','var',
    // builtins
    'append','cap','close','complex','copy','delete','imag','len','make','new',
    'panic','print','println','real','recover',
    // common types
    'bool','byte','complex64','complex128','error','float32','float64',
    'int','int8','int16','int32','int64','rune','string',
    'uint','uint8','uint16','uint32','uint64','uintptr',
    // common stdlib
    'fmt.Println','fmt.Printf','fmt.Sprintf','fmt.Errorf','fmt.Scanf','fmt.Scan',
    'fmt.Print','fmt.Fprintf','fmt.Sscanf',
    'os.Exit','os.Args','os.Stdin','os.Stdout','os.Stderr',
    'strings.Contains','strings.HasPrefix','strings.HasSuffix','strings.TrimSpace',
    'strings.Split','strings.Join','strings.Replace','strings.ToLower','strings.ToUpper',
    'strconv.Atoi','strconv.Itoa','strconv.ParseInt','strconv.ParseFloat',
    'math.Abs','math.Max','math.Min','math.Sqrt','math.Pow','math.Inf',
    'sort.Ints','sort.Strings','sort.Slice',
  ],
  python: [
    'False','None','True','and','as','assert','async','await','break','class',
    'continue','def','del','elif','else','except','finally','for','from',
    'global','if','import','in','is','lambda','nonlocal','not','or','pass',
    'raise','return','try','while','with','yield',
    // builtins
    'abs','all','any','bin','bool','breakpoint','bytearray','bytes','callable',
    'chr','classmethod','compile','complex','delattr','dict','dir','divmod',
    'enumerate','eval','exec','filter','float','format','frozenset','getattr',
    'globals','hasattr','hash','help','hex','id','input','int','isinstance',
    'issubclass','iter','len','list','locals','map','max','memoryview','min',
    'next','object','oct','open','ord','pow','print','property','range',
    'repr','reversed','round','set','setattr','slice','sorted','staticmethod',
    'str','sum','super','tuple','type','vars','zip',
    // common imports
    'import os','import sys','import math','import json','import re',
    'from collections import','from itertools import',
  ],
  javascript: [
    'break','case','catch','class','const','continue','debugger','default',
    'delete','do','else','export','extends','finally','for','function','if',
    'import','in','instanceof','let','new','return','static','super','switch',
    'this','throw','try','typeof','var','void','while','with','yield',
    // builtins
    'Array','Boolean','Date','Error','Function','JSON','Math','Number','Object',
    'Promise','RegExp','Set','Map','String','Symbol',
    'console.log','console.error','console.warn',
    'JSON.parse','JSON.stringify',
    'Math.abs','Math.max','Math.min','Math.floor','Math.ceil','Math.round','Math.sqrt',
    'parseInt','parseFloat','isNaN','isFinite',
    'setTimeout','setInterval','clearTimeout','clearInterval',
    'Promise.resolve','Promise.reject','Promise.all',
  ],
  bash: [
    'if','then','else','elif','fi','for','while','do','done','case','esac',
    'function','return','exit','break','continue','local','export','readonly',
    'echo','printf','read','source','shift','set','unset',
    'grep','sed','awk','cut','sort','uniq','wc','cat','head','tail',
    'ls','cd','pwd','mkdir','rm','mv','cp','chmod','chown','find',
    '$?','$0','$1','$2','$#','$@','$$',
  ],
};

// Merge language keywords with words already in the document
function buildHinter(lang) {
  var keywords = LANG_KEYWORDS[lang] || [];
  return function(cm) {
    var anyword = CodeMirror.hint.anyword(cm) || {list: [], from: cm.getCursor(), to: cm.getCursor()};
    var cur = cm.getCursor();
    var token = cm.getTokenAt(cur);
    var word = token.string.replace(/^\W+/, '');
    if (!word) return null;

    var wordLower = word.toLowerCase();
    var seen = new Set(anyword.list);

    // Prepend matching keywords (exact prefix match, case-insensitive)
    var kwMatches = keywords.filter(function(k) {
      return k.toLowerCase().indexOf(wordLower) === 0 && !seen.has(k);
    });

    var combined = kwMatches.concat(anyword.list);
    if (combined.length === 0) return null;

    return {
      list: combined,
      from: CodeMirror.Pos(cur.line, token.start + (token.string.length - word.length)),
      to:   CodeMirror.Pos(cur.line, cur.ch),
    };
  };
}

function initCodeEditor() {
  _cmEditor = null;

  var existing = document.getElementById('sub-code-editor');
  if (existing) existing.innerHTML = '';

  setTimeout(function() {
    var target = document.getElementById('sub-code-editor');
    if (!target || !window.CodeMirror) return;
    if (target.closest('#detail-pane') === null) return;

    var langEl = document.getElementById('sub-lang');
    var initialLang = langEl ? langEl.value : 'go';

    _cmEditor = CodeMirror(target, {
      value: '',
      mode: langToMode(initialLang),
      theme: 'dracula',
      lineNumbers: true,
      indentUnit: 4,
      tabSize: 4,
      indentWithTabs: true,
      matchBrackets: true,
      autoCloseBrackets: true,
      lineWrapping: false,
      autofocus: true,
      hintOptions: {
        completeSingle: false,   // don't auto-insert when only 1 match
        alignWithWord: true,
        hint: buildHinter(initialLang),
      },
      extraKeys: {
        Tab: function(cm) {
          if (cm.somethingSelected()) {
            cm.indentSelection('add');
          } else {
            // If hint list is open, pick the selection; otherwise indent
            if (cm.state.completionActive) {
              cm.state.completionActive.widget && CodeMirror.commands.pickCompletion(cm);
            } else {
              cm.replaceSelection('    ', 'end');
            }
          }
        },
        'Shift-Tab': function(cm) {
          cm.indentSelection('subtract');
        },
        'Ctrl-Space': function(cm) {
          CodeMirror.commands.autocomplete(cm, null, {completeSingle: false});
        },
      }
    });

    _cmEditor.setSize('100%', '340px');
    _cmEditor.refresh();

    // Auto-trigger completions after each printable character (debounced)
    var hintTimer = null;
    _cmEditor.on('change', function(cm, change) {
      if (change.origin === '+delete' || change.origin === 'paste') return;
      var typed = change.text && change.text[0];
      // Only trigger on word characters; skip spaces, braces, etc.
      if (!typed || !/\w/.test(typed)) return;

      clearTimeout(hintTimer);
      hintTimer = setTimeout(function() {
        if (!cm.state.completionActive) {
          cm.showHint({completeSingle: false, hint: buildHinter(cm._currentLang || 'go')});
        }
      }, 200);
    });

    // Sync language dropdown
    if (langEl) {
      _cmEditor._currentLang = langEl.value;
      _cmEditor.setOption('mode', langToMode(langEl.value));
      _cmEditor.setOption('hintOptions', {
        completeSingle: false,
        hint: buildHinter(langEl.value),
      });

      langEl.addEventListener('change', function() {
        if (_cmEditor) {
          _cmEditor._currentLang = langEl.value;
          _cmEditor.setOption('mode', langToMode(langEl.value));
          _cmEditor.setOption('hintOptions', {
            completeSingle: false,
            hint: buildHinter(langEl.value),
          });
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

function getEditorCode() {
  if (_cmEditor) return _cmEditor.getValue();
  var el = document.getElementById('sub-code');
  return el ? el.value : '';
}