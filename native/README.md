# Bibliotecas nativas MLow (opus_mlow)

## Windows (desenvolvimento)

Incluídas no repositório:

- `opus_mlow.dll`
- `libopus-0.dll`

Build: `make build-mlow` ou `go build -tags mlow` com `CGO_ENABLED=1` e MinGW no PATH.

## Linux (Docker / produção)

O repositório **não inclui** `.so` por padrão (binários são específicos de plataforma).

Para build Docker com áudio:

1. Compile ou baixe `libopus_mlow.so` e `libopus-0.so` para linux/amd64 (ou arm64)
2. Coloque em `native/`:
   - `native/libopus_mlow.so`
   - `native/libopus.so.0` → symlink para `libopus_mlow.so`
   - `native/libopus-0.so` → symlink para `libopus_mlow.so`
3. `docker build -t gows-plus .`

Sem os `.so`, use build sem tag `mlow` — chamadas funcionam em modo **signaling-only** (sem áudio bidirecional).

Fonte: [github.com/edgardmessias/opus_mlow](https://github.com/edgardmessias/opus_mlow)
