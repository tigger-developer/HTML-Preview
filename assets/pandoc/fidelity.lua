-- ABOUTME: Preserves display metadata and decorates semantic Org inline values.
-- ABOUTME: Runs only inside Pandoc and never reads files or executes source code.
local function mark_inlines(inlines)
  local out = pandoc.List()
  local i = 1
  while i <= #inlines do
    local item = inlines[i]
    if item.t == "Str" and item.text:match("^%[#%w%]$") then
      out:insert(pandoc.Span({ item }, pandoc.Attr("", { "priority" })))
    elseif
      item.t == "Str" and (item.text:match("^%[%d*/%d*%]") or item.text:match("^%[%d+%%%]"))
    then
      out:insert(pandoc.Span({ item }, pandoc.Attr("", { "cookie" })))
    elseif item.t == "Str" and item.text:match("^[<%[]%d%d%d%d%-%d%d%-%d%d") then
      local close = item.text:sub(1, 1) == "<" and ">" or "]"
      local parts = pandoc.List()
      local last = i
      local found = false
      while last <= #inlines do
        local part = inlines[last]
        if part.t ~= "Str" and part.t ~= "Space" then
          break
        end
        parts:insert(part)
        if part.t == "Str" and part.text:find(close, 1, true) then
          found = true
          break
        end
        last = last + 1
      end
      if found then
        out:insert(pandoc.Span(parts, pandoc.Attr("", { "timestamp" })))
        i = last
      else
        out:insert(item)
      end
    else
      out:insert(item)
    end
    i = i + 1
  end
  return out
end

local function display_metadata(doc)
  local prefix = pandoc.List()
  for _, key in ipairs({ "title", "author", "date", "lang" }) do
    local value = doc.meta[key]
    if value then
      local text = pandoc.utils.stringify(value)
      prefix:insert(pandoc.Para({ pandoc.Str(text) }))
    end
  end
  prefix:extend(doc.blocks)
  doc.blocks = prefix
  doc.meta = {}
  return doc
end

return { { Inlines = mark_inlines }, { Pandoc = display_metadata } }
