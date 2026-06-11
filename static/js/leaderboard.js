async function loadLeaderboard() {
  try {
    var res = await apiFetch('/api/leaderboard');
    if (!res.ok) return;
    var board = await res.json();
    var ranks = ['gold', 'silver', 'bronze'];
    var medals = ['\uD83E\uDD47', '\uD83E\uDD48', '\uD83E\uDD49'];
    document.getElementById('lb-body').innerHTML = board.map(function(c, i) {
      return '<tr>' +
        '<td><span class="lb-rank ' + (ranks[i] || '') + '">' + (i < 3 ? medals[i] : i + 1) + '</span></td>' +
        '<td><span class="lb-name">' + escHtml(c.author_name) + '</span></td>' +
        '<td><span style="font-family:var(--font-mono)">' + c.submission_count + '</span></td>' +
        '<td>' + starsHtml(c.avg_rating) + ' <span style="font-family:var(--font-mono);font-size:11px;color:var(--muted)">' + c.avg_rating.toFixed(1) + '</span></td>' +
        '<td><span style="font-family:var(--font-mono)">' + c.total_reviews + '</span></td>' +
        '<td>' + (c.user_id && c.user_id !== currentUser.id ?
          '<button class="follow-btn ' + (followingIds.has(c.user_id) ? 'following' : '') + '" id="fb-' + c.user_id + '" onclick="toggleFollow(' + c.user_id + ',this)">' +
            (followingIds.has(c.user_id) ? '&#x2713; Following' : '+ Follow') +
          '</button>' : '\u2014') +
        '</td>' +
      '</tr>';
    }).join('');
  } catch(e) {}
}