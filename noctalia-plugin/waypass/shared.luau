--!nonstrict
-- Entry usage statistics (frecency) shared by the panel and launcher entries.
-- Entries run in separate VMs, so every write re-reads the file first.
-- usage.json holds only opaque entry IDs and numbers: never labels, field
-- values, or query texts (ADR 0005).

export type Usage = { score: number, last_used: number }
export type UsageMap = { [string]: Usage }

local M = {}

M.WEIGHT_PRIMARY = 1.0 -- copy password or TOTP
M.WEIGHT_FIELD = 0.5 -- copy any other field
M.WEIGHT_OPEN = 0.1 -- open an entry card
-- fzy returns this for a full (case-insensitive) match; such rows form their own tier.
M.EXACT_SCORE = 1024
M.FREQUENT_MIN = 0.05

local MAX_ENTRIES = 200
local DAY_MS = 86400000
local QUERY_BONUS = 0.5
local HALF_LIFE_DAYS: { [string]: number } = { ["7"] = 7, ["14"] = 14, ["30"] = 30 }

function M.enabled(): boolean
  return noctalia.getConfig("smart_sort") ~= false
end

local function halfLifeMs(): number
  return (HALF_LIFE_DAYS[tostring(noctalia.getConfig("usage_half_life_days"))] or 14) * DAY_MS
end

local function usagePath(): string?
  local dir = noctalia.pluginDataDir()
  if type(dir) ~= "string" or dir == "" then return nil end
  return dir .. "/usage.json"
end

function M.weight(usage: Usage?, now: number): number
  if usage == nil then return 0 end
  local dt = math.max(0, now - usage.last_used)
  return usage.score * 2 ^ (-dt / halfLifeMs())
end

function M.queryBonus(weight: number): number
  return QUERY_BONUS * math.log(1 + weight, 2)
end

local function finite(value: any): boolean
  return type(value) == "number" and value == value and value > -math.huge and value < math.huge
end

local function readMap(): UsageMap
  local result: UsageMap = {}
  local path = usagePath()
  if path == nil or not noctalia.fileExists(path) then return result end
  local text = noctalia.readFile(path)
  local decoded = type(text) == "string" and noctalia.json.decode(text) or nil
  if type(decoded) ~= "table" or type(decoded.entries) ~= "table" then return result end
  for id, usage in pairs(decoded.entries) do
    if type(id) == "string" and id:match("^[A-Za-z0-9_-]+$") ~= nil and type(usage) == "table"
      and finite(usage.score) and usage.score > 0 and finite(usage.last_used) then
      result[id] = { score = usage.score, last_used = usage.last_used }
    end
  end
  return result
end

local function writeMap(map: UsageMap): boolean
  local path = usagePath()
  if path == nil then return false end
  local text = noctalia.json.encode({ version = 1, entries = map })
  if type(text) ~= "string" then return false end
  local tmp = path .. ".tmp"
  if not noctalia.writeFile(tmp, text) then return false end
  return (noctalia.renameFile(tmp, path))
end

-- Keeps the MAX_ENTRIES heaviest entries; returns true when something was dropped.
local function cap(map: UsageMap, now: number): boolean
  local ids = {}
  for id in pairs(map) do table.insert(ids, id) end
  if #ids <= MAX_ENTRIES then return false end
  table.sort(ids, function(a, b) return M.weight(map[a], now) > M.weight(map[b], now) end)
  for index = MAX_ENTRIES + 1, #ids do map[ids[index]] = nil end
  return true
end

-- validIDs must come from a complete, successful `list`; nil skips pruning.
function M.load(validIDs: { [string]: boolean }?): UsageMap
  if not M.enabled() then return {} end
  local map = readMap()
  local changed = false
  if validIDs ~= nil then
    for id in pairs(map) do
      if not validIDs[id] then map[id] = nil; changed = true end
    end
  end
  if cap(map, noctalia.nowMs()) then changed = true end
  if changed then writeMap(map) end
  return map
end

function M.record(id: string, weight: number): UsageMap?
  if not M.enabled() then return nil end
  local map = readMap()
  local now = noctalia.nowMs()
  map[id] = { score = M.weight(map[id], now) + weight, last_used = now }
  cap(map, now)
  writeMap(map)
  return map
end

function M.reset()
  local path = usagePath()
  if path ~= nil and noctalia.fileExists(path) then noctalia.removeFile(path) end
end

return M
