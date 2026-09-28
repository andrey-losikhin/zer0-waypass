--!nonstrict
-- Public values are ephemeral; secret values never enter this module.
local HELPER = "zer0-waypass-helper"
local SESSION_TTL_MS = 3 * 60 * 1000
local query, items, fields = "", {}, {}
local selectedIndex, fieldIndex = 0, 0
local mode, busy, message = "list", false, "No entries found"
local generation, revision, cardEntryID, cardLabel = 0, nil, nil, nil
local rememberedQuery, rememberedEntryID, rememberedFieldID = "", nil, nil
local rememberedCard, rememberedAtMs = false, 0
local restoreEntryID, restoreFieldID, restoreCard = nil, nil, false
local pendingCopyArgv = nil
local PANEL_ID = "zer0/waypass:waypass"
-- locked: gpg-agent has no cached key, so decrypting would open pinentry under
-- this layer-shell surface. Unlock runs only after the panel is closed.
local locked, pendingUnlockID, allowPromptID, panelOpen = false, nil, nil, false

local function selected(list, index) if index < 1 or index > #list then return nil end; return list[index] end
-- ui.scroll cannot scroll to a child, so only a window that follows the
-- keyboard selection is rendered; sizes fit the fixed 520 px panel height.
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
local renderList, renderCard, openCard, back, copyField

renderList = function()
  local rows = {}
  listTop = windowTop(listTop, selectedIndex, #items, LIST_WINDOW)
  local last = math.min(#items, listTop + LIST_WINDOW - 1)
  if listTop > 1 then table.insert(rows, moreRow("more-up", "↑ "..(listTop-1).." more", function() selectedIndex=math.max(1,listTop-LIST_WINDOW); listTop=selectedIndex; renderList() end)) end
  for index = listTop, last do
    local item = items[index]
    local active = index == selectedIndex
    table.insert(rows, ui.row({ key="entry-"..item.id, gap=10, paddingV=9, paddingH=12, radius=10, fill=active and "primary/0.12" or "surface", border=active and "primary" or "surface", borderWidth=1, onClick=function() selectedIndex=index; renderList() end }, {
      ui.glyph({ name=active and "chevron-right" or "key-round", size=16, color=active and "primary" or "on_surface_variant" }),
      ui.label({ text=item.label, color=active and "primary" or "on_surface", flexGrow=1 }),
    }))
  end
  if last < #items then table.insert(rows, moreRow("more-down", "↓ "..(#items-last).." more", function() selectedIndex=math.min(#items,last+1); renderList() end)) end
  if #rows == 0 then table.insert(rows, ui.label({ text=busy and "Searching…" or message, color="on_surface_variant" })) end
  panel.render(ui.column({ flexGrow=1, gap=12, align="stretch" }, {
    ui.row({ gap=10, align="center" }, { ui.glyph({name="search",size=18}), ui.input({key="waypass-query",value=query,placeholder="Search password entries",flexGrow=1,focus=true,onChange="onQueryChange",onSubmit="onQuerySubmit"}) }),
    ui.label({text="Enter: open card",color="on_surface_variant"}), ui.scroll({flexGrow=1,gap=4},rows),
  }))
end

renderCard = function()
  local rows = {}
  fieldTop = windowTop(fieldTop, fieldIndex, #fields, CARD_WINDOW)
  local last = math.min(#fields, fieldTop + CARD_WINDOW - 1)
  if fieldTop > 1 then table.insert(rows, moreRow("more-up", "↑ "..(fieldTop-1).." more", function() fieldIndex=math.max(1,fieldTop-CARD_WINDOW); fieldTop=fieldIndex; renderCard() end)) end
  for index = fieldTop, last do
    local field = fields[index]
    local active = index == fieldIndex
    local value = field.visibility == "public" and (field.value or "") or "Hidden — copy only"
    table.insert(rows, ui.column({ key="field-"..field.id,gap=3,paddingV=9,paddingH=12,radius=10,fill=active and "primary/0.12" or "surface",border=active and "primary" or "surface",borderWidth=1,onClick=function() fieldIndex=index; renderCard() end }, {
      ui.row({gap=8,align="center"},{ui.label({text=index<=9 and tostring(index) or "",color="primary"}),ui.label({text=field.name,fontWeight="medium",flexGrow=1}),ui.label({text="Copy",color="on_surface_variant"})}),
      ui.label({text=value,color="on_surface_variant",wrap=true}),
    }))
  end
  if last < #fields then table.insert(rows, moreRow("more-down", "↓ "..(#fields-last).." more", function() fieldIndex=math.min(#fields,last+1); renderCard() end)) end
  if locked then
    table.insert(rows,ui.column({gap=8,paddingV=12,paddingH=12,radius=10,fill="primary/0.12",border="primary",borderWidth=1,onClick=function() copyField() end},{
      ui.row({gap=8,align="center"},{ui.glyph({name="lock",size=16,color="primary"}),ui.label({text="GPG key is locked",fontWeight="medium",flexGrow=1}),ui.label({text="Unlock",color="primary"})}),
      ui.label({text="The panel hides while pinentry asks for the passphrase and reopens after unlock.",color="on_surface_variant",wrap=true}),
    }))
  elseif #rows == 0 then table.insert(rows,ui.label({text=busy and "Loading fields…" or message,color="on_surface_variant"})) end
  local hint=locked and "Enter: unlock · Esc/Backspace: back" or "1–9 / arrows: select · Enter: copy · Esc/Backspace: back"
  panel.render(ui.column({flexGrow=1,gap=12,align="stretch"},{
    ui.row({gap=8,align="center"},{ui.button({text="Back to list",variant="ghost",onClick=back}),ui.glyph({name="key-round",size=18,color="primary"}),ui.label({text=cardLabel or "Entry",fontWeight="medium",flexGrow=1})}),
    ui.label({text=hint,color="on_surface_variant"}),ui.scroll({flexGrow=1,gap=4},rows),
  }))
end

local function search()
  generation += 1; local current, requested = generation, query
  busy,items,selectedIndex,message=true,{},0,"No entries found"; renderList()
  local argv={HELPER,"list"}; if requested~="" then table.insert(argv,requested) end
  noctalia.runAsync(argv,function(result)
    if mode~="list" or current~=generation or requested~=query then return end
    busy,items=false,{}
    if type(result)=="table" and result.exitCode==0 then local d=noctalia.json.decode(result.stdout or ""); if type(d)=="table" and type(d.items)=="table" then for _,v in ipairs(d.items) do if type(v)=="table" and type(v.id)=="string" and type(v.label)=="string" then table.insert(items,{id=v.id,label=v.label}) end end end else message="Unable to read password entries" end
    selectedIndex,listTop=#items>0 and 1 or 0,1
    if restoreEntryID~=nil then for index,v in ipairs(items) do if v.id==restoreEntryID then selectedIndex=index; break end end end
    local shouldRestore=restoreCard and selected(items,selectedIndex)~=nil and selected(items,selectedIndex).id==restoreEntryID
    restoreEntryID=nil; restoreCard=false
    renderList(); if shouldRestore then openCard() end
  end)
end

openCard = function()
  local item=selected(items,selectedIndex); if busy or item==nil then return end
  generation+=1; local current=generation
  mode,busy,fields,fieldIndex,revision,cardEntryID,cardLabel,message,locked="card",true,{},0,nil,item.id,item.label,"No fields available",false; renderCard()
  local argv={HELPER,"fields",item.id}
  if allowPromptID==item.id then table.insert(argv,"--allow-prompt") end
  allowPromptID=nil
  noctalia.runAsync(argv,function(result)
    if mode~="card" or current~=generation or cardEntryID~=item.id then return end
    busy,fields=false,{}
    local failure=type(result)=="table" and result.exitCode~=0 and noctalia.json.decode(result.stderr or "") or nil
    if type(failure)=="table" and type(failure.error)=="table" and failure.error.code=="backend_locked" then locked=true; restoreFieldID=nil; renderCard(); return end
    if type(result)=="table" and result.exitCode==0 then local d=noctalia.json.decode(result.stdout or ""); if type(d)=="table" and d.protocol==2 and type(d.revision)=="string" and type(d.fields)=="table" then revision=d.revision; for _,f in ipairs(d.fields) do if type(f)=="table" and type(f.id)=="string" and type(f.name)=="string" and (f.visibility=="public" or f.visibility=="secret") then local legacy=d.revision=="" and (f.kind=="password" or f.kind=="username") and f.kind or nil; table.insert(fields,{id=f.id,name=f.name,kind=f.kind,visibility=f.visibility,multiline=f.multiline==true,value=f.visibility=="public" and f.value or nil,legacy=legacy}) end end end else fields={{id="legacy-password",name="Password",kind="password",visibility="secret",legacy="password"},{id="legacy-username",name="Username",kind="username",visibility="secret",legacy="username"}}; message="Legacy entry" end
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
  if field.legacy~=nil then argv={HELPER,"copy",field.legacy,cardEntryID,"--ttl","30"}
  elseif revision~=nil then argv={HELPER,"copy","field",cardEntryID,revision,field.id,"--ttl","30"}
  else return end
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
  generation+=1; items,fields,revision,cardEntryID,locked={},{},nil,nil,false
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
function onQueryChange(value) if mode=="list" then query=type(value)=="string" and value or ""; restoreEntryID,restoreFieldID,restoreCard=nil,nil,false; search() end end
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
