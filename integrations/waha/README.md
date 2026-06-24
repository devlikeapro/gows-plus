# Integração WAHA — Chamadas VoIP (GOWS)

Este diretório descreve como o [WAHA](https://github.com/devlikeapro/waha) deve consumir os novos RPCs do gows-plus.

## Novos RPCs gRPC (`MessageService`)

| RPC | Request | Response |
|-----|---------|----------|
| `StartCall` | `session`, `jid` (telefone ou JID), `video` | `call_id` |
| `AcceptCall` | `session`, `call_id`, `owner_id?` | `Empty` |
| `RejectCall` | `session`, `from`, `id` | `Empty` |
| `EndCall` | `session`, `call_id` | `Empty` |
| `ExchangeCallWebRTC` | `session`, `call_id`, `sdp_offer` | `sdp_answer` |
| `GetCallState` | `session` | `active`, `call_id`, `from`, `direction`, `status`, `event` |

## REST WAHA (a implementar no NestJS)

Espelhar as rotas do WaCalls:

```
POST   /api/{session}/calls              → StartCall
POST   /api/{session}/calls/{id}/accept  → AcceptCall (+ header X-Client-Id → owner_id)
POST   /api/{session}/calls/reject       → RejectCall (já existe)
DELETE /api/{session}/calls/{id}         → EndCall
POST   /api/{session}/calls/{id}/webrtc  → ExchangeCallWebRTC
GET    /api/{session}/calls/state        → GetCallState (opcional)
```

### Exemplo `calls.controller.ts`

```typescript
@Post(':session/calls')
start(@Param('session') session: string, @Body() body: { phone?: string; jid?: string; video?: boolean }) {
  const jid = body.jid ?? `${normalizePhone(body.phone)}@c.us`;
  return this.gows.startCall(session, { jid, video: body.video ?? false });
}

@Post(':session/calls/:id/webrtc')
webrtc(@Param('session') session: string, @Param('id') id: string, @Body() body: { sdp_offer: string }) {
  return this.gows.exchangeCallWebRTC(session, { callId: id, sdpOffer: body.sdp_offer });
}
```

## Webhooks / eventos

O `EventStream` gRPC emite `CallLifecycleEvent` com o campo `event` já no formato WAHA:

| `event` (stream) | Quando |
|------------------|--------|
| `call.received` | Chamada entrante (`OnIncoming`) |
| `call.ringing` | Estado ringing |
| `call.connecting` | Aceite / negociação |
| `call.active` | Mídia ativa |
| `call.ended` | Encerramento |

Payload JSON:

```json
{
  "event": "call.active",
  "id": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
  "from": "5511999999999@s.whatsapp.net",
  "direction": "inbound",
  "status": "active",
  "reason": "",
  "timestamp": 1710000000000
}
```

No consumidor NestJS, mapear `EventJson.event === "call.active"` (ou tipo `gows.CallLifecycleEvent`) para webhook HTTP `call.active`.

## Fluxo do cliente de áudio

1. `POST /calls` ou receber webhook `call.received`
2. Browser/app cria `RTCPeerConnection`, gera SDP offer
3. `POST /calls/{id}/webrtc` com `{ "sdp_offer": "..." }`
4. Aplicar `sdp_answer` no peer connection
5. Áudio bidirecional via Opus 48 kHz

Ver [`tools/call-test-client/`](../tools/call-test-client/) para teste manual.
