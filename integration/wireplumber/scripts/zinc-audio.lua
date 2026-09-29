-- Policy protocol 1; compatibility baseline: WirePlumber 0.5.14 / PipeWire 1.4.11.
local state = require("zinc-audio-state")
local targets = require("zinc-audio-targets")
local bridges = require("zinc-audio-bridges")
local permissions = require("zinc-audio-permissions")
local linking = require("zinc-audio-linking")
local log = Log.open_topic("zinc-audio")

state.objects = ObjectManager {
  Interest { type = "client" }, Interest { type = "node" },
  Interest { type = "port" }, Interest { type = "factory" },
  Interest { type = "metadata" }, Interest { type = "link" },
}

local function revoke(session, reason)
  if session.dead then return end
  session.dead, session.ready = true, false
  permissions.refresh(state)
  bridges.destroy(state, session)
  state.reply(session, "revoked", reason)
end

local function register(client)
  if not state.controller(client) then return end
  local identifier = state.id(client)
  if state.clients[identifier] then return end
  state.clients[identifier] = true
  local success, session = pcall(targets.prepare, state, client)
  if not success then
    state.reply({ controller = client }, "error", tostring(session))
    return
  end
  state.sessions[session.instance] = session
  local created, detail = pcall(bridges.create, session)
  if not created then revoke(session, tostring(detail)) end
end

local function reconcile()
  for client in state.objects:iterate { type = "client" } do register(client) end
  for instance, session in pairs(state.sessions) do
    local owner = state.lookup("client", session.controller_id)
    if not owner then
      session.detached = true
      revoke(session, "controller disconnected")
      state.sessions[instance] = nil
    elseif not session.dead then
      local success, complete = pcall(bridges.check, state, session)
      if not success then revoke(session, tostring(complete))
      elseif complete and not session.ready and not session.pending then
        session.pending = true
        Core.sync(function(detail)
          if session.dead then return end
          if detail then revoke(session, tostring(detail)); return end
          session.ready = true
          local result = {}
          for _, endpoint in ipairs(session.endpoints) do
            table.insert(result, Json.Object {
              direction = endpoint.direction, name = endpoint.name, target = endpoint.target,
            })
          end
          session.controller:update_properties { ["zinc.audio.endpoints"] = Json.Array(result):get_data() }
          permissions.refresh(state)
          state.reply(session, "ready")
        end)
      elseif session.ready and not complete then revoke(session, "bridge link disappeared") end
      local ping = owner.properties["zinc.audio.ping"]
      if session.ready and ping and ping ~= session.ping then
        session.ping = ping
        owner:update_properties { ["zinc.audio.pong"] = ping }
      end
    end
  end
  permissions.refresh(state)
end

state.objects:connect("object-removed", function(_, object)
  state.clients[state.id(object)] = nil
  state.grants[state.id(object)] = nil
end)
state.objects:connect("objects-changed", function() reconcile() end)
linking.install(state)
state.objects:activate()
Core.timeout_add(250, function()
  local success, detail = pcall(reconcile)
  if not success then
    log:warning("policy failure: " .. tostring(detail))
    for _, session in pairs(state.sessions) do revoke(session, "policy failure") end
  end
  return true
end)
