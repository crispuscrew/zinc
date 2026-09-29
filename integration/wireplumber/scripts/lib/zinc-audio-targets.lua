local targets = {}

local function default_name(state, direction)
  local metadata = state.objects:lookup {
    type = "metadata", Constraint { "metadata.name", "=", "default" },
  }
  local key = direction == "microphone" and "default.audio.source" or "default.audio.sink"
  local value = metadata and metadata:find(0, key)
  if not value then error("session default unavailable: " .. direction) end
  local parsed = Json.Raw(value):parse()
  if type(parsed) ~= "table" or type(parsed.name) ~= "string" then error("invalid session default") end
  return parsed.name
end

local function resolve(state, selection)
  local direction = selection.direction
  if direction ~= "playback" and direction ~= "microphone" and direction ~= "monitor" then
    error("invalid audio direction")
  end
  local name = selection.default and default_name(state, direction) or selection.name
  if type(name) ~= "string" or name == "" or #name > 1024 then error("invalid node.name") end
  local expected = direction == "microphone" and "Audio/Source" or "Audio/Sink"
  local found
  for node in state.objects:iterate { type = "node" } do
    if node.properties["node.name"] == name then
      if found then error("ambiguous node.name: " .. name) end
      if node.properties["media.class"] ~= expected or not state.trusted_node(node) then
        error("wrong direction or untrusted node: " .. name)
      end
      found = node
    end
  end
  if not found then error("named node unavailable: " .. name) end
  return found, name
end

function targets.prepare(state, client)
  local request = Json.Raw(client.properties["zinc.audio.request"] or "{}"):parse()
  if type(request) ~= "table" or type(request.app) ~= "string" or request.app == "" or
      type(request.instance) ~= "string" or type(request.token) ~= "string" or
      #request.token ~= 32 or not request.token:match("^[0-9a-f]+$") or
      type(request.selections) ~= "table" or #request.selections == 0 or #request.selections > 32 then
    error("invalid broker request")
  end
  if state.sessions[request.instance] then error("instance already registered") end
  local session = { controller = client, controller_id = state.id(client), app = request.app, instance = request.instance,
    token = request.token, endpoints = {}, modules = {}, ready = false, dead = false }
  local seen = {}
  for _, selection in ipairs(request.selections) do
    local node, name = resolve(state, selection)
    local serial = tostring(node.properties["object.serial"])
    local key = selection.direction .. ":" .. serial
    if not seen[key] then
      local alias = "za." .. request.token .. "." .. tostring(#session.endpoints+1)
      table.insert(session.endpoints, { direction = selection.direction, name = alias,
        target = name, target_id = state.id(node), target_serial = serial })
      seen[key] = true
    end
  end
  return session
end

return targets
