// ABOUTME: Presents service annotations and refreshes owned document regions.
// ABOUTME: Keeps the active composer outside refreshed content and guards navigation.
async function annotationRequest(url, options = {}) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 5000);
  try {
    const response = await fetch(url, { ...options, signal: controller.signal, credentials: 'omit', cache: 'no-store' });
    const limit = options.html ? 50 * 1024 * 1024 : 11 * 1024 * 1024;
    const reader = response.body.getReader(); const chunks = []; let size = 0;
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      size += value.length;
      if (size > limit) { await reader.cancel(); throw new Error('The service response exceeds its limit.'); }
      chunks.push(value);
    }
    const bytes = new Uint8Array(size); let offset = 0;
    for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length; }
    const text = new TextDecoder('utf-8', { fatal: true }).decode(bytes);
    const type = response.headers.get('Content-Type') || '';
    const data = type.startsWith('application/json') ? JSON.parse(text) : null;
    if (!response.ok) {
      const error = new Error(annotationFailureMessage(data?.error, response.status));
      error.status = response.status; error.code = data?.error; throw error;
    }
    if (options.html && type.startsWith('text/html')) return text;
    if (!options.html && data) return data;
    throw new Error('The service returned an unexpected response type.');
  } finally { clearTimeout(timer); }
}

function annotationFailureMessage(code, status) {
  if (['composer_capacity', 'grant_capacity'].includes(code)) return 'Annotation capacity reached. Close another comment or restart the service, then retry.';
  if (code === 'annotation_upgrade_required') return 'This preview uses an older annotation protocol. Reload the page before annotating.';
  if (code === 'invalid_label') return 'Use 1–64 letters, digits, underscores or hyphens, beginning with a letter.';
  if (code === 'label_conflict') return 'This footnote ID is already in use. Choose another ID.';
  if (code === 'point_unmappable') return 'This insertion point cannot be matched safely to the source. Your draft is retained.';
  if (code === 'stale_source' || code === 'stale_body') return 'The source changed. Waiting for a refreshed preview.';
  if (code?.startsWith('target_')) return 'The insertion point is unavailable or no longer unique. Your draft is retained.';
  if (['source_replaced', 'source_changed', 'unsafe_source', 'unsafe_sidecar'].includes(code)) return 'The file identity changed or is unsafe to update. Copy the draft and reopen after checking the file.';
  if (['corrupt_store', 'store_changed', 'foreign_sidecar', 'unsupported_store'].includes(code)) return 'Stored annotation data needs inspection. Earlier records and this draft are retained; no repair is automatic.';
  if (['closed_comment', 'composer_conflict', 'sequence_conflict'].includes(code)) return 'This comment cannot accept another revision. Copy the draft and create a new comment.';
  if (code === 'operation_conflict') return 'The service found conflicting saved data. Copy the draft before retrying.';
  if (status === 413) return 'The comment, insertion context or annotation store exceeds its storage limit.';
  if (status === 403 || status === 404) return 'The preview no longer has access. Copy the draft and reopen the document through htmlpreview.';
  if (status === 429 || status === 503) return 'Saving is temporarily unavailable. The draft is retained for retry.';
  return 'The annotation request failed. Copy the draft before leaving and retry after checking the service.';
}

function annotationElement(tag, text, className) {
  const node = document.createElement(tag);
  if (text) node.textContent = text;
  if (className) node.className = className;
  return node;
}

function annotationButton(text) {
  const node = annotationElement('button', text); node.type = 'button'; return node;
}

function annotationRevisions(data) {
  return Object.fromEntries(['revision', 'source_revision', 'body_revision'].map(key => [key, data[key]]));
}

function validateAnnotationState(state) {
  const revisions = ['revision', 'source_revision', 'body_revision'];
  if (!state || state.protocol !== 2 || !revisions.every(key => typeof state[key] === 'string' && /^[a-f0-9]{64}$/.test(state[key])) ||
      !Array.isArray(state.comments) || state.comments.length > 10000 || !Array.isArray(state.footnote_labels) || typeof state.writable !== 'boolean' ||
      typeof state.reason !== 'string' || (state.writable && (!['embedded', 'sidecar'].includes(state.storage) || typeof state.write_token !== 'string' || !state.write_token))) {
    throw new Error('Unsupported annotation state.');
  }
  return state;
}

export class AnnotationPanel {
  constructor(data, options = {}) {
    this.data = data; this.request = options.request || annotationRequest;
    this.clock = options.clock || { now: () => performance.now(), setTimeout: (fn, ms) => setTimeout(fn, ms), clearTimeout: id => clearTimeout(id) };
    this.reinitialize = options.reinitialize || reinitializePreview;
    this.controller = new AbortController(); this.events = { signal: this.controller.signal };
    this.disposed = false; this.composer = null; this.poll = null; this.pendingRefresh = null; this.failures = 0;
    this.panel = annotationElement('aside', '', 'hp-annotations'); this.panel.id = 'hp-annotations'; this.panel.hidden = true;
    this.panel.setAttribute('aria-label', 'Annotations');
    this.toggle = annotationButton('Annotations'); this.toggle.setAttribute('aria-controls', this.panel.id); this.toggle.setAttribute('aria-expanded', 'false'); this.toggle.setAttribute('aria-pressed', 'false');
    this.connection = annotationElement('p', 'Loading annotations…', 'hp-annotation-status');
    this.connection.setAttribute('role', 'status'); this.connection.setAttribute('aria-live', 'polite');
    this.reconnect = annotationButton('Reconnect'); this.reconnect.hidden = true;
    this.list = annotationElement('div'); this.editor = annotationElement('div');
    this.panel.append(annotationElement('h2', 'Annotations'), this.connection, this.reconnect, this.editor, this.list);
    document.body.append(this.panel); this.attachToggle();
    this.listen();
    if (typeof ResizeObserver === 'function') {
      this.markerObserver = new ResizeObserver(() => this.positionCaret());
      this.markerObserver.observe(document.body);
    }
    this.ready = this.refresh().catch(error => this.showFailure(error));
  }

  attachToggle() { (document.querySelector('#hp-header .hp-toolbar') || document.getElementById('hp-header-row')).append(this.toggle); }

  listen() {
    document.addEventListener('hp-before-outline', event => {
      event.preventDefault();
      this.transition(() => { event.detail.apply(); this.toggle.disabled = false; }).catch(error => this.showComposerFailure(error));
    }, this.events);
    document.addEventListener('hp-before-plaintext', event => {
      event.preventDefault();
      this.transition(async current => {
        if (event.detail.on) {
          await this.refresh();
          if (!current()) return;
          await this.replaceSource(this.state);
        }
        if (!current()) return;
        event.detail.apply(); this.applyMode(false); this.toggle.disabled = event.detail.on;
      }).catch(error => this.showComposerFailure(error));
    }, this.events);
    this.toggle.addEventListener('click', () => this.setMode(!(this.modeRequest?.on ?? !this.panel.hidden)).catch(error => this.showComposerFailure(error)), this.events);
    this.reconnect.addEventListener('click', () => this.refresh().catch(error => this.showFailure(error)), this.events);
    document.addEventListener('click', event => {
      if (this.panel.hidden || !this.state?.writable || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return;
      if (window.getSelection()?.isCollapsed === false) return;
      const main = document.getElementById('hp-document');
      if (!(event.target instanceof Element) || !main.contains(event.target) || event.target.closest('a, button, summary, code, pre')) return;
      let range;
      if (document.caretPositionFromPoint) {
        const point = document.caretPositionFromPoint(event.clientX, event.clientY);
        if (point) { range = document.createRange(); range.setStart(point.offsetNode, point.offset); range.collapse(true); }
      } else if (document.caretRangeFromPoint) range = document.caretRangeFromPoint(event.clientX, event.clientY);
      const target = range && canonicalMap(main).point(range, this.state.body_revision, this.data.explicit_ids || {});
      if (target) this.transition(() => {
        const result = resolveAnnotationTarget(target, canonicalMap(document.getElementById('hp-document')).text, this.state.body_revision, this.headingSpans());
        if (result.status === 'resolved') this.openComposer(result.target);
        else this.connection.textContent = 'This insertion point changed. Choose a point in the refreshed document.';
      }).catch(error => this.showComposerFailure(error));
    }, this.events);
    document.addEventListener('keydown', event => this.placeWithKeyboard(event), this.events);
    document.addEventListener('scroll', () => this.positionCaret(), { ...this.events, capture: true, passive: true });
    window.addEventListener('resize', () => this.positionCaret(), this.events);
    window.addEventListener('beforeprint', () => this.placeEndnotes(false), this.events);
    window.addEventListener('afterprint', () => this.placeEndnotes(!this.panel.hidden), this.events);
    document.addEventListener('visibilitychange', () => {
      this.cancelPoll();
      if (!document.hidden) this.refresh().catch(error => this.showFailure(error));
      if (this.composer) this.composer.flush().catch(error => this.showComposerFailure(error));
    }, this.events);
    for (const event of ['focus', 'pageshow']) window.addEventListener(event, () => {
      if (!document.hidden) this.refresh().catch(error => this.showFailure(error));
      if (this.composer) this.composer.flush().catch(error => this.showComposerFailure(error));
    }, this.events);
    window.addEventListener('beforeunload', event => {
      if (this.composer?.dirty) { event.preventDefault(); event.returnValue = ''; }
    }, this.events);
    document.addEventListener('keydown', event => {
      if (event.key !== 'Escape' || !this.composer || this.composer.composing) return;
      event.preventDefault(); this.closeComposer().catch(error => this.showComposerFailure(error));
    }, this.events);
    document.addEventListener('click', event => this.guardNavigation(event), { ...this.events, capture: true });
    document.addEventListener('click', event => {
      if (event.defaultPrevented || event.button !== 0 || !this.composer || !(event.target instanceof Element) || this.editor.contains(event.target) || event.target.closest('#hp-header')) return;
      // Share the same pending close with a clicked insertion point or link.
      // Failure keeps the editor visible instead of discarding the draft.
      this.closeComposer().catch(error => this.showComposerFailure(error));
    }, { ...this.events, capture: true });
    this.editor.addEventListener('focusout', event => {
      if (!event.relatedTarget || this.editor.contains(event.relatedTarget) || !this.composer) return;
      this.closeComposer().catch(error => this.showComposerFailure(error));
    }, this.events);
  }

  guardNavigation(event) {
    const link = event.target instanceof Element && event.target.closest('a[href]');
    if (!link || !this.composer || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || link.target === '_blank' || link.hasAttribute('download')) return;
    const url = new URL(link.href);
    if (url.origin === location.origin && url.pathname === location.pathname && url.search === location.search) return;
    event.preventDefault(); event.stopImmediatePropagation();
    this.transition(() => location.assign(url.href)).catch(error => this.showComposerFailure(error));
  }

  async transition(action) {
    const version = this.transitionVersion = (this.transitionVersion || 0) + 1;
    await this.closeComposer();
    const current = () => !this.disposed && version === this.transitionVersion;
    if (current()) await action(current);
  }

  async setMode(on) {
    const request = { on }; this.modeRequest = request;
    try { await this.transition(() => this.applyMode(on)); }
    finally { if (this.modeRequest === request) this.modeRequest = undefined; }
  }

  applyMode(on) {
    this.panel.hidden = !on;
    this.toggle.setAttribute('aria-expanded', String(on));
    this.toggle.setAttribute('aria-pressed', String(on));
    document.body.classList.toggle('hp-annotating', on);
    this.prepareKeyboard(on);
    this.placeEndnotes(on);
  }

  openComposer(target) {
    if (!this.state?.writable || this.composer || target?.type !== 'point' || document.body.classList.contains('hp-plaintext')) return;
    this.clearCaret();
    this.editor.replaceChildren();
    this.markInsertionPoint(target);
    this.textarea = annotationElement('textarea'); this.textarea.id = 'hp-annotation-text'; this.textarea.rows = 6;
    const label = annotationElement('label', 'Your comment'); label.htmlFor = this.textarea.id;
    this.status = annotationElement('p', 'Not saved', 'hp-annotation-status'); this.status.setAttribute('role', 'status'); this.status.setAttribute('aria-live', 'polite');
    const retry = annotationButton('Retry'); const copy = annotationButton('Copy draft'); const restore = annotationButton('Restore last autosave');
    this.recovery = annotationElement('div', '', 'hp-annotation-actions'); this.recovery.hidden = true; this.recovery.append(retry, copy, restore);
    this.idInput = annotationElement('input'); this.idInput.id = 'hp-annotation-id'; this.idInput.type = 'text'; this.idInput.maxLength = 64;
    this.idInput.value = defaultFootnoteID(this.data.display_name || '', this.state.footnote_labels);
    const idLabel = annotationElement('label', 'Footnote ID', 'hp-annotation-id-label'); idLabel.htmlFor = this.idInput.id;
    this.idError = annotationElement('small'); this.idError.id = 'hp-annotation-id-error'; this.idInput.setAttribute('aria-describedby', this.idError.id);
    this.editor.append(label, this.textarea, this.status, idLabel, this.idInput, this.idError, this.recovery);

    this.composer = new AnnotationComposer({ clock: this.clock, revisions: annotationRevisions(this.state), target, label: this.idInput.value,
      send: (request, secret) => this.request(this.data.endpoint, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-HTMLPreview-Annotation-Token': this.state.write_token, 'X-HTMLPreview-Composer-Token': secret }, body: JSON.stringify(request) }),
      onState: state => {
        const saved = !state.error && !state.dirty && (state.status === 'Autosaved draft' || state.status === 'Saved');
        this.status.textContent = (saved ? 'Auto saved' : state.status) + (state.error ? ': ' + state.error.message : '');
        this.status.classList.toggle('hp-autosaved', saved);
        this.editor.dataset.saveState = state.error ? 'error' : state.dirty ? 'pending' : saved ? 'saved' : 'empty';
        this.recovery.hidden = !state.error;
        const fieldError = ['invalid_label', 'label_conflict'].includes(state.error?.code);
        this.idInput.setAttribute('aria-invalid', String(fieldError)); this.idError.textContent = fieldError ? state.error.message : '';

        if (this.composer?.sequence && this.displayedSequence !== this.composer.sequence) { this.displayedSequence = this.composer.sequence; this.refresh().catch(error => this.showFailure(error)); }
      },
      onStale: async current => { await this.refresh(); return this.rebaseTarget(current); },
    });
    this.idInput.addEventListener('input', () => this.composer.setLabel(this.idInput.value), this.events);
    this.textarea.addEventListener('input', () => this.composer.input(this.textarea.value), this.events);
    this.textarea.addEventListener('compositionstart', () => this.composer.composition(true), this.events);
    this.textarea.addEventListener('compositionend', () => this.composer.composition(false), this.events);
    retry.addEventListener('click', () => this.retryComposer(), this.events);
    restore.addEventListener('click', () => { this.textarea.value = this.composer.savedText; this.idInput.value = this.composer.savedLabel; this.composer.restoreSaved(); this.textarea.focus(); }, this.events);
    copy.addEventListener('click', () => {
      this.status.classList.remove('hp-autosaved');
      if (!navigator.clipboard) { this.textarea.focus(); this.textarea.select(); this.status.textContent = 'Select and copy the draft using your keyboard.'; return; }
      navigator.clipboard.writeText(this.textarea.value).then(() => { this.status.textContent = 'Draft copied.'; }, () => { this.textarea.focus(); this.textarea.select(); this.status.textContent = 'Clipboard unavailable. Select and copy the draft.'; });
    }, this.events);
    this.textarea.focus();
  }

  async retryComposer() {
    try { await this.refresh(); await this.composer.retry(); }
    catch (error) { this.showComposerFailure(error); }
  }

  closeComposer() {
    if (!this.composer || this.closing) return this.closing || Promise.resolve();
    this.closing = this.finishComposer();
    return this.closing;
  }

  async finishComposer() {
    this.textarea.readOnly = true; this.idInput.readOnly = true;
    try {
      await this.composer.close();
      const restoreFocus = this.editor.contains(document.activeElement);
      this.composer.dispose(); this.composer = null; this.editor.replaceChildren(); this.clearInsertionPoint();
      if (restoreFocus) this.toggle.focus();
      await this.refresh();
    } finally { this.closing = null; if (this.composer) { this.textarea.readOnly = false; this.idInput.readOnly = false; } }
  }

  showComposerFailure(error) {
    if (this.disposed) return;
    if (!this.composer) { this.showFailure(error); return; }
    this.status.classList.remove('hp-autosaved'); this.editor.dataset.saveState = 'error';
    this.status.textContent = 'Not saved: ' + error.message; this.recovery.hidden = false;
    this.applyMode(true); this.textarea.focus();
  }

  showFailure(error) {
    if (this.disposed) return;
    this.connection.textContent = 'Annotations unavailable: ' + error.message;
    this.reconnect.hidden = false;
    if (!this.composer && this.panel.hidden) {
      this.notice ||= annotationElement('p', '', 'hp-annotation-status'); this.notice.setAttribute('role', 'status');
      this.notice.textContent = this.connection.textContent; document.getElementById('hp-header').after(this.notice);
    }
    if (this.composer && [403, 404].includes(error.status)) this.composer.suspend('The document is unavailable. Copy your draft before leaving.');
  }

  cancelPoll() { if (this.poll !== null) this.clock.clearTimeout(this.poll); this.poll = null; }

  schedulePoll() {
    this.cancelPoll();
    if (this.disposed || document.hidden || this.lastError?.code === 'grant_capacity') return;
    const delay = this.failures ? [2000, 4000, 8000, 30000][Math.min(this.failures - 1, 3)] : 1000;
    this.poll = this.clock.setTimeout(() => { this.poll = null; this.refresh().catch(error => this.showFailure(error)); }, delay);
  }

  async refresh() {
    if (this.disposed) return;
    if (this.pendingRefresh) return this.pendingRefresh;
    this.cancelPoll();
    this.pendingRefresh = this.loadState();
    try { await this.pendingRefresh; this.failures = 0; this.lastError = null; }
    catch (error) { this.failures += 1; this.lastError = error; throw error; }
    finally { this.pendingRefresh = null; this.schedulePoll(); }
  }

  async loadState() {
    let state = validateAnnotationState(await this.request(this.data.endpoint));
    if (this.disposed) return;
    if (state.revision !== this.data.revision) state = await this.replaceSource(state);
    if (this.disposed) return;
    const changed = !this.state || this.state.revision !== state.revision;
    this.state = state; this.data.revision = state.revision;
    this.reconnect.hidden = true; this.notice?.remove();
    this.connection.textContent = state.writable ? (state.storage === 'sidecar' ? 'Read-only source; footnotes are saved in a sidecar.' : '') : 'Reading only: ' + state.reason.replaceAll('_', ' ');
    if (changed) this.renderComments(state.comments);
    if (this.composer) {
      if (!state.writable) this.composer.suspend('Saving is unavailable: ' + state.reason.replaceAll('_', ' '));
      else if (!this.composer.inFlight) {
        const replacement = this.rebaseTarget(this.composer.target);
        if (replacement) this.composer.rebase(replacement.target, replacement.revisions);
      }
      if (this.composer.paused) this.clearInsertionPoint();
      else if (!this.insertionRange) this.markInsertionPoint(this.composer.target);
    }
    this.attachToggle();
  }

  rebaseTarget(target) {
    const result = resolveAnnotationTarget(target, canonicalMap(document.getElementById('hp-document')).text, this.state.body_revision, this.headingSpans());
    if (result.status !== 'resolved') { this.composer?.suspend('The insertion point is ' + result.status + '. Your draft is retained.'); return null; }
    return { target: result.target, revisions: annotationRevisions(this.state) };
  }

  headingSpans() {
    const main = document.getElementById('hp-document'); const spans = {};
    for (const [key, id] of Object.entries(this.data.explicit_ids || {})) {
      const node = document.getElementById(id);
      if (!node || !main.contains(node)) continue;
      const range = document.createRange(); range.selectNodeContents(node);
      const prefix = document.createRange(); prefix.selectNodeContents(main); prefix.setEndBefore(node);
      const before = authoredText(prefix.cloneContents());
      const start = Array.from(before).length + (before ? 1 : 0);
      spans[key] = { start, end: start + Array.from(authoredText(range.cloneContents())).length };
    }
    return spans;
  }

  renderComments() {
    const main = document.getElementById('hp-document');
    const fresh = main.querySelector('section.footnotes');
    if (fresh) {
      if (this.endnotes && this.endnotes !== fresh) this.endnotes.remove();
      this.endnotes = fresh;
      if (this.endnotesSlot) this.endnotesSlot.remove();
      this.endnotesSlot = document.createComment('Native footnotes'); main.append(this.endnotesSlot);
      this.placeEndnotes(!this.panel.hidden);
    } else if (this.endnotesSlot && !this.endnotesSlot.isConnected) {
      this.endnotes?.remove(); this.endnotes = null; this.endnotesSlot = null;
    }
    for (const paragraph of this.endnotes?.querySelectorAll('li > p:last-of-type') || []) {
      const comment = this.state?.comments.find(item => paragraph.textContent.startsWith('Author: ' + item.author + '; Created: ' + item.created_at));
      if (!comment) continue;
      const date = new Date(comment.created_at);
      if (Number.isNaN(date.getTime())) continue;
      const backlinks = [...paragraph.querySelectorAll('a.footnote-back')];
      const timestamp = annotationElement('time', new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date));
      timestamp.dateTime = comment.created_at; timestamp.title = comment.created_at;
      paragraph.replaceChildren(document.createTextNode(comment.author + ' · '), timestamp, document.createTextNode(' '), ...backlinks);
      paragraph.classList.add('hp-annotation-author');
    }
  }

  prepareKeyboard(on) {
    this.clearCaret();
    for (const [node, value] of this.keyboardBlocks || []) {
      if (value === null) node.removeAttribute('tabindex'); else node.setAttribute('tabindex', value);
    }
    this.keyboardBlocks = new Map();
    if (!on) return;
    for (const node of document.querySelectorAll('#hp-document :is(p,li,td,th)')) {
      if (node.closest('pre,code,.footnotes,[data-hp-org-drawer]') || node.querySelector('p,li,td,th')) continue;
      this.keyboardBlocks.set(node, node.getAttribute('tabindex')); node.tabIndex = 0;
    }
  }

  clearCaret() { this.caretMarker?.remove(); this.caretMarker = null; this.keyboardCaret = null; }

  markInsertionPoint(target) {
    const map = canonicalMap(document.getElementById('hp-document'));
    const resolved = resolveAnnotationTarget(target, map.text, this.data.body_revision, this.headingSpans());
    this.insertionRange = resolved.status === 'resolved' ? map.rangeAt(resolved.target.position) : null;
    if (!this.insertionRange) { this.clearInsertionPoint(); return; }
    if (!this.insertionMarker) {
      this.insertionMarker = annotationElement('span', '', 'hp-point-marker hp-insertion-marker');
      this.insertionMarker.setAttribute('aria-hidden', 'true'); document.body.append(this.insertionMarker);
    }
    this.positionCaret();
  }

  clearInsertionPoint() { this.insertionMarker?.remove(); this.insertionMarker = null; this.insertionRange = null; }

  placeWithKeyboard(event) {
    if (this.panel.hidden || this.composer || !this.state?.writable || event.isComposing) return;
    if (event.key === 'Escape' && this.keyboardCaret) { event.preventDefault(); this.clearCaret(); return; }
    if (!['Enter', 'ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
    const block = event.target;
    if (!this.keyboardBlocks?.has(block)) return;
    if (!this.keyboardCaret && event.key !== 'Enter') return;
    event.preventDefault();
    if (this.keyboardCaret?.block !== block) {
      this.clearCaret();
      const nodes = []; const walker = document.createTreeWalker(block, NodeFilter.SHOW_TEXT);
      while (walker.nextNode()) {
        const node = walker.currentNode;
        if (node.data.trim() && !node.parentElement.closest('a,code,pre,button')) nodes.push(node);
      }
      if (!nodes.length) return;
      this.keyboardCaret = { block, nodes, index: 0, offset: 0 };
      this.caretMarker = annotationElement('span', '│', 'hp-point-marker'); this.caretMarker.setAttribute('aria-hidden', 'true'); document.body.append(this.caretMarker);
      this.connection.textContent = 'Use Left/Right, Home or End to place the footnote; Enter adds it, Escape cancels.';
    } else if (event.key === 'Enter') {
      const target = canonicalMap(document.getElementById('hp-document')).point(this.keyboardRange(), this.state.body_revision, this.data.explicit_ids || {});
      if (target) this.openComposer(target);
      else this.connection.textContent = 'This point cannot be matched safely. Move the caret to ordinary text.';
      return;
    } else {
      const caret = this.keyboardCaret;
      const length = Array.from(caret.nodes[caret.index].data).length;
      if (event.key === 'Home') { caret.index = 0; caret.offset = 0; }
      if (event.key === 'End') { caret.index = caret.nodes.length - 1; caret.offset = Array.from(caret.nodes[caret.index].data).length; }
      if (event.key === 'ArrowRight') {
        if (caret.offset < length) caret.offset += 1;
        else if (caret.index + 1 < caret.nodes.length) { caret.index += 1; caret.offset = 0; }
      }
      if (event.key === 'ArrowLeft') {
        if (caret.offset > 0) caret.offset -= 1;
        else if (caret.index > 0) { caret.index -= 1; caret.offset = Array.from(caret.nodes[caret.index].data).length; }
      }
    }
    this.positionCaret();
  }

  positionCaret() {
    if (this.insertionMarker && this.insertionRange) {
      const bounds = this.insertionRange.getBoundingClientRect();
      const top = document.getElementById('hp-header').getBoundingClientRect().bottom;
      const reader = document.getElementById('hp-reader').getBoundingClientRect();
      const bottom = document.body.classList.contains('hp-annotating') && reader.height > 0 ? Math.min(window.innerHeight, reader.bottom) : window.innerHeight;
      this.insertionMarker.hidden = bounds.height === 0 || bounds.bottom <= top || bounds.top >= bottom;
      this.insertionMarker.style.left = bounds.left + 'px'; this.insertionMarker.style.top = bounds.top + 'px';
      this.insertionMarker.style.height = bounds.height + 'px';
    }
    if (!this.keyboardCaret || !this.caretMarker) return;
    const bounds = this.keyboardRange().getBoundingClientRect();
    this.caretMarker.style.left = bounds.left + 'px'; this.caretMarker.style.top = bounds.top + 'px';
  }

  keyboardRange() {
    const caret = this.keyboardCaret; const node = caret.nodes[caret.index]; const range = document.createRange();
    range.setStart(node, Array.from(node.data).slice(0, caret.offset).join('').length); range.collapse(true); return range;
  }

  placeEndnotes(sidebar) {
    if (!this.endnotes || !this.endnotesSlot?.isConnected) return;
    if (sidebar) this.list.append(this.endnotes);
    else this.endnotesSlot.after(this.endnotes);
    this.endnotes.hidden = false;
  }

  async replaceSource(state) {
    for (let attempt = 0; attempt < 3; attempt += 1) {
      const html = await this.request(this.data.page_url, { html: true });
      if (this.disposed) return state;
      const page = new DOMParser().parseFromString(html, 'text/html');
      const metadata = page.getElementById('hp-annotation-data');
      const data = metadata && JSON.parse(metadata.content.textContent);
      if (data && ['revision', 'source_revision', 'body_revision'].every(key => data[key] === state[key])) {
        await this.swapRegions(page, data); return state;
      }
      state = validateAnnotationState(await this.request(this.data.endpoint));
    }
    throw new Error('The source is still changing. Waiting for a stable version; your draft is retained.');
  }

  async swapRegions(page, data) {
    if (!page.getElementById('hp-document') || !page.getElementById('hp-header')) throw new Error('The refreshed document is incomplete.');
    const folds = new Map();
    for (const [key, id] of Object.entries(this.data.explicit_ids || {})) {
      const node = document.getElementById(id);
      if (node) folds.set(key, node.getAttribute('data-hp-fold-mode'));
    }
    const scroll = window.scrollY; const focus = document.activeElement;
    let anchor;
    for (const [key, id] of Object.entries(this.data.explicit_ids || {})) {
      const node = document.getElementById(id); const top = node?.getBoundingClientRect().top;
      if (top !== undefined && top <= 0) anchor = { key, top };
    }
    this.clearInsertionPoint();
    await this.reinitialize(() => {
      for (const id of ['hp-header', 'hp-frontmatter', 'hp-toc', 'hp-document-title', 'hp-document', 'hp-source-data', 'hp-source-text', 'hp-annotation-data']) {
        const old = document.getElementById(id); const fresh = page.getElementById(id);
        if (fresh && old) old.replaceWith(document.importNode(fresh, true));
        else if (old) old.remove();
        else if (fresh) {
          const next = [...fresh.parentElement.children].slice([...fresh.parentElement.children].indexOf(fresh) + 1).map(node => document.getElementById(node.id)).find(Boolean);
          const inserted = document.importNode(fresh, true);
          if (next) next.before(inserted); else document.getElementById(fresh.parentElement.id)?.append(inserted);
        }
      }
      document.title = page.title;
      document.body.classList.toggle('hp-has-nav', Boolean(page.getElementById('hp-toc')));
      for (const [key, mode] of folds) {
        const node = document.getElementById(data.explicit_ids?.[key]);
        if (node && ['all', 'children', 'folded'].includes(mode)) node.setAttribute('data-hp-visibility', mode);
      }
    });
    this.data = data; this.attachToggle(); this.renderComments(); this.prepareKeyboard(!this.panel.hidden);
    if (focus?.isConnected) focus.focus({ preventScroll: true });
    const node = anchor && document.getElementById(data.explicit_ids?.[anchor.key]);
    const position = node ? window.scrollY + node.getBoundingClientRect().top - anchor.top : scroll;
    window.scrollTo(0, Math.max(0, Math.min(position, document.documentElement.scrollHeight - window.innerHeight)));
  }

  dispose() {
    this.disposed = true; this.cancelPoll(); this.controller.abort(); this.composer?.dispose();
    this.markerObserver?.disconnect(); this.clearInsertionPoint();
    this.prepareKeyboard(false);
    this.placeEndnotes(false); this.endnotesSlot?.remove(); document.body.classList.remove('hp-annotating');
    this.toggle.remove(); this.panel.remove(); this.notice?.remove();
  }
}

const annotationMetadata = document.getElementById('hp-annotation-data');
if (annotationMetadata) {
  previewLifecycle.ready.then(() => new AnnotationPanel(JSON.parse(annotationMetadata.content.textContent))).catch(error => {
    const status = annotationElement('p', 'Annotations unavailable: ' + error.message, 'hp-annotation-status');
    status.setAttribute('role', 'status'); document.getElementById('hp-header').after(status);
  });
}
