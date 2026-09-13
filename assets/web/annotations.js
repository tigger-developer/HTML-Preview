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
  if (code === 'stale_source' || code === 'stale_body') return 'The source changed. Waiting for a refreshed preview.';
  if (code?.startsWith('target_')) return 'The selected passage is unavailable or no longer unique. Your draft is retained.';
  if (['source_replaced', 'source_changed', 'unsafe_source', 'unsafe_sidecar'].includes(code)) return 'The file identity changed or is unsafe to append. Copy the draft and reopen after checking the file.';
  if (['corrupt_store', 'store_changed', 'foreign_sidecar', 'unsupported_store'].includes(code)) return 'Stored annotation data needs inspection. Earlier records and this draft are retained; no repair is automatic.';
  if (['closed_comment', 'composer_conflict', 'sequence_conflict'].includes(code)) return 'This comment cannot accept another revision. Copy the draft and create a new comment.';
  if (code === 'operation_conflict') return 'The service found conflicting saved data. Copy the draft before retrying.';
  if (status === 413) return 'The comment, selected passage or annotation history exceeds its storage limit.';
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

export class AnnotationPanel {
  constructor(data, options = {}) {
    this.data = data; this.request = options.request || annotationRequest;
    this.clock = options.clock || { now: () => performance.now(), setTimeout: (fn, ms) => setTimeout(fn, ms), clearTimeout: id => clearTimeout(id) };
    this.reinitialize = options.reinitialize || reinitializePreview;
    this.controller = new AbortController(); this.events = { signal: this.controller.signal };
    this.disposed = false; this.composer = null; this.poll = null; this.pendingRefresh = null; this.failures = 0;
    this.panel = annotationElement('aside', '', 'hp-annotations'); this.panel.id = 'hp-annotations'; this.panel.hidden = true;
    this.panel.setAttribute('aria-label', 'Annotations');
    this.toggle = annotationButton('Annotations'); this.toggle.setAttribute('aria-controls', this.panel.id); this.toggle.setAttribute('aria-expanded', 'false');
    this.add = annotationButton('Add comment'); this.add.disabled = true;
    this.connection = annotationElement('p', 'Loading annotations…', 'hp-annotation-status');
    this.connection.setAttribute('role', 'status'); this.connection.setAttribute('aria-live', 'polite');
    this.reconnect = annotationButton('Reconnect'); this.reconnect.hidden = true;
    this.list = annotationElement('div'); this.editor = annotationElement('div');
    this.appendix = annotationElement('section', '', 'hp-annotation-print'); this.appendix.setAttribute('aria-label', 'Saved annotations');
    this.panel.append(annotationElement('h2', 'Annotations'), this.connection, this.reconnect, this.add, this.editor, this.list);
    document.body.append(this.panel, this.appendix); this.attachToggle();
    this.listen();
    this.ready = this.refresh().catch(error => this.showFailure(error));
  }

  attachToggle() { (document.querySelector('#hp-header .hp-toolbar') || document.getElementById('hp-header-row')).append(this.toggle); }

  listen() {
    this.toggle.addEventListener('click', () => {
      this.panel.hidden = !this.panel.hidden; this.toggle.setAttribute('aria-expanded', String(!this.panel.hidden));
      if (!this.panel.hidden && matchMedia('(max-width:45rem)').matches) this.panel.scrollIntoView({ block: 'start' });
    }, this.events);
    this.reconnect.addEventListener('click', () => this.refresh().catch(error => this.showFailure(error)), this.events);
    this.add.addEventListener('click', () => this.openComposer(), this.events);
    document.addEventListener('selectionchange', () => {
      const selection = window.getSelection(); const main = document.getElementById('hp-document');
      if (selection?.rangeCount && !selection.isCollapsed && main.contains(selection.anchorNode) && main.contains(selection.focusNode)) this.selection = selection.getRangeAt(0).cloneRange();
      else if (selection?.isCollapsed && main.contains(selection.anchorNode)) this.selection = null;
    }, this.events);
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
  }

  guardNavigation(event) {
    const link = event.target instanceof Element && event.target.closest('a[href]');
    if (!link || !this.composer || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || link.target === '_blank' || link.hasAttribute('download')) return;
    const url = new URL(link.href);
    if (url.origin !== location.origin || (url.pathname === location.pathname && url.search === location.search)) return;
    event.preventDefault(); event.stopImmediatePropagation();
    this.closeComposer().then(() => location.assign(url.href)).catch(error => this.showComposerFailure(error));
  }

  openComposer() {
    if (!this.state?.writable || this.composer) return;
    const main = document.getElementById('hp-document');
    let target = { type: 'document' };
    if (this.selection && !this.selection.collapsed) {
      target = canonicalMap(main).selector(this.selection, this.state.body_revision, this.data.explicit_ids || {});
      if (!target) { this.connection.textContent = 'Select a shorter passage entirely inside the document, or clear the selection for a document comment.'; return; }
    }
    this.editor.replaceChildren(); this.add.disabled = true;
    this.editor.append(annotationElement('p', target.exact || 'Whole document', 'hp-annotation-quote'));
    this.textarea = annotationElement('textarea'); this.textarea.id = 'hp-annotation-text'; this.textarea.rows = 6;
    const label = annotationElement('label', 'Your comment'); label.htmlFor = this.textarea.id;
    this.status = annotationElement('p', 'Not saved', 'hp-annotation-status'); this.status.setAttribute('role', 'status'); this.status.setAttribute('aria-live', 'polite');
    const close = annotationButton('Close comment'); const retry = annotationButton('Retry'); const copy = annotationButton('Copy draft'); const restore = annotationButton('Restore last autosave');
    this.recovery = annotationElement('div', '', 'hp-annotation-actions'); this.recovery.hidden = true; this.recovery.append(retry, copy, restore);
    this.editor.append(label, this.textarea, this.status, close, this.recovery);
    this.composer = new AnnotationComposer({ clock: this.clock, revisions: annotationRevisions(this.state), target,
      send: (request, secret) => this.request(this.data.endpoint, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-HTMLPreview-Annotation-Token': this.state.write_token, 'X-HTMLPreview-Composer-Token': secret }, body: JSON.stringify(request) }),
      onState: state => {
        this.status.textContent = state.status + (state.error ? ': ' + state.error.message : ''); this.recovery.hidden = !state.error;
        if (this.composer?.sequence && this.displayedSequence !== this.composer.sequence) { this.displayedSequence = this.composer.sequence; this.renderComments(this.state.events); }
      },
      onStale: async current => { await this.refresh(); return this.rebaseTarget(current); },
    });
    this.textarea.addEventListener('input', () => this.composer.input(this.textarea.value), this.events);
    this.textarea.addEventListener('compositionstart', () => this.composer.composition(true), this.events);
    this.textarea.addEventListener('compositionend', () => this.composer.composition(false), this.events);
    close.addEventListener('click', () => this.closeComposer().catch(error => this.showComposerFailure(error)), this.events);
    retry.addEventListener('click', () => this.retryComposer(), this.events);
    restore.addEventListener('click', () => { this.textarea.value = this.composer.savedText; this.composer.restoreSaved(); this.textarea.focus(); }, this.events);
    copy.addEventListener('click', () => {
      if (!navigator.clipboard) { this.textarea.focus(); this.textarea.select(); this.status.textContent = 'Select and copy the draft using your keyboard.'; return; }
      navigator.clipboard.writeText(this.textarea.value).then(() => { this.status.textContent = 'Draft copied.'; }, () => { this.textarea.focus(); this.textarea.select(); this.status.textContent = 'Clipboard unavailable. Select and copy the draft.'; });
    }, this.events);
    this.textarea.focus();
  }

  async retryComposer() {
    try { await this.refresh(); await this.composer.retry(); }
    catch (error) { this.showComposerFailure(error); }
  }

  async closeComposer() {
    if (!this.composer || this.closing) return this.closing;
    this.textarea.readOnly = true;
    this.closing = this.composer.close();
    try {
      await this.closing;
      this.composer.dispose(); this.composer = null; this.editor.replaceChildren(); this.add.disabled = !this.state.writable; this.add.focus();
      await this.refresh();
    } finally { this.closing = null; if (this.composer) this.textarea.readOnly = false; }
  }

  showComposerFailure(error) {
    if (this.disposed) return;
    this.status.textContent = 'Not saved: ' + error.message; this.recovery.hidden = false;
    this.panel.hidden = false; this.toggle.setAttribute('aria-expanded', 'true'); this.textarea.focus();
  }

  showFailure(error) {
    if (this.disposed) return;
    this.connection.textContent = 'Annotations unavailable: ' + error.message;
    this.reconnect.hidden = false;
    this.add.disabled = true;
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
    let state = await this.request(this.data.endpoint);
    if (this.disposed) return;
    if (state.protocol !== 1 || !Array.isArray(state.events)) throw new Error('Unsupported annotation state.');
    if (state.source_revision !== this.data.source_revision) state = await this.replaceSource(state);
    if (this.disposed) return;
    const changed = !this.state || this.state.revision !== state.revision;
    this.state = state; this.data.revision = state.revision;
    this.reconnect.hidden = true;
    this.connection.textContent = state.writable ? (state.storage === 'sidecar' ? 'Comments autosave beside this read-only source.' : 'Comments autosave in this document.') : 'Reading only: ' + state.reason.replaceAll('_', ' ');
    this.add.disabled = !state.writable || Boolean(this.composer);
    if (changed) this.renderComments(state.events);
    if (this.composer) {
      if (!state.writable) this.composer.suspend('Saving is unavailable: ' + state.reason.replaceAll('_', ' '));
      else if (!this.composer.inFlight) {
        const replacement = this.rebaseTarget(this.composer.target);
        if (replacement) this.composer.rebase(replacement.target, replacement.revisions);
      }
    }
    this.attachToggle();
  }

  rebaseTarget(target) {
    const result = resolveAnnotationTarget(target, canonicalMap(document.getElementById('hp-document')).text, this.state.body_revision, this.headingSpans());
    if (result.status !== 'resolved') { this.composer?.suspend('The selected passage is ' + result.status + '. Your draft is retained.'); return null; }
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

  renderComments(events) {
    const composer = this.composer;
    const acknowledged = composer?.savedReceipt;
    if (acknowledged && this.data.display_name) {
      const existing = events.find(event => event.annotation_id === composer.annotationID);
      if (!existing || existing.sequence < composer.sequence) {
        events = events.filter(event => event.annotation_id !== composer.annotationID).concat({ annotation_id: composer.annotationID, sequence: composer.sequence, author: this.data.display_name, recorded_at: acknowledged.stored_at, text: composer.savedText, target: composer.savedTarget, closed: acknowledged.closed, status: 'resolved' });
      }
    }
    this.list.replaceChildren(); this.appendix.replaceChildren(annotationElement('h2', 'Saved annotations'));
    this.appendix.hidden = events.length === 0;
    for (const event of events) {
      const comment = annotationElement('article', '', 'hp-annotation-comment');
      comment.append(annotationElement('p', event.author + ' · ' + event.recorded_at, 'hp-annotation-author'));
      if (event.target.exact) comment.append(annotationElement('blockquote', event.target.exact));
      comment.append(annotationElement('p', event.text, 'hp-annotation-text'));
      let status = event.closed ? 'Saved' : this.composer?.annotationID === event.annotation_id ? 'Autosaved draft' : 'Recovered draft · read only';
      const unacknowledged = composer?.annotationID === event.annotation_id && event.sequence > composer.sequence;
      if (unacknowledged) status = 'Stored draft · acknowledgement pending';
      if (event.status !== 'resolved') status += ' · ' + event.status;
      comment.append(annotationElement('p', status, 'hp-annotation-status'));
      this.list.append(comment);
      if (!unacknowledged) this.appendix.append(comment.cloneNode(true));
    }
    if (!events.length) this.list.append(annotationElement('p', 'No saved annotations.', 'hp-annotation-status'));
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
      state = await this.request(this.data.endpoint);
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
    await this.reinitialize(() => {
      for (const id of ['hp-header', 'hp-toc', 'hp-document-title', 'hp-document', 'hp-annotation-data']) {
        const old = document.getElementById(id); const fresh = page.getElementById(id);
        if (fresh && old) old.replaceWith(document.importNode(fresh, true));
        else if (old) old.remove();
        else if (fresh) document.getElementById('hp-document').before(document.importNode(fresh, true));
      }
      document.title = page.title;
      document.body.classList.toggle('hp-has-nav', Boolean(page.getElementById('hp-toc')));
      for (const [key, mode] of folds) {
        const node = document.getElementById(data.explicit_ids?.[key]);
        if (node && ['all', 'children', 'folded'].includes(mode)) node.setAttribute('data-hp-visibility', mode);
      }
    });
    this.data = data; this.selection = null; this.attachToggle();
    if (focus?.isConnected) focus.focus({ preventScroll: true });
    const node = anchor && document.getElementById(data.explicit_ids?.[anchor.key]);
    const position = node ? window.scrollY + node.getBoundingClientRect().top - anchor.top : scroll;
    window.scrollTo(0, Math.max(0, Math.min(position, document.documentElement.scrollHeight - window.innerHeight)));
  }

  dispose() {
    this.disposed = true; this.cancelPoll(); this.controller.abort(); this.composer?.dispose();
    this.toggle.remove(); this.panel.remove(); this.appendix.remove();
  }
}

const annotationMetadata = document.getElementById('hp-annotation-data');
if (annotationMetadata) {
  previewLifecycle.ready.then(() => new AnnotationPanel(JSON.parse(annotationMetadata.content.textContent))).catch(error => {
    const status = annotationElement('p', 'Annotations unavailable: ' + error.message, 'hp-annotation-status');
    status.setAttribute('role', 'status'); document.getElementById('hp-header').after(status);
  });
}
