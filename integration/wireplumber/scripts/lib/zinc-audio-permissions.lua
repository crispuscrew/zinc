local permissions = {}

function permissions.apply(state, client, session)
  local identifier = state.id(client)
  local previous = state.grants[identifier] or {}
  local granted, changes = {}, { any = "-" }
  for global in pairs(previous) do changes[global] = "-" end
  if session and session.ready and not session.dead then
    granted[0], granted[identifier] = "rx", "rwxm"
    for _, endpoint in ipairs(session.endpoints) do granted[endpoint.node_id] = "rx" end
    for factory in state.objects:iterate { type = "factory" } do
      local name = factory.properties["factory.name"]
      if name == "client-node" or name == "link-factory" then granted[state.id(factory)] = "rx" end
    end
    for node in state.objects:iterate { type = "node" } do
      if tonumber(node.properties["client.id"]) == identifier then granted[state.id(node)] = "rwxm" end
    end
    for port in state.objects:iterate { type = "port" } do
      if granted[tonumber(port.properties["node.id"])] then granted[state.id(port)] = "rx" end
    end
  end
  for global, value in pairs(granted) do changes[global] = value end
  client:update_permissions(changes)
  state.grants[identifier] = granted
end

function permissions.refresh(state)
  for client in state.objects:iterate { type = "client" } do
    if state.is_sandbox(client) then permissions.apply(state, client, state.session_for(client)) end
  end
end

return permissions
