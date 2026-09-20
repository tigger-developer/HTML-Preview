// ABOUTME: Guards native textarea edits before they enter annotation autosave.
// ABOUTME: Preserves selections and composition while enforcing length and Org boundaries.
const annotationCharacterLimit = 4000;
const annotationByteLimit = 16384;

function annotationTextSize(text) {
  return { characters: Array.from(text).length, bytes: new TextEncoder().encode(text).length };
}

function annotationTextFits(text) {
  const size = annotationTextSize(text);
  return size.characters <= annotationCharacterLimit && size.bytes <= annotationByteLimit;
}

function annotationOrgBoundary(text) {
  let block = ''; let blanks = 0;
  // The first line follows the footnote label, so cannot itself be a blank line
  // or block delimiter. Subsequent lines follow the writer's literalContext.
  // The final unterminated line is the current typing line, not a completed
  // blank line: "first\n\n" still permits typing the second paragraph.
  for (const line of text.replaceAll('\r\n', '\n').replaceAll('\r', '\n').split('\n').slice(1, -1)) {
    const trimmed = line.trim().toUpperCase();
    if (block) {
      if (trimmed === '#+END_' + block) block = '';
      continue;
    }
    if (!trimmed) {
      blanks += 1;
      if (blanks === 2) return true;
    } else {
      blanks = 0;
      block = trimmed.match(/^#\+BEGIN_(\S+)/)?.[1] || '';
    }
  }
  return false;
}

export function guardAnnotationInput(textarea, { org, onInput, onComposition, onNotice, events }) {
  const capture = () => ({ text: textarea.value, start: textarea.selectionStart, end: textarea.selectionEnd,
    direction: textarea.selectionDirection, scroll: textarea.scrollTop });
  let previous = capture(); let composing = false;
  const rejection = text => {
    if (!annotationTextFits(text)) {
      const before = annotationTextSize(previous.text); const after = annotationTextSize(text);
      // Do not truncate an oversized authored footnote on open. Permit gradual
      // shortening while the composer's existing validity check suspends saving.
      if (!(after.characters < before.characters && after.bytes <= before.bytes)) {
        return '4,000-character limit. Input not added.';
      }
    }
    if (org && annotationOrgBoundary(text)) return 'Use one blank line between paragraphs. Input not added.';
    return '';
  };
  const replaceSelection = text => textarea.value.slice(0, textarea.selectionStart) + text + textarea.value.slice(textarea.selectionEnd);
  const blockInsertion = (event, text) => {
    const message = rejection(replaceSelection(text.replaceAll('\r\n', '\n').replaceAll('\r', '\n')));
    if (!message || !event.cancelable) return;
    event.preventDefault(); onNotice(message);
  };
  const commit = () => {
    if (textarea.value === previous.text) return;
    const message = rejection(textarea.value);
    if (message) {
      textarea.value = previous.text;
      textarea.setSelectionRange(previous.start, previous.end, previous.direction);
      textarea.scrollTop = previous.scroll;
      onNotice(message);
      return;
    }
    previous = capture();
    onNotice(annotationTextFits(previous.text) ? '' : 'Shorten this footnote to 4,000 characters.');
    onInput(previous.text);
  };
  textarea.addEventListener('beforeinput', event => {
    if (composing || event.isComposing) return;
    previous = capture();
    const text = ['insertLineBreak', 'insertParagraph'].includes(event.inputType) ? '\n' : event.data;
    if (event.inputType.startsWith('insert') && typeof text === 'string') blockInsertion(event, text);
  }, events);
  textarea.addEventListener('paste', event => {
    if (composing || !event.clipboardData) return;
    previous = capture(); blockInsertion(event, event.clipboardData.getData('text/plain'));
  }, events);
  textarea.addEventListener('input', event => {
    if (!composing && !event.isComposing) commit();
  }, events);
  textarea.addEventListener('compositionstart', () => {
    previous = capture(); composing = true; onComposition(true);
  }, events);
  textarea.addEventListener('compositionend', () => {
    composing = false; commit(); onComposition(false);
  }, events);
  if (!annotationTextFits(previous.text)) onNotice('Shorten this footnote to 4,000 characters.');
}
