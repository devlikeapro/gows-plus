/**
 * Referência de cliente gRPC para os RPCs de chamada do gows-plus.
 * Integrar no GowsGrpcClient existente do WAHA (NestJS).
 */
export interface StartCallRequest {
  session: { id: string };
  jid: string;
  video?: boolean;
}

export interface AcceptCallRequest {
  session: { id: string };
  call_id: string;
  owner_id?: string;
}

export interface ExchangeCallWebRTCRequest {
  session: { id: string };
  call_id: string;
  sdp_offer: string;
}

export interface CallLifecyclePayload {
  event: 'call.received' | 'call.ringing' | 'call.connecting' | 'call.active' | 'call.ended' | 'call.rejected';
  id: string;
  from: string;
  direction: 'inbound' | 'outbound';
  status: string;
  reason?: string;
  timestamp: number;
}

export const GOWS_CALL_RPCS = {
  startCall: 'StartCall',
  acceptCall: 'AcceptCall',
  rejectCall: 'RejectCall',
  endCall: 'EndCall',
  exchangeCallWebRTC: 'ExchangeCallWebRTC',
  getCallState: 'GetCallState',
} as const;
