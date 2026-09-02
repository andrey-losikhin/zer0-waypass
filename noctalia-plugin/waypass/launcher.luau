--!nonstrict
-- zer0-waypass launcher provider: metadata only. The helper contract exposes
-- only opaque id/label; this module must never persist, notify, or display
-- decrypted fields (including username, URL, or password).

local HELPER = "zer0-waypass-helper"
-- Noctalia bounds callback-style runAsync calls; debounce_ms in plugin.toml
-- additionally coalesces rapid launcher queries.

local function rows(decoded)
  local result = {}
  if type(decoded) ~= "table" or type(decoded.items) ~= "table" then
    return result
  end
  for _, item in ipairs(decoded.items) do
    if type(item) == "table" and type(item.id) == "string" and type(item.label) == "string" then
      table.insert(result, {
        id = "copy:password:" .. item.id,
        title = item.label .. " — password",
        subtitle = "Copy password (up to 30 seconds)",
        glyph = "key-round",
      })
      table.insert(result, {
        id = "copy:username:" .. item.id,
        title = item.label .. " — username",
        subtitle = "Copy username (up to 30 seconds)",
        glyph = "user-round",
      })
    end
  end
  return result
end

local function stateRow(title, subtitle, glyph)
  return { id = "state", title = title, subtitle = subtitle, glyph = glyph }
end

function onQuery(query)
  local text = noctalia.string.trim(query or "")
  local argv = { HELPER, "list" }
  if text ~= "" then
    table.insert(argv, text)
  end
  noctalia.runAsync(argv, function(result)
    if type(result) ~= "table" or result.exitCode == nil then
      launcher.setResults(query, {
        stateRow("zer0-waypass helper unavailable", "Install or start zer0-waypass-helper", "plug-zap"),
      })
      return
    end
    if result.exitCode ~= 0 then
      launcher.setResults(query, {
        stateRow("Backend error", "Unable to read password-store metadata", "triangle-alert"),
      })
      return
    end
    local decoded = noctalia.json.decode(result.stdout or "")
    local items = rows(decoded)
    if #items == 0 then
      launcher.setResults(query, {
        stateRow("No entries found", "Try a different search", "search-x"),
      })
      return
    end
    launcher.setResults(query, items)
  end)
end

function onActivate(id)
  local action, entryID = id:match("^copy:(username):([A-Za-z0-9_-]+)$")
  if action == nil then
    action, entryID = id:match("^copy:(password):([A-Za-z0-9_-]+)$")
  end
  if action == nil or entryID == nil then
    return
  end
  -- No callback: this is intentionally detached dispatch. The helper owns
  -- decrypt/clipboard lifecycle and no secret or result returns to Noctalia.
  noctalia.runAsync({ HELPER, "copy", action, entryID, "--ttl", "30" })
end
