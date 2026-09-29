local linking = {}

function linking.install(state)
  local utils = require("linking-utils")
  SimpleEventHook {
    name = "zinc-audio/select-target",
    before = { "linking/find-defined-target", "linking/find-default-target", "linking/find-best-target",
      "linking/find-filter-target", "linking/prepare-link" },
    interests = { EventInterest { Constraint { "event.type", "=", "select-target" } } },
    execute = function(event)
      local _, manager, item, props = utils:unwrap_select_target_event(event)
      local client_id = props["client.id"]
      if not client_id then
        local node = item:get_associated_proxy("node")
        client_id = node and node.properties["client.id"]
      end
      local client = client_id and state.lookup("client", client_id)
      if not client or not state.is_sandbox(client) then return end
      local session = state.session_for(client)
      local wanted = props["target.object"]
      local playback = props["media.class"] == "Stream/Output/Audio"
      local capture = props["media.class"] == "Stream/Input/Audio"
      if session and session.ready and not session.dead and (playback or capture) then
        for _, endpoint in ipairs(session.endpoints) do
          local matching = playback == (endpoint.direction == "playback")
          if matching and (not wanted or wanted == endpoint.name) then
            local target = manager:lookup { type = "SiLinkable",
              Constraint { "node.id", "=", tostring(endpoint.node_id) } }
            if target then event:set_data("target", target); return end
          end
        end
      end
      -- Prevent every later fallback hook; no default sink/source substitution.
      event:stop_processing()
    end,
  }:register()
end

return linking
