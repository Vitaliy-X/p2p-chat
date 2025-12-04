export interface ConnectRequest {
  username: string;
  room: string;
  db_path: string;
  no_dht: boolean;
}

export interface PeerInfo {
  id: string;
  addresses: string[];
}

export interface Message {
  id: string;
  room: string;
  text: string;
  sender_id: string;
  sender_username: string;
  sent_at: string;
  version: number;
}

export interface State {
  connected: boolean;
  status: string;
  last_error?: string;
  peer_count: number;
  room_peer_count: number;
  peer_info: PeerInfo;
  messages?: Message[];
}

interface AppAPI {
  Connect(req: ConnectRequest): Promise<State>;
  Disconnect(): Promise<void>;
  Send(text: string): Promise<Message>;
  ManualConnect(addr: string): Promise<void>;
  CopyPeerInfo(): Promise<void>;
  Status(): Promise<State>;
}

interface RuntimeAPI {
  EventsOn(name: string, callback: (data: unknown) => void): () => void;
}

declare global {
  interface Window {
    go?: {
      main: {
        App: AppAPI;
      };
    };
    runtime?: RuntimeAPI;
  }
}

let mockState: State = {
  connected: false,
  status: 'disconnected',
  peer_count: 0,
  room_peer_count: 0,
  peer_info: { id: '', addresses: [] },
  messages: [],
};

const mockAPI: AppAPI = {
  async Connect(req) {
    mockState = {
      connected: true,
      status: 'connected',
      peer_count: 0,
      room_peer_count: 0,
      peer_info: {
        id: 'dev-peer',
        addresses: [`/ip4/127.0.0.1/tcp/0/p2p/dev-peer`],
      },
      messages: [],
    };
    void req;
    return mockState;
  },
  async Disconnect() {
    mockState = {
      connected: false,
      status: 'disconnected',
      peer_count: 0,
      room_peer_count: 0,
      peer_info: { id: '', addresses: [] },
      messages: [],
    };
  },
  async Send(text) {
    const message: Message = {
      id: `dev-${Date.now()}`,
      room: 'local',
      text,
      sender_id: 'dev-peer',
      sender_username: 'alice',
      sent_at: new Date().toISOString(),
      version: 1,
    };
    mockState.messages = [...(mockState.messages ?? []), message];
    return message;
  },
  async ManualConnect() {
    mockState = { ...mockState, peer_count: 1, room_peer_count: 1 };
  },
  async CopyPeerInfo() {},
  async Status() {
    return mockState;
  },
};

export const api = () => window.go?.main?.App ?? mockAPI;

export function onEvent<T>(name: string, callback: (data: T) => void): () => void {
  return window.runtime?.EventsOn(name, (data) => callback(data as T)) ?? (() => {});
}
