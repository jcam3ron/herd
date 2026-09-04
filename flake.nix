{
  description = "Snapshot and restore ghostty window layouts and zmx sessions for quick project switching";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      forAllSystems = f:
        nixpkgs.lib.genAttrs [ "x86_64-linux" "aarch64-darwin" ]
          (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: {
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
            platforms = [ "x86_64-linux" "aarch64-darwin" ];
          };
        };
        default = self.packages.${pkgs.stdenv.hostPlatform.system}.herd;
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.gopls ];
        };
      });
    };
}
