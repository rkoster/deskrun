{ config, pkgs, lib, ... }:

let
  deskrunWrapper = pkgs.writeShellScriptBin "deskrun" ''
    exec nix run github:rkoster/deskrun -- "$@"
  '';
in
{
  nix.settings.experimental-features = [ "nix-command" "flakes" ];

  virtualisation.docker = {
    enable = true;
    autoPrune = {
      enable = true;
      dates = "weekly";
    };
  };

  environment.systemPackages = with pkgs; [
    docker
    kind
    kubectl
    git
    curl
    htop
    deskrunWrapper
  ];

  boot.kernel.sysctl = {
    "fs.inotify.max_user_watches" = 524288;
    "fs.inotify.max_user_instances" = 512;
    # Disable IPv6 to avoid connection delays with Happy Eyeballs
    "net.ipv6.conf.all.disable_ipv6" = 1;
    "net.ipv6.conf.default.disable_ipv6" = 1;
  };

  # Disable IPv6 in networking configuration
  networking.enableIPv6 = false;

  systemd.services.docker.wantedBy = [ "multi-user.target" ];
}
