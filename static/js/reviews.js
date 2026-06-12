function setStar(subId, val) {
  starValues[subId] = val;
  var container = document.getElementById('stars-' + subId);
  if (!container) return;
  container.querySelectorAll('.star-btn').forEach(function(btn, i) {
    btn.innerHTML = (i + 1) <= val ? '&#x2605;' : '&#x2606;';
    btn.classList.toggle('lit', (i + 1) <= val);
  });
}

async function submitReview(subId) {
  var rating = starValues[subId] || 0;
  var commentEl = document.getElementById('comment-' + subId);
  var comment = commentEl ? commentEl.value.trim() : '';
  if (!rating) { showToast('Please select a star rating', 'error'); return; }
  if (!comment) { showToast('Please add a comment to your review', 'error'); return; }
  var res = await apiFetch('/api/reviews', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({submission_id: subId, rating: rating, comment: comment})
  });
  if (!res.ok) { showToast(await res.text(), 'error'); return; }
  showToast('Review submitted!', 'success');
  delete starValues[subId];
  if (commentEl) commentEl.value = '';
  var starsEl = document.getElementById('stars-' + subId);
  if (starsEl) starsEl.querySelectorAll('.star-btn').forEach(function(b) { b.innerHTML = '&#x2606;'; b.classList.remove('lit'); });
  // Hide the review form so user can't submit again
  var formEl = document.getElementById('review-form-' + subId);
  if (formEl) {
    formEl.innerHTML = '<div style="font-family:var(--font-mono);font-size:11px;color:var(--muted);padding:4px 0">You have already reviewed this solution.</div>';
  }
  loadReviews(subId);
}

async function loadReviews(subId) {
  try {
    var res = await apiFetch('/api/reviews?submission_id=' + subId);
    if (!res.ok) throw new Error('failed');
    var reviews = await res.json();
    var el = document.getElementById('reviews-' + subId);
    if (!el) return;

    // Check if current user already has a review — hide the form if so
    var myReview = currentUser && reviews
      ? reviews.find(function(r) { return r.user_id === currentUser.id; })
      : null;
    var formEl = document.getElementById('review-form-' + subId);
    if (formEl && myReview) {
      formEl.innerHTML = '<div style="font-family:var(--font-mono);font-size:11px;color:var(--muted);padding:4px 0">You have already reviewed this solution.</div>';
    }

    if (!reviews || !reviews.length) {
      el.innerHTML = '<div style="font-family:var(--font-mono);font-size:11px;color:var(--muted)">No reviews yet</div>';
      return;
    }
    el.innerHTML = reviews.map(function(r) {
      return '<div class="review-item">' +
        '<div class="review-header">' +
          '<span class="review-author">' + escHtml(r.reviewer_name) + '</span>' +
          '<span class="stars">' + starsHtml(r.rating) + '</span>' +
        '</div>' +
        (r.comment ? '<div class="review-comment">' + escHtml(r.comment) + '</div>' : '') +
      '</div>';
    }).join('');
  } catch(e) {}
}