# Symbols Nerd Font Mono provenance

- Upstream release: Nerd Fonts v3.4.0
- Official source TTF: https://raw.githubusercontent.com/ryanoasis/nerd-fonts/v3.4.0/patched-fonts/NerdFontsSymbolsOnly/SymbolsNerdFontMono-Regular.ttf
- License: https://github.com/ryanoasis/nerd-fonts/blob/v3.4.0/LICENSE
- Source TTF SHA-256 (freshly verified): `f0f624d9b474bea1662cf7e862d44aebe1ae1f6c7f9cb7a0ca5d0e5ac9561c60`
- Bundled WOFF2 SHA-256: `a8e2fc5ae3c2525812151b95da80c5beab0befa84aca84fc33aaed94317502df`

Conversion:

```sh
nix shell nixpkgs#woff2 -c woff2_compress SymbolsNerdFontMono-Regular.ttf
```
