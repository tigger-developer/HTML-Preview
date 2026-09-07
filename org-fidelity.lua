--[[
org-fidelity.lua — recover org-mode structure that pandoc's org reader
flattens into plain text, and expose it as styled DOM.

Usage:
  pandoc -f org -t html5 -s --section-divs \
         --lua-filter=org-fidelity.lua \
         --css=org-fidelity.css \
         --include-after-body=org-fold.html \
         doc.org -o doc.html

What it does
  * :PROPERTIES: drawers  -> a foldable <details class="org-properties"> table
                             (pandoc already puts them on the header as
                              data-* attributes; this makes them visible)
  * duplicate id bug      -> :ID: is renamed to data-org-id so it no longer
                             collides with :CUSTOM_ID: / the slug
  * [#A] priority cookies -> <span class="priority priority-A">
  * [1/3] [50%] cookies   -> <span class="cookie">
  * <2026-09-20 Sun>      -> <span class="timestamp active">
    [2026-09-20 Mon]      -> <span class="timestamp inactive">
  * :VISIBILITY: folded   -> left on the section for org-fold.html to honour

What it CANNOT do (the reader drops these before any filter runs):
  * DEADLINE:/SCHEDULED:/CLOSED: planning lines
  * :LOGBOOK: drawers
  See org-planning-prepass.sed for a workaround.
]]

local stringify = pandoc.utils.stringify

-- properties we never want to show in the visible drawer
local HIDDEN_PROPS = { visibility = true }

--------------------------------------------------------------------- helpers

local function is_html()
  return FORMAT:match('html')
end

local function esc(s)
  return (s:gsub('&', '&amp;'):gsub('<', '&lt;'):gsub('>', '&gt;')
           :gsub('"', '&quot;'))
end

--------------------------------------------------------- inline: cookies etc.

-- [#A] / [#B] / [#1]
local function priority_span(text)
  local p = text:match('^%[#(%w)%]$')
  if not p then return nil end
  return pandoc.Span({ pandoc.Str(text) },
                     pandoc.Attr('', { 'priority', 'priority-' .. p },
                                 { { 'priority', p } }))
end

-- [1/3] or [50%] possibly with trailing punctuation
local function cookie_split(text)
  local cookie, rest = text:match('^(%[%d*/%d*%])(.*)$')
  if not cookie then cookie, rest = text:match('^(%[%d+%%%])(.*)$') end
  if not cookie then return nil end
  local done = false
  local n, d = cookie:match('^%[(%d*)/(%d*)%]$')
  if n and d and n ~= '' and n == d then done = true end
  local pct = cookie:match('^%[(%d+)%%%]$')
  if pct and tonumber(pct) == 100 then done = true end
  local classes = { 'cookie' }
  if done then classes[#classes + 1] = 'cookie-done' end
  return pandoc.Span({ pandoc.Str(cookie) }, pandoc.Attr('', classes)), rest
end

-- timestamps span several Str/Space tokens: "<2026-09-20", "Sun", "09:00>"
local TS_OPEN = { ['<'] = '>', ['['] = ']' }

local function timestamp_at(inlines, i)
  local first = inlines[i]
  if first.t ~= 'Str' then return nil end
  local open = first.text:sub(1, 1)
  local close = TS_OPEN[open]
  if not close then return nil end
  if not first.text:match('^.%d%d%d%d%-%d%d%-%d%d') then return nil end
  -- walk forward until a token ends with the matching closer
  for j = i, math.min(i + 6, #inlines) do
    local tok = inlines[j]
    if tok.t == 'Str' then
      local body, trailer = tok.text:match('^(.-%' .. close .. ')(.*)$')
      if body then
        local parts = {}
        for k = i, j do parts[#parts + 1] = stringify(inlines[k]) end
        local text = table.concat(parts, ''):sub(1, -1)
        -- rebuild exact text (stringify drops spaces), so join manually
        text = ''
        for k = i, j do
          local t = inlines[k]
          if t.t == 'Space' then text = text .. ' '
          elseif t.t == 'Str' then text = text .. t.text end
        end
        if trailer ~= '' then text = text:sub(1, #text - #trailer) end
        local classes = { 'timestamp',
                          open == '<' and 'active' or 'inactive' }
        if text:match('%+%d+[hdwmy]') then classes[#classes + 1] = 'repeater' end
        return pandoc.Span({ pandoc.Str(text) }, pandoc.Attr('', classes)),
               j, trailer
      end
    elseif tok.t ~= 'Space' then
      return nil
    end
  end
  return nil
end

local function mark_inlines(inlines)
  local out, i = pandoc.List({}), 1
  local changed = false
  while i <= #inlines do
    local el = inlines[i]
    if el.t == 'Str' then
      local ts, last, trailer = timestamp_at(inlines, i)
      if ts then
        out:insert(ts)
        if trailer and trailer ~= '' then out:insert(pandoc.Str(trailer)) end
        i, changed = last + 1, true
        goto continue
      end
      local pri = priority_span(el.text)
      if pri then
        out:insert(pri); i, changed = i + 1, true; goto continue
      end
      local ck, rest = cookie_split(el.text)
      if ck then
        out:insert(ck)
        if rest ~= '' then out:insert(pandoc.Str(rest)) end
        i, changed = i + 1, true
        goto continue
      end
    end
    out:insert(el)
    i = i + 1
    ::continue::
  end
  if changed then return out end
end

--------------------------------------------------------------- property block

local function properties_block(attr)
  local rows = {}
  for _, kv in ipairs(attr.attributes) do
    local k, v = kv[1], kv[2]
    if not HIDDEN_PROPS[k:lower()] then
      local label = (k:lower() == 'org-id') and 'ID' or k:upper()
      rows[#rows + 1] = string.format(
        '<div class="prop"><dt>:%s:</dt><dd>%s</dd></div>',
        esc(label), esc(v))
    end
  end
  if #rows == 0 then return nil end
  return pandoc.RawBlock('html', table.concat({
    '<details class="org-properties">',
    '<summary>:PROPERTIES:</summary>',
    '<dl>', table.concat(rows, ''), '</dl>',
    '</details>' }, ''))
end

------------------------------------------------------------------- the filter

local function Header(h)
  -- :ID: collides with the element id pandoc already emitted
  local attrs = h.attr.attributes
  if attrs['id'] then
    attrs['org-id'] = attrs['id']
    attrs['id'] = nil
  end
  local marked = mark_inlines(h.content)
  if marked then h.content = marked end

  -- group the trailing :tags: into one container so they can be floated
  -- right without reversing their order
  local content = pandoc.List(h.content)
  -- pandoc separates consecutive tags with a non-breaking space Str
  local function is_gap(el)
    return el.t == 'Space' or el.t == 'SoftBreak'
        or (el.t == 'Str' and el.text:match('^[%s\194\160]*$') ~= nil)
  end
  local first_tag
  for i = #content, 1, -1 do
    local el = content[i]
    if el.t == 'Span' and el.classes:includes('tag') then
      first_tag = i
    elseif not is_gap(el) then
      break
    end
  end
  if first_tag then
    local tags = pandoc.List({})
    for i = first_tag, #content do
      if content[i].t == 'Span' then tags:insert(content[i]) end
    end
    for _ = #content, first_tag, -1 do content:remove(#content) end
    content:insert(pandoc.Span(tags, pandoc.Attr('', { 'tags' })))
    h.content = content
  end
  return h
end

local function Blocks(blocks)
  if not is_html() then return nil end
  local out = pandoc.List({})
  for _, b in ipairs(blocks) do
    out:insert(b)
    if b.t == 'Header' then
      local props = properties_block(b.attr)
      if props then out:insert(props) end
    end
  end
  return out
end

local function Para(p)
  local marked = mark_inlines(p.content)
  if marked then p.content = marked end
  return p
end

local function Plain(p)
  local marked = mark_inlines(p.content)
  if marked then p.content = marked end
  return p
end

-- give unnamed drawers a stable hook and keep the drawer name visible
local function Div(d)
  if d.classes:includes('drawer') then
    for _, c in ipairs(d.classes) do
      if c ~= 'drawer' then
        d.attributes['drawer-name'] = c
        break
      end
    end
  end
  return d
end

return {
  { Header = Header, Para = Para, Plain = Plain, Div = Div },
  { Blocks = Blocks },
}
