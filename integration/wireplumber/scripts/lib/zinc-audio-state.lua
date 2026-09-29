local state = {
  engine = "com.github.crispuscrew.zinc",
  access = "zinc-audio-v1",
  version = "1",
  sessions = {}, clients = {}, grants = {},
}

function state.id(object) return tonumber(object["bound-id"]) end

function state.lookup(kind, identifier)
  return state.objects:lookup {
    type = kind, Constraint { "bound-id", "=", tonumber(identifier), type = "gobject" },
  }
end

function state.is_sandbox(client)
  return client.properties["pipewire.sec.engine"] == state.engine
end

function state.controller(client)
  local props = client.properties
  return props["pipewire.sec.engine"] == nil and
    props["pipewire.sec.socket"] == Core.get_info().name .. "-manager" and
    props["zinc.audio.protocol"] == state.version
end

function state.trusted_node(node)
  -- Private endpoints must not become another instance's implicit host default.
  if (node.properties["node.name"] or ""):match("^za%.[0-9a-f]+%.") then return false end
  local owner = node.properties["client.id"]
  if owner == nil then return true end
  local client = state.lookup("client", owner)
  return client ~= nil and client.properties["pipewire.sec.engine"] == nil
end

function state.our_node(node)
  local owner = node.properties["client.id"]
  local client = owner and state.lookup("client", owner)
  return client ~= nil and client.properties["pipewire.sec.engine"] == nil and
    client.properties["pipewire.sec.pid"] == tostring(Core.get_properties()["application.process.id"])
end

function state.reply(session, status, detail)
  if session.detached then return end
  session.controller:update_properties {
    ["zinc.audio.version"] = state.version, ["zinc.audio.status"] = status,
    ["zinc.audio.error"] = detail or "",
  }
end

function state.session_for(client)
  local props = client.properties
  local session = state.sessions[props["pipewire.sec.instance-id"]]
  if session and session.app == props["pipewire.sec.app-id"] and
      props["pipewire.access"] == state.access then return session end
end

return state
