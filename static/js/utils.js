function escHtml(s) {
  return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function starsHtml(avg) {
  var full = Math.round(avg);
  return [1,2,3,4,5].map(function(i) {
    return '<span style="color:' + (i <= full ? 'var(--warn)' : 'var(--muted)') + '">' + (i <= full ? '\u2605' : '\u2606') + '</span>';
  }).join('');
}

function timeAgo(dateStr) {
  var diff = (Date.now() - new Date(dateStr).getTime()) / 1000;
  if (diff < 60) return 'just now';
  if (diff < 3600) return Math.floor(diff / 60) + 'm ago';
  if (diff < 86400) return Math.floor(diff / 3600) + 'h ago';
  return Math.floor(diff / 86400) + 'd ago';
}

function showToast(msg, type) {
  var el = document.createElement('div');
  el.className = 'toast toast-' + (type || 'success');
  el.textContent = msg;
  document.body.appendChild(el);
  setTimeout(function() { el.remove(); }, 4000);
}

function showConfirmModal(title, bodyHtml, confirmLabel, onConfirm) {
  var existing = document.getElementById('confirm-modal');
  if (existing) existing.remove();

  var el = document.createElement('div');
  el.id = 'confirm-modal';
  el.className = 'modal-overlay';
  el.innerHTML =
    '<div class="modal-box">' +
      '<h3>' + escHtml(title) + '</h3>' +
      '<p class="modal-confirm-body">' + bodyHtml + '</p>' +
      '<div class="modal-actions">' +
        '<button class="btn btn-primary btn-sm" id="confirm-modal-ok">' + escHtml(confirmLabel) + '</button>' +
        '<button class="btn btn-ghost btn-sm" onclick="document.getElementById(\'confirm-modal\').remove()">Cancel</button>' +
      '</div>' +
    '</div>';
  document.body.appendChild(el);

  document.getElementById('confirm-modal-ok').addEventListener('click', function() {
    el.remove();
    onConfirm();
  });
}