--!nonstrict
-- zer0-waypass launcher provider: metadata only. The helper contract exposes
-- only opaque id/label; this module must never persist, notify, or display
-- decrypted fields (including username, URL, or password).

local HELPER = "zer0-waypass-helper"
-- Noctalia bounds callback-style runAsync calls; debounce_ms in plugin.toml
-- additionally coalesces rapid launcher queries.

local tr = noctalia.tr
local usageStats = require("./shared.luau")

local function ttlArg()
  local ttl = tonumber(noctalia.getConfig("clipboard_ttl")) or 30
  return tostring(math.floor(math.max(5, math.min(120, ttl))))
end

-- complete: the listing was not filtered by a query, so stale usage IDs can be pruned.
local function rows(decoded, complete)
  local result = {}
  if type(decoded) ~= "table" or type(decoded.items) ~= "table" then
    return result
  end
  local entries, valid = {}, {}
  for order, item in ipairs(decoded.items) do
    if type(item) == "table" and type(item.id) == "string" and type(item.label) == "string" then
      table.insert(entries, { id = item.id, label = item.label, order = order })
      valid[item.id] = true
    end
  end
  local usage = usageStats.load(complete and valid or nil)
  local now = noctalia.nowMs()
  for _, entry in ipairs(entries) do entry.weight = usageStats.weight(usage[entry.id], now) end
  table.sort(entries, function(a, b)
    if a.weight ~= b.weight then return a.weight > b.weight end
    return a.order < b.order
  end)
  for _, item in ipairs(entries) do
    table.insert(result, {
      id = "copy:password:" .. item.id,
      title = tr("launcher.password_title", { label = item.label }),
      subtitle = tr("launcher.password_subtitle", { ttl = ttlArg() }),
      glyph = "key",
    })
    table.insert(result, {
      id = "copy:username:" .. item.id,
      title = tr("launcher.username_title", { label = item.label }),
      subtitle = tr("launcher.username_subtitle", { ttl = ttlArg() }),
      glyph = "user-round",
    })
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
        stateRow(tr("launcher.unavailable"), tr("launcher.unavailable_hint"), "plug-zap"),
      })
      return
    end
    if result.exitCode ~= 0 then
      launcher.setResults(query, {
        stateRow(tr("launcher.backend_error"), tr("launcher.backend_error_hint"), "triangle-alert"),
      })
      return
    end
    local decoded = noctalia.json.decode(result.stdout or "")
    local items = rows(decoded, text == "")
    if #items == 0 then
      launcher.setResults(query, {
        stateRow(tr("launcher.empty"), tr("launcher.empty_hint"), "search-x"),
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
  usageStats.record(entryID, action == "password" and usageStats.WEIGHT_PRIMARY or usageStats.WEIGHT_FIELD)
  local argv = { HELPER, "copy", action, entryID, "--ttl", ttlArg() }
  if noctalia.getConfig("notify_on_error") ~= false then table.insert(argv, "--notify") end
  noctalia.runAsync(argv)
end
