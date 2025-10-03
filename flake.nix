{
  inputs.nixpkgs.url = "github:nixos/nixpkgs";
  inputs.fenix = {
    url = "github:nix-community/fenix";
    inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    { fenix, nixpkgs, ... }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };

      rustToolchain = fenix.packages.${system}.fromToolchainFile {
        file = ./rust-toolchain.toml;
        sha256 = "sha256-SJwZ8g0zF2WrKDVmHrVG3pD2RGoQeo24MEXnNx5FyuI=";
      };
    in
    {
      devShells.${system}.default = pkgs.mkShell {
        buildInputs = with pkgs; [
          # # Rust toolchain
          rustToolchain

          go
          gopls

          # Development tools
          git
          curl
          wget
          jq

          # Build dependencies
          pkg-config
          openssl
          openssl.dev

          # System libraries that might be needed
          gcc
          glibc

          # Optional but useful tools
          cargo-edit # Cargo subcommands for editing Cargo.toml

          # For HTTP/networking projects
          netcat
          nmap
          wireshark-cli

          nil
          nixfmt-rfc-style
        ];

        # Environment variables
        PKG_CONFIG_PATH = "${pkgs.openssl.dev}/lib/pkgconfig";
        OPENSSL_DIR = "${pkgs.openssl.dev}";
        OPENSSL_LIB_DIR = "${pkgs.openssl.out}/lib";
        OPENSSL_INCLUDE_DIR = "${pkgs.openssl.dev}/include";

        shellHook = ''
          echo "🦀 Rust development environment loaded!"
          echo "Rust version: $(rustc --version)"
          echo "Cargo version: $(cargo --version)"
          echo ""
          echo "Available tools:"
          echo "  - rust-analyzer (LSP server)"
          echo "  - clippy (linter)"
          echo "  - rustfmt (formatter)"
          echo "  - bacon (background checker)"
          echo "  - cargo-watch (file watcher)"
          echo ""
          echo "Run 'cargo --help' to see available commands"
        '';
      };
    };
}
