{ lib, buildGoModule }:

buildGoModule {
  pname = "nekomonogatari-bot";
  version = "0.1.0";

  src = lib.cleanSourceWith {
    src = ./.;
    filter =
      path: type:
      let
        name = baseNameOf path;
      in
      !(
        name == ".git"
        || name == ".devenv"
        || name == ".direnv"
        || name == ".env"
        || lib.hasPrefix ".env." name
        || name == "config.yaml"
        || name == "node_modules"
        || name == "result"
        || lib.hasSuffix ".db" name
        || lib.hasSuffix ".db-shm" name
        || lib.hasSuffix ".db-wal" name
      );
  };

  vendorHash = "sha256-bjjpjBIahrojK/XzC1wyyt/NubJug1YoOPcnsZKfLRc=";

  env.CGO_ENABLED = "0";
  subPackages = [ "." ];

  checkPhase = ''
    runHook preCheck
    go test ./...
    runHook postCheck
  '';

  meta = {
    description = "Plugin-based Telegram group bot for Forgejo registration and Minecraft whitelisting";
    homepage = "https://forge.asnk.io/sugar/nekomonogatari-bot";
    license = lib.licenses.agpl3Only;
    mainProgram = "nekomonogatari-bot";
    platforms = lib.platforms.unix;
  };
}
