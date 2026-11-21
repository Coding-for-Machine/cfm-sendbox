{ pkgs ? import <nixpkgs> {} }:
  pkgs.mkShell {
    # Tools needed to hack on the Go code and docs
    nativeBuildInputs = with pkgs; [ go jq mkdocs ];
}
