{ self, tools }:
# home-manager module. ZDE installs Zinc in its layer 1, so this is the shape it
# arrives in: enable it, get the tools on PATH.
#
# `tools` defaults to the container-side three rather than all five, because that is
# what a desktop needs to define and run apps. zvr is opt-in since a VM runner without
# qemu on the machine is a binary that cannot work, and zlg is opt-in because a desktop
# shipping its own launcher does not want a second one on PATH.
{ config, lib, pkgs, ... }:
let cfg = config.programs.zinc;
in {
  options.programs.zinc = {
    enable = lib.mkEnableOption "the Zinc sandboxing tools";

    tools = lib.mkOption {
      type = lib.types.listOf (lib.types.enum (builtins.attrNames tools));
      default = [ "zc" "zcr" "zlt" ];
      example = [ "zc" "zcr" "zvr" "zlt" "zlg" ];
      description = ''
        Which Zinc tools to put on PATH. zc authors app files, zcr runs container
        apps, zvr runs VM apps, zlt and zlg are the terminal and Wayland launchers.

        zc shells out to whichever runner owns an app, so installing zc without zcr
        gives you authoring and no way to run what you authored.
      '';
    };

    packages = lib.mkOption {
      type = lib.types.attrsOf lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system};
      defaultText = lib.literalExpression "zinc.packages.\${system}";
      description = "The package set the selected tools are taken from. Override to build Zinc from a different source.";
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = map (name: cfg.packages.${name}) cfg.tools;

    # Deliberately NOT done here: installing podman or qemu. Both are system-level on
    # NixOS (virtualisation.podman, and the kvm group for qemu), a home-manager module
    # cannot enable them, and pulling copies into the user profile would produce a
    # second podman that does not share the system's storage or its rootless setup.
    # The tools report a missing runtime clearly when it is absent.
    warnings = lib.optional (!(builtins.elem "zc" cfg.tools) && cfg.tools != [ ])
      "programs.zinc: no zc, so there is nothing to author the apps the installed runners run.";
  };
}
