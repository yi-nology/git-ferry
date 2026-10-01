{
  description = "Self-hosted Git sync / mirror / backup hub";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system);
    in
    {
      packages = forAllSystems (system:
        let pkgs = import nixpkgs { inherit system; };
        in {
          default = pkgs.buildGoModule {
            pname = "git-ferry";
            version = "1.19.1";
            src = ./.;
            # vendorHash 首次构建后由 nix 提示填入
            vendorHash = null;
            proxyVendor = true;
            subPackages = [ "." "cmd/gitferry" ];
            meta = with pkgs.lib; {
              description = "Self-hosted Git sync / mirror / backup hub";
              homepage = "https://github.com/yi-nology/git-ferry";
              license = licenses.mit;
              mainProgram = "git-ferry";
            };
          };
        });

      nixosModules.default = { config, lib, pkgs, ... }:
        with lib;
        let cfg = config.services.gitferry;
        in {
          options.services.gitferry = {
            enable = mkEnableOption "GitFerry";
            package = mkOption {
              type = types.package;
              default = self.packages.${pkgs.system}.default;
            };
            dataDir = mkOption {
              type = types.path;
              default = "/var/lib/gitferry";
            };
            openFirewall = mkOption {
              type = types.bool;
              default = false;
            };
            port = mkOption {
              type = types.port;
              default = 8890;
            };
          };
          config = mkIf cfg.enable {
            systemd.services.gitferry = {
              description = "GitFerry";
              after = [ "network-online.target" ];
              wantedBy = [ "multi-user.target" ];
              serviceConfig = {
                ExecStart = "${cfg.package}/bin/git-ferry";
                WorkingDirectory = cfg.dataDir;
                DynamicUser = true;
                StateDirectory = "gitferry";
                Restart = "on-failure";
              };
            };
            networking.firewall.allowedTCPPorts = mkIf cfg.openFirewall [ cfg.port ];
          };
        };
    };
}
