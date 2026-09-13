// ABOUTME: Maps authored DOM selections to canonical Unicode scalar offsets.
// ABOUTME: Reattaches only exact uniquely corroborated passages after source refresh.
const annotationBlocks = new Set('address article aside blockquote br caption dd details div dl dt figcaption figure h1 h2 h3 h4 h5 h6 hr li main ol p pre section summary table tbody td tfoot th thead tr ul'.split(' '));
const annotationSpace = /[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+/gu;

function authoredText(root) {
  const pieces = [];
  function visit(node) {
    if (node.nodeType === Node.TEXT_NODE) { pieces.push(node.data); return; }
    const tag = node.nodeType === Node.ELEMENT_NODE ? node.tagName.toLowerCase() : '';
    if (['script', 'style', 'button', 'nav', 'textarea', 'template'].includes(tag)) return;
    const block = annotationBlocks.has(tag);
    if (block) pieces.push(' ');
    for (const child of node.childNodes) visit(child);
    if (block) pieces.push(' ');
  }
  visit(root);
  return pieces.join('').replace(annotationSpace, ' ').replace(/^ | $/g, '');
}

export function canonicalMap(root) {
  const text = authoredText(root);
  const scalars = Array.from(text);
  return {
    text,
    selector(range, bodyRevision, explicitIDs) {
      if (!root.contains(range.startContainer) || !root.contains(range.endContainer) || range.collapsed) return null;
      const exact = authoredText(range.cloneContents());
      if (!exact || new TextEncoder().encode(exact).length > 8192) return null;
      const prefixRange = document.createRange();
      prefixRange.selectNodeContents(root); prefixRange.setEnd(range.startContainer, range.startOffset);
      let start = Array.from(authoredText(prefixRange.cloneContents())).length;
      while (scalars[start] === ' ') start += 1;
      const end = start + Array.from(exact).length;
      if (scalars.slice(start, end).join('') !== exact) return null;
      const target = { type: 'text', body_revision: bodyRevision, exact, prefix: scalars.slice(Math.max(0, start - 64), start).join(''), suffix: scalars.slice(end, end + 64).join(''), start, end };
      const ancestor = range.commonAncestorContainer.nodeType === Node.ELEMENT_NODE ? range.commonAncestorContainer : range.commonAncestorContainer.parentElement;
      const section = ancestor.closest('section[id]');
      if (section) {
        const entry = Object.entries(explicitIDs).find(([, id]) => id === section.id);
        if (entry) target.heading_id = entry[0];
      }
      return target;
    },
  };
}

export function resolveAnnotationTarget(target, text, bodyRevision, headings = {}) {
  if (target.type === 'document') return { target, status: 'resolved' };
  const body = Array.from(text); const exact = Array.from(target.exact);
  if (target.body_revision === bodyRevision && body.slice(target.start, target.end).join('') === target.exact) return { target, status: 'resolved' };
  const positions = [];
  for (let at = 0; at + exact.length <= body.length; at += 1) {
    if (body.slice(at, at + exact.length).join('') !== target.exact) continue;
    if (target.prefix && !body.slice(Math.max(0, at - Array.from(target.prefix).length), at).join('').endsWith(target.prefix)) continue;
    if (target.suffix && !body.slice(at + exact.length, at + exact.length + Array.from(target.suffix).length).join('').startsWith(target.suffix)) continue;
    const heading = headings[target.heading_id];
    if (heading && (at < heading.start || at + exact.length > heading.end)) continue;
    positions.push(at);
  }
  if (positions.length !== 1) return { target, status: positions.length ? 'ambiguous' : 'missing' };
  return { target: { ...target, body_revision: bodyRevision, start: positions[0], end: positions[0] + exact.length }, status: 'resolved' };
}
