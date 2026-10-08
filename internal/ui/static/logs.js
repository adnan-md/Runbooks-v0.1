(function() {
  var viewer = document.getElementById('log-viewer');
  if (!viewer) return;
  var jobId = viewer.dataset.jobId;
  if (!jobId) return;
  viewer.textContent = '';
  var seen = 0;
  var es = new EventSource('/api/v1/jobs/' + jobId + '/logs/sse');
  es.onmessage = function(ev) {
    var pre = document.createElement('div');
    pre.textContent = ev.data;
    viewer.appendChild(pre);
    viewer.scrollTop = viewer.scrollHeight;
    seen++;
  };
  es.onerror = function() {
    if (es.readyState === EventSource.CLOSED) {
      var end = document.createElement('div');
      end.textContent = '--- stream closed ---';
      end.style.color = '#94a3b8';
      viewer.appendChild(end);
    }
  };
})();
