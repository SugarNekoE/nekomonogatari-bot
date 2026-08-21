{ pkgs, ... }:

{
  languages = {
    go = {
      enable = true;
      version = "1.26.0";
    };
    javascript = {
      enable = true;
      package = pkgs.nodejs_24;
      corepack = {
        enable = true;
      };
    };
    typescript = {
      enable = true;
    };
  };

  git-hooks = {
    enable = true;
    hooks = {
      convco = {
        enable = true;
      };
    };
  };
}
