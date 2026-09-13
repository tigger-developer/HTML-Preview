// ABOUTME: Exercises real packaged annotation modules with controlled clocks and transport.
// ABOUTME: Reports native browser assertions without treating visual judgement as automation.
const report = { browser: navigator.userAgent, passed: 0, failures: [] };
function assert(condition, message) {
  if (!condition) throw new Error(message);
  report.passed += 1;
}
class TestClock {
  time = 0;
  serial = 0;
  timers = new Map();
  now = () => this.time;
  setTimeout = (fn, delay) => { const id = ++this.serial; this.timers.set(id, { fn, at: this.time + delay }); return id; };
  clearTimeout = id => this.timers.delete(id);
  async advance(ms) {
    const until = this.time + ms;
    for (;;) {
      const next = [...this.timers].sort((a, b) => a[1].at - b[1].at)[0];
      if (!next || next[1].at > until) break;
      this.time = next[1].at; this.timers.delete(next[0]); next[1].fn();
      for (let i = 0; i < 12; i += 1) await Promise.resolve();
    }
    this.time = until;
    for (let i = 0; i < 12; i += 1) await Promise.resolve();
  }
}
try {
  const app = await import('./module.js');
  assert(typeof app.AnnotationComposer === 'function', 'Packaged annotation composer is available');
  assert(typeof app.canonicalMap === 'function', 'Packaged canonical text mapper is available');
  const fixtureResponse = await fetch('./text-fixtures.json');
  assert(fixtureResponse.ok, 'Shared canonical fixtures are available');
  for (const fixture of await fixtureResponse.json()) {
    const document = new DOMParser().parseFromString(fixture.html, 'text/html');
    assert(app.canonicalMap(document).text === fixture.text, 'Shared canonical fixture: ' + fixture.name);
  }
  const clock = new TestClock();
  const requests = [];
  const statuses = [];
  const composer = new app.AnnotationComposer({
    clock, jitter: () => 0,
    send: async request => { requests.push(structuredClone(request)); return { annotation_id: request.annotation_id, sequence: request.sequence, closed: request.kind === 'close' }; },
    revisions: { revision: 'a'.repeat(64), source_revision: 'b'.repeat(64), body_revision: 'c'.repeat(64) },
    target: { type: 'document' }, onState: state => statuses.push(state),
  });
  composer.input('First');
  await clock.advance(299);
  assert(requests.length === 0, 'Debounce does not save before 300 ms');
  await clock.advance(1);
  assert(requests.length === 1 && requests[0].text === 'First', 'Debounce saves the latest draft');
  assert(statuses.at(-1).status === 'Autosaved draft', 'Acknowledged draft status is visible');
  composer.composition(true); composer.input('IME text');
  await clock.advance(2500);
  assert(requests.length === 1, 'IME composition suspends submission');
  composer.composition(false); await clock.advance(300);
  assert(requests.length === 2 && requests[1].text === 'IME text', 'Composition end saves current text');
  composer.input('Final');
  const closing = composer.close();
  await clock.advance(0); await closing;
  assert(requests.at(-2).text === 'Final' && requests.at(-1).kind === 'close', 'Close flushes latest text before sealing');
  assert(statuses.at(-1).status === 'Saved', 'Closed acknowledged comment is saved');
  composer.dispose();

  const continuousClock = new TestClock(); const continuousRequests = [];
  const continuous = new app.AnnotationComposer({ clock: continuousClock, target: { type: 'document' }, revisions: { revision: 'a'.repeat(64), source_revision: 'b'.repeat(64), body_revision: 'c'.repeat(64) },
    send: async request => { continuousRequests.push(request); return { annotation_id: request.annotation_id, sequence: request.sequence, closed: false }; },
  });
  continuous.input('0');
  for (let i = 1; i < 10; i += 1) { await continuousClock.advance(200); continuous.input(String(i)); }
  await continuousClock.advance(199);
  assert(continuousRequests.length === 0, 'Continuous typing coalesces intermediate drafts');
  await continuousClock.advance(1);
  assert(continuousRequests.length === 1 && continuousRequests[0].text === '9', 'Continuous typing saves at the two-second bound');
  continuous.dispose();

  const coalesced = []; let release;
  const slowClock = new TestClock();
  const slow = new app.AnnotationComposer({ clock: slowClock, jitter: () => 0,
    revisions: { revision: 'a'.repeat(64), source_revision: 'b'.repeat(64), body_revision: 'c'.repeat(64) }, target: { type: 'document' },
    send: request => { coalesced.push(structuredClone(request)); return new Promise(resolve => { release = () => resolve({ annotation_id: request.annotation_id, sequence: request.sequence, closed: false }); }); },
  });
  slow.input('Older'); await slowClock.advance(300);
  slow.input('Newest'); await slowClock.advance(2000);
  assert(coalesced.length === 1, 'One request stays in flight while newer text is queued');
  release(); for (let i = 0; i < 20; i += 1) await Promise.resolve();
  assert(coalesced.length === 2 && coalesced[1].text === 'Newest', 'Latest unsent text is coalesced after acknowledgement');
  release(); for (let i = 0; i < 20; i += 1) await Promise.resolve(); slow.dispose();

  const retryClock = new TestClock(); const retries = [];
  const retrying = new app.AnnotationComposer({ clock: retryClock, jitter: () => 0,
    revisions: { revision: 'a'.repeat(64), source_revision: 'b'.repeat(64), body_revision: 'c'.repeat(64) }, target: { type: 'document' },
    send: async request => { retries.push(structuredClone(request)); if (retries.length < 4) { const error = new Error('Busy'); error.status = 429; error.code = 'busy'; throw error; } return { annotation_id: request.annotation_id, sequence: request.sequence, closed: false }; },
  });
  retrying.input('Retry without duplication'); await retryClock.advance(300);
  await retryClock.advance(999); assert(retries.length === 1, 'Busy retries wait at least one second');
  await retryClock.advance(1); await retryClock.advance(2000); await retryClock.advance(4000);
  assert(retries.length === 4 && retries.every(item => JSON.stringify(item) === JSON.stringify(retries[0])), 'Retry schedule retains identical operation and payload');
  retrying.dispose();

  const node = document.createElement('div');
  node.append(document.createTextNode('  café\u00a0'));
  const em = document.createElement('em'); em.textContent = '😀 code'; node.append(em);
  const map = app.canonicalMap(node);
  assert(map.text === 'café 😀 code', 'Canonical mapping uses scalar text and whitespace collapse');
  const range = document.createRange(); range.setStart(node.firstChild, 2); range.setEnd(em.firstChild, 2);
  const selection = map.selector(range, 'd'.repeat(64), {});
  assert(selection.exact === 'café 😀' && selection.start === 0 && selection.end === 6, 'Cross-node selection counts Unicode scalars');

  assert(typeof app.AnnotationPanel === 'function', 'Packaged annotation presentation is available');
  const panelClock = new TestClock();
  const state = { protocol: 1, revision: 'a'.repeat(64), source_revision: 'b'.repeat(64), body_revision: 'c'.repeat(64), events: [], writable: true, write_token: 'test-token', storage: 'embedded', reason: '' };
  const panel = new app.AnnotationPanel({ endpoint: '/fixture-state', page_url: location.href, display_name: 'Fixture Reviewer', ...state, explicit_ids: {} }, {
    clock: panelClock, request: async (_url, options = {}) => {
      if (options.method === 'POST') {
        const event = JSON.parse(options.body);
        state.revision = String(event.sequence).repeat(64);
        state.events = [{ ...event, author: 'Fixture Reviewer', recorded_at: '2026-09-13T12:00:00Z', closed: event.kind === 'close', status: 'resolved' }];
        return { annotation_id: event.annotation_id, sequence: event.sequence, closed: event.kind === 'close', stored_at: '2026-09-13T12:00:00Z', revision: state.revision, source_revision: state.source_revision, body_revision: state.body_revision };
      }
      return structuredClone(state);
    }, reinitialize: async () => {},
  });
  await panel.ready;
  panel.toggle.click();
  assert(!panel.panel.hidden && panel.toggle.getAttribute('aria-expanded') === 'true', 'Annotations button reveals accessible panel');
  panel.add.click(); for (let i = 0; i < 15; i += 1) await Promise.resolve();
  const textarea = panel.panel.querySelector('textarea');
  assert(textarea && textarea.labels.length === 1 && document.activeElement === textarea, 'Composer has a label and receives focus');
  textarea.value = '<script>literal annotation</script>'; textarea.dispatchEvent(new Event('input', { bubbles: true }));
  await panelClock.advance(300);
  assert(panel.editor.querySelector('[role=status]').textContent.includes('Autosaved draft'), 'Composer announces acknowledged autosave');
  assert(panel.appendix.textContent.includes('<script>literal annotation</script>') && panel.appendix.textContent.includes('Fixture Reviewer'), 'Print includes the latest acknowledgement before the next poll');
  await panel.closeComposer(); await panel.refresh();
  assert(panel.panel.textContent.includes('<script>literal annotation</script>') && !panel.panel.querySelector('script'), 'Stored comments render as inert text');
  assert(!panel.panel.querySelector('textarea'), 'Closing removes the editor after acknowledgement');
  panel.dispose();

  const refreshClock = new TestClock();
  const liveState = { ...state, events: [], source_revision: '1'.repeat(64), body_revision: '2'.repeat(64) };
  const liveData = { endpoint: '/live-state', page_url: '/live-page', ...liveState, explicit_ids: { stable: 'stable' } };
  await app.reinitializePreview(() => {
    document.getElementById('hp-document').innerHTML = '<section id="stable" data-hp-level="1"><h2>Original heading</h2><p>Retained phrase.</p></section>';
  });
  const live = new app.AnnotationPanel(liveData, { clock: refreshClock, request: async (_url, options = {}) => {
    if (options.html) {
      const next = { ...liveData, ...liveState };
      return '<!doctype html><title>Changed title</title><header id="hp-header"><div id="hp-header-row"><div data-hp-source="fixture.org">fixture.org</div></div></header><main id="hp-document" data-hp-startup="showall"><section id="stable" data-hp-level="1"><h2>Changed heading</h2><p>Retained phrase. New prose.</p><details data-hp-org-drawer open><summary>Properties</summary>Value</details></section></main><template id="hp-annotation-data">' + JSON.stringify(next) + '</template>';
    }
    if (options.method === 'POST') {
      const error = new Error('Capacity reached'); error.status = 503; error.code = 'composer_capacity'; throw error;
    }
    return structuredClone(liveState);
  } });
  await live.ready; live.toggle.click(); live.add.click();
  const liveTextarea = live.textarea;
  liveTextarea.value = 'Never lose this draft'; liveTextarea.dispatchEvent(new Event('input'));
  document.querySelector('#stable > .hp-toggle').click();
  liveState.source_revision = '3'.repeat(64); liveState.body_revision = '4'.repeat(64); liveState.revision = '5'.repeat(64);
  await live.refresh();
  assert(live.textarea === liveTextarea && liveTextarea.value === 'Never lose this draft', 'Source refresh keeps the same composer and dirty text');
  assert(document.title === 'Changed title' && document.querySelector('#stable > h2').textContent === 'Changed heading', 'Refresh replaces owned title and authored content');
  assert(document.querySelector('#stable > .hp-toggle').getAttribute('aria-expanded') === 'false', 'Verified explicit heading keeps folded state');
  assert(!document.querySelector('#stable details').open, 'Refreshed drawers return to folded defaults');
  const fold = document.querySelector('#stable > .hp-toggle'); fold.click();
  assert(fold.getAttribute('aria-expanded') === 'true' && document.querySelectorAll('#stable > .hp-toggle').length === 1, 'Refresh preserves one independent clickable folding bar');
  await refreshClock.advance(300);
  assert(live.status.textContent.includes('Not saved'), 'Capacity error remains visibly unsaved without automatic retry');
  let closeFailed = false;
  try { await live.closeComposer(); } catch { closeFailed = true; }
  assert(closeFailed && live.textarea === liveTextarea && !liveTextarea.readOnly, 'Failed close retains an editable draft');
  const unload = new Event('beforeunload', { cancelable: true }); window.dispatchEvent(unload);
  assert(unload.defaultPrevented, 'Unsaved draft guards browser navigation');
  assert(!live.appendix.textContent.includes('Never lose this draft'), 'Print appendix excludes unacknowledged text');
  live.dispose();

  const invalid = new app.AnnotationPanel({ endpoint: '/invalid', page_url: location.href, ...state }, { clock: new TestClock(), request: async () => ({ ...state, body_revision: '', writable: true }) });
  await invalid.ready;
  assert(invalid.add.disabled && invalid.connection.textContent.includes('unavailable'), 'Malformed state cannot enable annotation writes');
  invalid.dispose();
} catch (error) {
  report.failures.push(error instanceof Error ? error.message : String(error));
}
document.getElementById('test-results').textContent = report.failures.length ? `FAIL: ${report.failures.join('; ')}` : `PASS: ${report.passed} assertions. This tab can be closed.`;
try {
  const response = await fetch('./results', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(report) });
  if (!response.ok) throw new Error(`Result delivery failed: ${response.status}`);
} catch (error) {
  document.getElementById('test-results').textContent += ` Result delivery error: ${error.message}`;
}
