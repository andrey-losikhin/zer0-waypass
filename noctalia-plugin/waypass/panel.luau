--!nonstrict
-- Public values are ephemeral; secret values never enter this module.
local HELPER = "zer0-waypass-helper"
local SESSION_TTL_MS = 3 * 60 * 1000
local query, allItems, items, fields = "", {}, {}, {}
local selectedIndex, fieldIndex = 0, 0
local mode, busy, message = "list", false, ""
local generation, revision, cardEntryID, cardLabel = 0, nil, nil, nil
local rememberedQuery, rememberedEntryID, rememberedFieldID = "", nil, nil
local rememberedCard, rememberedAtMs = false, 0
local restoreEntryID, restoreFieldID, restoreCard = nil, nil, false
local pendingCopyArgv = nil
local PANEL_ID = "zer0/waypass:waypass"
-- locked: gpg-agent has no cached key, so decrypting would open pinentry under
-- this layer-shell surface. Unlock runs only after the panel is closed.
local locked, pendingUnlockID, allowPromptID, panelOpen = false, nil, nil, false

local tr = noctalia.tr
local usageStats = require("./shared.luau")
local usage = {}

local function ttlArg()
  local ttl = tonumber(noctalia.getConfig("clipboard_ttl")) or 30
  return tostring(math.floor(math.max(5, math.min(120, ttl))))
end
local function copyArgv(...)
  local argv = { HELPER, "copy", ... }
  table.insert(argv, "--ttl"); table.insert(argv, ttlArg())
  if noctalia.getConfig("notify_on_error") ~= false then table.insert(argv, "--notify") end
  return argv
end
local function recordUsage(id, weight)
  local updated = usageStats.record(id, weight)
  if updated ~= nil then usage = updated end
end

local function selected(list, index) if index < 1 or index > #list then return nil end; return list[index] end
-- ui.scroll cannot scroll to a child, so only a window that follows the
-- keyboard selection is rendered in a plain column. Labels are capped with
-- maxLines, so row heights are fixed and the window fits the 520 px panel.
local LIST_WINDOW, CARD_WINDOW = 8, 5
local listTop, fieldTop = 1, 1
local function windowTop(top, index, count, size)
  if count <= size then return 1 end
  if index >= 1 and index < top then top = index elseif index >= top + size then top = index - size + 1 end
  return math.max(1, math.min(top, count - size + 1))
end
local function moreRow(key, text, onClick)
  return ui.row({key=key,paddingH=12,onClick=onClick},{ui.label({text=text,color="primary"})})
end
local renderList, renderCard, openCard, back, copyField, applyFilter
local function resetUsage() usageStats.reset(); usage = {}; applyFilter(); renderList() end

renderList = function()
  local rows = {}
  listTop = windowTop(listTop, selectedIndex, #items, LIST_WINDOW)
  local last = math.min(#items, listTop + LIST_WINDOW - 1)
  if listTop > 1 then table.insert(rows, moreRow("more-up", tr("list.more_up",{count=listTop-1}), function() selectedIndex=math.max(1,listTop-LIST_WINDOW); listTop=selectedIndex; renderList() end)) end
  for index = listTop, last do
    local item = items[index]
    local active = index == selectedIndex
    table.insert(rows, ui.row({ key="entry-"..item.id, gap=10, paddingV=9, paddingH=12, radius=10, fill=active and "primary/0.12" or "surface", border=active and "primary" or "surface", borderWidth=1, onClick=function() selectedIndex=index; renderList() end }, {
      ui.glyph({ name=active and "chevron-right" or "key", size=16, color=active and "primary" or "on_surface_variant" }),
      ui.label({ text=item.label, color=active and "primary" or "on_surface", flexGrow=1, maxLines=1 }),
      item.frequent and ui.label({ text=tr("list.frequent"), color="on_surface_variant" }) or nil,
    }))
  end
  if last < #items then table.insert(rows, moreRow("more-down", tr("list.more_down",{count=#items-last}), function() selectedIndex=math.min(#items,last+1); renderList() end)) end
  if #rows == 0 then table.insert(rows, ui.label({ text=busy and tr("list.searching") or (message~="" and message or tr("list.empty")), color="on_surface_variant" })) end
  panel.render(ui.column({ flexGrow=1, gap=12, align="stretch" }, {
    ui.row({ gap=10, align="center" }, { ui.glyph({name="search",size=18}), ui.input({key="waypass-query",value=query,placeholder=tr("list.placeholder"),flexGrow=1,focus=true,onChange="onQueryChange",onSubmit="onQuerySubmit"}), ui.button({glyph="history-off",variant="ghost",tooltip=tr("list.reset_usage"),onClick=resetUsage}) }),
    ui.label({text=tr("list.hint"),color="on_surface_variant"}), ui.column({flexGrow=1,gap=4,align="stretch"},rows),
  }))
end

renderCard = function()
  local rows = {}
  fieldTop = windowTop(fieldTop, fieldIndex, #fields, CARD_WINDOW)
  local last = math.min(#fields, fieldTop + CARD_WINDOW - 1)
  if fieldTop > 1 then table.insert(rows, moreRow("more-up", tr("list.more_up",{count=fieldTop-1}), function() fieldIndex=math.max(1,fieldTop-CARD_WINDOW); fieldTop=fieldIndex; renderCard() end)) end
  for index = fieldTop, last do
    local field = fields[index]
    local active = index == fieldIndex
    local value = field.totp and tr("card.totp_value") or (field.visibility == "public" and (field.value or "") or tr("card.hidden"))
    table.insert(rows, ui.column({ key="field-"..field.id,gap=3,paddingV=9,paddingH=12,radius=10,fill=active and "primary/0.12" or "surface",border=active and "primary" or "surface",borderWidth=1,onClick=function() fieldIndex=index; renderCard() end }, {
      ui.row({gap=8,align="center"},{ui.label({text=index<=9 and tostring(index) or "",color="primary"}),ui.label({text=field.name,fontWeight="medium",flexGrow=1,maxLines=1}),ui.label({text=field.totp and tr("card.copy_code") or tr("card.copy"),color="on_surface_variant"})}),
      ui.label({text=value,color="on_surface_variant",maxLines=1}),
    }))
  end
  if last < #fields then table.insert(rows, moreRow("more-down", tr("list.more_down",{count=#fields-last}), function() fieldIndex=math.min(#fields,last+1); renderCard() end)) end
  if locked then
    table.insert(rows,ui.column({gap=8,paddingV=12,paddingH=12,radius=10,fill="primary/0.12",border="primary",borderWidth=1,onClick=function() copyField() end},{
      ui.row({gap=8,align="center"},{ui.glyph({name="lock",size=16,color="primary"}),ui.label({text=tr("card.locked_title"),fontWeight="medium",flexGrow=1}),ui.label({text=tr("card.unlock"),color="primary"})}),
      ui.label({text=tr("card.locked_body"),color="on_surface_variant",maxLines=3}),
    }))
  elseif #rows == 0 then table.insert(rows,ui.label({text=busy and tr("card.loading") or (message~="" and message or tr("card.empty")),color="on_surface_variant"})) end
  local hint=locked and tr("card.hint_locked") or tr("card.hint")
  panel.render(ui.column({flexGrow=1,gap=12,align="stretch"},{
    ui.row({gap=8,align="center"},{ui.button({text=tr("card.back"),variant="ghost",onClick=back}),ui.glyph({name="key",size=18,color="primary"}),ui.label({text=cardLabel or tr("card.entry"),fontWeight="medium",flexGrow=1})}),
    ui.label({text=hint,color="on_surface_variant",maxLines=1}),ui.column({flexGrow=1,gap=4,align="stretch"},rows),
  }))
end

-- Fuzzy-filters the entry list loaded on open; typing never spawns the helper.
-- Empty query: usage weight, then helper order. Query: exact matches first,
-- then fuzzy score plus a log-scaled usage bonus.
applyFilter = function()
  items = {}
  local text = noctalia.string.trim(query)
  local now = noctalia.nowMs()
  for order, item in ipairs(allItems) do
    local weight = usageStats.weight(usage[item.id], now)
    if text == "" then
      table.insert(items, { id=item.id, label=item.label, frequent=weight>=usageStats.FREQUENT_MIN, exact=false, score=weight, order=order })
    else
      local score = noctalia.fuzzyScore(text, item.label)
      if score ~= nil then
        local exact = score >= usageStats.EXACT_SCORE
        table.insert(items, { id=item.id, label=item.label, exact=exact, score=exact and score or score+usageStats.queryBonus(weight), order=order })
      end
    end
  end
  table.sort(items, function(a, b)
    if a.exact ~= b.exact then return a.exact end
    if a.score ~= b.score then return a.score > b.score end
    return a.order < b.order
  end)
  selectedIndex, listTop = #items > 0 and 1 or 0, 1
end

local function search()
  generation += 1; local current = generation
  busy,allItems,items,selectedIndex,message=true,{},{},0,""; renderList()
  noctalia.runAsync({HELPER,"list"},function(result)
    if mode~="list" or current~=generation then return end
    busy,allItems,usage=false,{},{}
    if type(result)=="table" and result.exitCode==0 then local d=noctalia.json.decode(result.stdout or ""); if type(d)=="table" and type(d.items)=="table" then
      local valid={}
      for _,v in ipairs(d.items) do if type(v)=="table" and type(v.id)=="string" and type(v.label)=="string" then table.insert(allItems,{id=v.id,label=v.label}); valid[v.id]=true end end
      usage=usageStats.load(valid)
    end else message=tr("list.error") end
    applyFilter()
    if restoreEntryID~=nil then for index,v in ipairs(items) do if v.id==restoreEntryID then selectedIndex=index; break end end end
    local shouldRestore=restoreCard and selected(items,selectedIndex)~=nil and selected(items,selectedIndex).id==restoreEntryID
    restoreEntryID=nil; restoreCard=false
    renderList(); if shouldRestore then openCard(false) end
  end)
end

openCard = function(userAction)
  local item=selected(items,selectedIndex); if busy or item==nil then return end
  if userAction~=false then recordUsage(item.id,usageStats.WEIGHT_OPEN) end
  generation+=1; local current=generation
  mode,busy,fields,fieldIndex,revision,cardEntryID,cardLabel,message,locked="card",true,{},0,nil,item.id,item.label,"",false; renderCard()
  local argv={HELPER,"fields",item.id}
  if allowPromptID==item.id then table.insert(argv,"--allow-prompt") end
  allowPromptID=nil
  noctalia.runAsync(argv,function(result)
    if mode~="card" or current~=generation or cardEntryID~=item.id then return end
    busy,fields=false,{}
    local failure=type(result)=="table" and result.exitCode~=0 and noctalia.json.decode(result.stderr or "") or nil
    if type(failure)=="table" and type(failure.error)=="table" and failure.error.code=="backend_locked" then locked=true; restoreFieldID=nil; renderCard(); return end
    if type(result)=="table" and result.exitCode==0 then
      local d=noctalia.json.decode(result.stdout or "")
      if type(d)=="table" and d.protocol==2 and type(d.revision)=="string" and type(d.fields)=="table" then
        revision=d.revision
        for _,f in ipairs(d.fields) do
          if type(f)=="table" and type(f.id)=="string" and type(f.name)=="string" and (f.visibility=="public" or f.visibility=="secret") then
            -- TOTP seeds are never shown or copied; the helper copies a fresh code.
            local totp=f.kind=="totp_secret" or (d.revision=="" and (f.name:lower()=="totp" or f.name:lower()=="otpauth"))
            table.insert(fields,{id=f.id,name=f.name,kind=f.kind,visibility=f.visibility,multiline=f.multiline==true,value=f.visibility=="public" and f.value or nil,legacy=d.revision=="",totp=totp})
          end
        end
      else message=tr("card.error") end
    else message=tr("card.error") end
    fieldIndex,fieldTop=#fields>0 and 1 or 0,1
    if restoreFieldID~=nil then for index,f in ipairs(fields) do if f.id==restoreFieldID then fieldIndex=index; break end end end
    restoreFieldID=nil; renderCard()
  end)
end

copyField = function()
  if busy then return end
  if locked then pendingUnlockID=cardEntryID; panel.close(); return end
  local field=selected(fields,fieldIndex); if field==nil then return end
  local argv
  if field.totp then argv=copyArgv("totp",cardEntryID)
  elseif field.legacy then argv=copyArgv("legacy-field",cardEntryID,field.id)
  elseif revision~=nil then argv=copyArgv("field",cardEntryID,revision,field.id)
  else return end
  recordUsage(cardEntryID,(field.totp or field.kind=="password") and usageStats.WEIGHT_PRIMARY or usageStats.WEIGHT_FIELD)
  -- Do not start gopass until onClose: a layer-shell surface is always drawn
  -- above regular pinentry windows, even when pinentry owns keyboard focus.
  pendingCopyArgv=argv
  panel.close()
end
back = function() generation+=1; mode,busy,fields,fieldIndex,revision,cardEntryID,cardLabel,locked="list",false,{},0,nil,nil,nil,false; rememberedCard=false; restoreFieldID=nil; renderList() end

function onOpen(_context)
  panelOpen=true
  local now=noctalia.nowMs(); local valid=rememberedAtMs>0 and now>=rememberedAtMs and now-rememberedAtMs<=SESSION_TTL_MS
  query,mode,items,fields,selectedIndex,fieldIndex=valid and rememberedQuery or "","list",{},{},0,0
  restoreEntryID=valid and rememberedEntryID or nil; restoreFieldID=valid and rememberedFieldID or nil; restoreCard=valid and rememberedCard or false
  if not valid then rememberedQuery,rememberedEntryID,rememberedFieldID,rememberedCard,rememberedAtMs="",nil,nil,false,0 end
  generation+=1; search()
end
function onClose()
  panelOpen=false
  local copyArgv, unlockID = pendingCopyArgv, pendingUnlockID; pendingCopyArgv, pendingUnlockID = nil, nil
  rememberedQuery=query; rememberedAtMs=noctalia.nowMs()
  if mode=="card" and cardEntryID~=nil then local f=selected(fields,fieldIndex); rememberedEntryID=cardEntryID; rememberedFieldID=f~=nil and f.id or nil; rememberedCard=true
  else local item=selected(items,selectedIndex); rememberedEntryID=item~=nil and item.id or nil; rememberedFieldID=nil; rememberedCard=false end
  generation+=1; allItems,items,fields,revision,cardEntryID,locked={},{},{},nil,nil,false
  if copyArgv~=nil then noctalia.runAsync(copyArgv) end
  if unlockID~=nil then
    noctalia.runAsync({HELPER,"unlock",unlockID},function(result)
      if type(result)~="table" or result.exitCode~=0 then return end
      -- The key is cached now; restore the remembered card without a lock check.
      allowPromptID=unlockID; rememberedAtMs=noctalia.nowMs()
      if not panelOpen then noctalia.togglePanel(PANEL_ID) end
    end,60000)
  end
end
function onQueryChange(value) if mode=="list" and not busy then query=type(value)=="string" and value or ""; restoreEntryID,restoreFieldID,restoreCard=nil,nil,false; applyFilter(); renderList() elseif mode=="list" then query=type(value)=="string" and value or "" end end
function onQuerySubmit() if mode=="list" then openCard() else copyField() end end
function onKey(chord,pressed)
  if not pressed then return end; local key=tostring(chord):lower()
  if mode=="card" then
    if key=="escape" or key=="backspace" then back()
    elseif key=="down" and #fields>0 then fieldIndex=fieldIndex%#fields+1; renderCard()
    elseif key=="up" and #fields>0 then fieldIndex=(fieldIndex-2)%#fields+1; renderCard()
    elseif key=="return" or key=="enter" then copyField()
    else local n=tonumber(key); if n~=nil and n>=1 and n<=9 and n<=#fields then fieldIndex=n; copyField() end end
  else
    if key=="escape" then panel.close()
    elseif key=="down" and #items>0 then selectedIndex=selectedIndex%#items+1; renderList()
    elseif key=="up" and #items>0 then selectedIndex=(selectedIndex-2)%#items+1; renderList()
    elseif key=="return" or key=="enter" then openCard() end
  end
end
