# Chamadas VoIP nativas (GOWS)

## Visão geral

O gows-plus integra o stack VoIP do WaCalls (`src/voip/`) no mesmo processo e `whatsmeow.Client` por sessão WAHA.

- **Signaling**: stanzas `<call>` via `DangerousInternalClient`
- **Mídia**: MLow 16 kHz + SRTP + relay SCTP (Meta)
- **Browser**: bridge WebRTC Opus 48 kHz (`src/callbridge/`)

## Build

### Sem áudio (signaling only)

```bash
cd src && go build -o ../bin/gows .
```

### Com áudio (MLow via CGO)

Requer compilador C e bibliotecas em `native/`:

```bash
# Windows (MSYS2)
$env:PATH = "C:\msys64\mingw64\bin;$PWD\native;$env:PATH"
$env:CGO_ENABLED = "1"
go build -tags mlow -o bin/gows ./src

# Linux / Docker
CGO_ENABLED=1 go build -tags mlow -o bin/gows ./src
```

### Docker

A imagem usa `-tags mlow` quando `native/libopus_mlow.so` está presente. Para Linux, obtenha o `.so` a partir do projeto [opus_mlow](https://github.com/edgardmessias/opus_mlow) e coloque em `native/` junto com `libopus-0.so`.

```bash
docker build -t gows-plus .
```

## Requisitos de rede (produção)

| Requisito | Detalhe |
|-----------|---------|
| UDP saída | Relays STUN/SCTP da Meta (porta ~3480) |
| NAT | Container/VM deve conseguir binding UDP |
| Linked device | Sessão pareada como dispositivo vinculado |

Sem conectividade UDP aos relays, a chamada pode chegar a `ringing` mas não a `active`.

## Limitações

- Uma chamada ativa por sessão
- 1:1 voz (vídeo: signaling apenas, sem pipeline VP8)
- Cliente WebRTC necessário para áudio (bots headless precisam de integração futura)

## Teste E2E

1. Suba WAHA com engine GOWS e este binário
2. Implemente REST conforme [`integrations/waha/README.md`](../integrations/waha/README.md)
3. Abra [`tools/call-test-client/index.html`](../tools/call-test-client/index.html) no browser
4. Inicie chamada para um número WhatsApp real

## Variáveis de ambiente

| Variável | Efeito |
|----------|--------|
| `WAHA_GOWS_DEVICE_HISTORY_SYNC_SUPPORT_CALL_LOG_HISTORY` | Registra capability de call log no device |

`PatchDeviceProps()` é chamado no startup de `main.go`.
