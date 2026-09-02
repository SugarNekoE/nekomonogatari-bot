{
  description = "Nekomonogatari Telegram bot";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
      pkgsFor = system: import nixpkgs { inherit system; };
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          nekomonogatari-bot = pkgs.callPackage ./package.nix { };
          default = self.packages.${system}.nekomonogatari-bot;
        }
      );

      checks = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
          testSystem = nixpkgs.lib.nixosSystem {
            inherit system;
            modules = [
              self.nixosModules.default
              {
                system.stateVersion = "26.05";
                services.nekomonogatari-bot = {
                  enable = true;
                  configPath = "/run/secrets/nekomonogatari-bot.yaml";
                };
              }
            ];
          };
          service = testSystem.config.systemd.services.nekomonogatari-bot;
        in
        {
          inherit (self.packages.${system}) nekomonogatari-bot;
          nixos-module =
            pkgs.runCommand "nekomonogatari-bot-module-check"
              {
                configPath = service.environment.CONFIG_PATH;
                execStart = service.serviceConfig.ExecStart;
              }
              ''
                test "$configPath" = /run/secrets/nekomonogatari-bot.yaml
                test -x "$execStart"
                touch "$out"
              '';
        }
      );

      overlays.default = final: _prev: {
        nekomonogatari-bot = final.callPackage ./package.nix { };
      };

      nixosModules = {
        nekomonogatari-bot = import ./nixos-module.nix;
        default = self.nixosModules.nekomonogatari-bot;
      };

      devShells = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          default = pkgs.mkShell {
            packages = [
              pkgs.go
              pkgs.nodejs_24
              pkgs.nixfmt-tree
            ];
          };
        }
      );

      formatter = forAllSystems (system: (pkgsFor system).nixfmt-tree);
    };
}
