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

-- The unpredictable channel is supplied only by the command. It owns records,
-- never source HTML, and is consumed by Go before the passive-content policy.
local function preserve_values(doc)
  local token = doc.meta["htmlpreview-code-token"]
  if type(token) ~= "string" or #token ~= 32 or not token:match("^[0-9a-f]+$") then
    error("missing or malformed htmlpreview transport token")
  end
  doc.meta["htmlpreview-code-token"] = nil
  local code_prefix = "htmlpreview-code-" .. token .. "-"
  local heading_prefix = "htmlpreview-heading-" .. token .. "-"
  local raw_format = "htmlpreview-code-" .. token
  local seen = {}

  local function escape(text)
    return (text:gsub("&", "&amp;"):gsub("<", "&lt;"):gsub(">", "&gt;"))
  end
  local function record(kind, number, value)
    return pandoc.RawBlock(
      "html",
      '<pre id="htmlpreview-'
        .. kind
        .. "-record-"
        .. token
        .. "-"
        .. number
        .. '">'
        .. escape(pandoc.json.encode(value))
        .. "</pre>"
    )
  end
  doc = doc:walk({
    RawBlock = function(block)
      if block.format ~= raw_format then
        return nil
      end
      local ok, value = pcall(pandoc.json.decode, block.text, false)
      if not ok or type(value) ~= "table" then
        error("invalid Org code record")
      end
      local keys = { id = true, text = true, language = true }
      local count = 0
      for key, item in pairs(value) do
        if not keys[key] or type(item) ~= "string" then
          error("invalid Org code fields")
        end
        count = count + 1
      end
      if
        count ~= 3
        or value.id:sub(1, #code_prefix) ~= code_prefix
        or not value.id:sub(#code_prefix + 1):match("^[1-9]%d*$")
        or seen[value.id]
      then
        error("invalid or duplicate Org code identifier")
      end
      seen[value.id] = true
      local classes = value.language ~= "" and { value.language } or {}
      return pandoc.CodeBlock(value.text, pandoc.Attr("", classes))
    end,
  })

  local codes = 0
  doc = doc:walk({
    CodeBlock = function(block)
      codes = codes + 1
      for i, language in ipairs(block.classes) do
        if language == "sh" or language == "shell" then
          block.classes[i] = "bash"
        end
      end
      local id = code_prefix .. codes
      return pandoc.Div(
        { block, record("code", codes, { id = id, text = block.text }) },
        pandoc.Attr(id)
      )
    end,
  })
  local headings = 0
  doc = doc:walk({
    Header = function(header)
      headings = headings + 1
      local id = heading_prefix .. headings
      local original = header.identifier
      header.identifier = id
      return { header, record("heading", headings, { id = id, originalID = original }) }
    end,
  })
  return display_metadata(doc)
end

return { { Inlines = mark_inlines }, { Pandoc = preserve_values } }
