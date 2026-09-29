local bridges = {}

function bridges.create(session)
  for _, endpoint in ipairs(session.endpoints) do
    local playback = endpoint.direction == "playback"
    local device = {
      ["node.name"] = endpoint.name, ["node.description"] = endpoint.direction,
      ["media.class"] = playback and "Audio/Sink" or "Audio/Source",
      ["node.virtual"] = true, ["priority.session"] = 0,
      ["node.autoconnect"] = false,
    }
    local stream = {
      ["node.name"] = endpoint.name .. ".bridge",
      ["target.object"] = endpoint.target_serial,
      ["media.class"] = playback and "Stream/Output/Audio" or "Stream/Input/Audio",
      ["node.passive"] = true, ["node.dont-fallback"] = true,
      ["node.dont-reconnect"] = true, ["node.dont-move"] = true,
      ["stream.capture.sink"] = endpoint.direction == "monitor",
    }
    local args = Json.Object {
      ["capture.props"] = Json.Object(playback and device or stream),
      ["playback.props"] = Json.Object(playback and stream or device),
    }
    local module = LocalModule("libpipewire-module-loopback", args:get_data(), {})
    if not module then error("could not create private audio bridge") end
    table.insert(session.modules, module)
  end
end

local function node_named(state, name)
  local found
  for node in state.objects:iterate { type = "node" } do
    if node.properties["node.name"] == name then
      if found or not state.our_node(node) then error("private endpoint identity collision") end
      found = node
    end
  end
  return found
end

function bridges.check(state, session)
  local complete = true
  for _, endpoint in ipairs(session.endpoints) do
    local target = state.lookup("node", endpoint.target_id)
    local expected = endpoint.direction == "microphone" and "Audio/Source" or "Audio/Sink"
    if not target or tostring(target.properties["object.serial"]) ~= endpoint.target_serial or
        target.properties["node.name"] ~= endpoint.target or target.properties["media.class"] ~= expected or
        not state.trusted_node(target) then
      error("selected target disappeared or changed")
    end
    local device = node_named(state, endpoint.name)
    local stream = node_named(state, endpoint.name .. ".bridge")
    if not device or not stream then
      if session.ready then error("private bridge disappeared") end
      complete = false
    else
      endpoint.node_id, endpoint.bridge_id = state.id(device), state.id(stream)
      local linked = false
      for link in state.objects:iterate { type = "link" } do
        local props = link.properties
        local input, output = tonumber(props["link.input.node"]), tonumber(props["link.output.node"])
        if (input == endpoint.target_id and output == endpoint.bridge_id) or
            (output == endpoint.target_id and input == endpoint.bridge_id) then linked = true end
      end
      if not linked then complete = false end
    end
  end
  return complete
end

function bridges.destroy(state, session)
  -- Destroy graph endpoints deterministically; 0.5.14 LocalModule has no unload().
  for _, endpoint in ipairs(session.endpoints) do
    for _, identifier in ipairs { endpoint.node_id, endpoint.bridge_id } do
      local node = state.lookup("node", identifier)
      if node and state.our_node(node) then node:request_destroy() end
    end
  end
  session.modules = {}
end

return bridges
