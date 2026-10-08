(function() {
  var refreshTimer = null;
  function startAutoRefresh() {
    if (refreshTimer) clearInterval(refreshTimer);
    refreshTimer = setInterval(function() {
      fetch(window.location.href, { headers: { 'X-Requested-With': 'runbooks-refresh' } })
        .then(function(r) { return r.ok ? r.text() : null; })
        .then(function(html) {
          if (!html) return;
          var parser = new DOMParser();
          var doc = parser.parseFromString(html, 'text/html');
          var main = document.querySelector('.main');
          var newMain = doc.querySelector('.main');
          if (main && newMain) {
            main.innerHTML = newMain.innerHTML;
          }
        })
        .catch(function() {});
    }, 5000);
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', startAutoRefresh);
  } else {
    startAutoRefresh();
  }
})();
