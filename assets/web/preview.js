// ABOUTME: Adds explicit clipboard activation and accessible outline controls.
// ABOUTME: No source content runs as code and static reading requires no script.
const header = document.getElementById('hp-header');
const heading = header.firstElementChild;
const sourcePath = heading.getAttribute('data-hp-source');
const controller = new AbortController();
const events = { signal: controller.signal };
let feedbackTimer;
let pending = false;
const copy = document.createElement('button');
copy.type = 'button';
copy.className = 'hp-copy';
copy.setAttribute('aria-label', 'Copy source path');
while (heading.firstChild) copy.append(heading.firstChild);
heading.append(copy);
const status = document.createElement('span');
status.className = 'hp-status';
status.setAttribute('role', 'status');
status.setAttribute('aria-live', 'polite');
header.append(status);
const manual = document.createElement('textarea');
manual.className = 'hp-manual';
manual.readOnly = true;
manual.value = sourcePath;
manual.setAttribute('aria-label', 'Source path for manual copying');
manual.hidden = true;
header.append(manual);

function copyFailed() {
  pending = false;
  copy.disabled = false;
  status.textContent = 'Clipboard unavailable. Select and copy the path below.';
  manual.hidden = false;
}
copy.addEventListener('click', () => {
  if (pending) return;
  clearTimeout(feedbackTimer);
  status.textContent = '';
  manual.hidden = true;
  pending = true;
  copy.disabled = true;
  try {
    const writing = navigator.clipboard.writeText(sourcePath);
    writing.then(() => {
      if (controller.signal.aborted) return;
      pending = false;
      copy.disabled = false;
      status.textContent = 'Source path copied.';
      feedbackTimer = setTimeout(() => { status.textContent = ''; }, 2000);
    }, () => { if (!controller.signal.aborted) copyFailed(); });
  } catch { copyFailed(); }
}, events);

window.addEventListener('pagehide', () => {
  clearTimeout(feedbackTimer);
  controller.abort();
}, { once: true });
