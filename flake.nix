{
  description = "Snapshot and restore ghostty window layouts and zmx sessions for quick project switching";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    treefmt-nix.url = "github:numtide/treefmt-nix";
    treefmt-nix.inputs.nixpkgs.follows = "nixpkgs";
    git-hooks.url = "github:cachix/git-hooks.nix";
    git-hooks.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      self,
      nixpkgs,
      treefmt-nix,
      git-hooks,
    }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system nixpkgs.legacyPackages.${system});

      treefmtEval = forAllSystems (
        system: pkgs:
        treefmt-nix.lib.evalModule pkgs {
          projectRootFile = "flake.nix";
          programs.gofmt.enable = true;
          programs.nixfmt.enable = true;
          programs.shellcheck.enable = true;
          programs.mdformat = {
            enable = true;
            plugins = ps: [ ps.mdformat-frontmatter ];
            settings.number = true;
            settings.wrap = 80;
          };
        }
      );

      preCommitCheck = forAllSystems (
        system: pkgs:
        git-hooks.lib.${system}.run {
          src = self;
          hooks = {
            treefmt = {
              enable = true;
              package = treefmtEval.${system}.config.build.wrapper;
            };
            golangci-lint = {
              enable = true;
              # golangci-lint's own hook runs per-package by default; run it
              # over the whole module the same way `nix build`'s checkPhase
              # runs `go test ./...` over the whole module. It also shells
              # out to `go` itself (package loading, gci formatter), which
              # isn't implicitly on PATH in the hook's sandbox.
              entry = "${pkgs.coreutils}/bin/env PATH=${pkgs.go}/bin:$PATH ${pkgs.golangci-lint}/bin/golangci-lint run ./...";
              pass_filenames = false;
            };
            gitleaks = {
              enable = true;
              name = "gitleaks";
              entry = "${pkgs.gitleaks}/bin/gitleaks git --pre-commit --redact --staged --verbose";
              pass_filenames = false;
            };
          };
        }
      );
    in
    {
      packages = forAllSystems (
        system: pkgs: {
          herd = pkgs.buildGoModule {
            pname = "herd";
            version = "0.1.0";
            src = self;
            vendorHash = null; # no external Go dependencies
            subPackages = [ "cmd/herd" ];

            nativeBuildInputs = [ pkgs.installShellFiles ];

            # subPackages narrows checkPhase's default `go test` too; run the
            # full suite explicitly so package tests outside cmd/herd count.
            checkPhase = ''
              runHook preCheck
              go test ./...
              runHook postCheck
            '';

            postInstall = ''
              install -Dm444 -t $out/share/herd/shell shell/herd.bash shell/herd.fish shell/herd.zsh

              installShellCompletion --bash --name herd completions/herd.bash
              installShellCompletion --zsh --name _herd completions/herd.zsh
              installShellCompletion --fish --name herd.fish completions/herd.fish
            '';

            meta = with pkgs.lib; {
              description = "Snapshot and restore ghostty window layouts and zmx sessions for quick project switching";
              homepage = "https://github.com/jcam3ron/herd";
              license = licenses.mit;
              mainProgram = "herd";
              platforms = [
                "x86_64-linux"
                "aarch64-darwin"
              ];
            };
          };
          default = self.packages.${system}.herd;
        }
      );

      formatter = forAllSystems (system: pkgs: treefmtEval.${system}.config.build.wrapper);

      checks = forAllSystems (
        system: pkgs: {
          formatting = treefmtEval.${system}.config.build.check self;
          pre-commit-check = preCommitCheck.${system};
        }
      );

      devShells = forAllSystems (
        system: pkgs: {
          default = pkgs.mkShell {
            inherit (preCommitCheck.${system}) shellHook;
            packages = [
              pkgs.go
              pkgs.gopls
              pkgs.golangci-lint
              pkgs.gitleaks
            ];
          };
        }
      );
    };
}
