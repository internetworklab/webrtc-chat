// type ConnectionAttributes map[string]string
export type ConnectionAttributes = Record<string, string>;

// type RenamePayload struct {
// 	NewNodeName    string `json:"new_node_name"`
// 	OriginNodeName string `json:"origin_node_name,omitempty"`
// }
export interface RenamePayload {
  new_node_name: string;
  origin_node_name?: string;
}

// type MessagePayload struct {
// 	NodeId   string                      `json:"node_id,omitempty"`
// 	Register *pkgconnreg.RegisterPayload `json:"register,omitempty"`
// 	Echo     *pkgconnreg.EchoPayload     `json:"echo,omitempty"`

// 	// emit when some node changes its name
// 	Rename *pkgconnreg.RenamePayload `json:"rename,omitempty"`

// 	// emit when node goes online
// 	Online *pkgconnreg.NodeGoesOnline `json:"online,omitempty"`

// 	// SDP and ICE offers are concepts from WebRTC
// 	SDPOffer *pkgconnreg.SDPOfferPayload `json:"sdp_offer,omitempty"`
// 	ICEOffer *pkgconnreg.ICEOfferPayload `json:"ice_offer,omitempty"`

// 	AttributesAnnouncement *pkgconnreg.AttributesAnnouncementPayload `json:"attributes_announcement,omitempty"`
// }

export interface MessagePayload {
  node_id?: string;
  // emits when register success
  register?: RegisterPayload;
  echo?: EchoPayload;
  online?: any;
  rename?: RenamePayload;

  // SDP and ICE offers are concepts from WebRTC
  sdp_offer?: SDPOfferPayload;
  ice_offer?: ICEOfferPayload;

  attributes_announcement?: AttributesAnnouncementPayload;
}

// type RegisterPayload struct {
//  	NodeName string  `json:"node_name"`
//  	Token    *string `json:"token,omitempty"`
// }
export interface RegisterPayload {
  node_name: string;
  token?: string;
}

// type EchoDirection string
// const (
//  	EchoDirectionC2S EchoDirection = "ping"
//  	EchoDirectionS2C EchoDirection = "pong"
// )
export type EchoDirection = "ping" | "pong";

export const EchoDirectionC2S = "ping" as const;
export const EchoDirectionS2C = "pong" as const;

// type EchoPayload struct {
//  	Direction       EchoDirection `json:"direction"`
//  	CorrelationID   string        `json:"correlation_id"`
//  	ServerTimestamp uint64        `json:"server_timestamp"`
//  	Timestamp       uint64        `json:"timestamp"`
//  	SeqID           uint64        `json:"seq_id"`
// }
export interface EchoPayload {
  direction: EchoDirection;
  correlation_id: string;
  server_timestamp: number;
  timestamp: number;
  seq_id: number;
}

// type AttributesAnnouncementPayload struct {
//  	Attributes  ConnectionAttributes `json:"attributes,omitempty"`
//  	Withdrawals []string             `json:"withdrawals,omitempty"`
// }
export interface AttributesAnnouncementPayload {
  attributes?: ConnectionAttributes;
  withdrawals?: string[];
}

// type AuthenticationType string
// const (
// 	AuthenticationTypeJWT  AuthenticationType = "jwt"
// 	AuthenticationTypeMTLS AuthenticationType = "mtls"
// )
export type AuthenticationType = "jwt" | "mtls";

export const AuthenticationTypeJWT = "jwt" as const;
export const AuthenticationTypeMTLS = "mtls" as const;

// type ConnRegistryData struct {
// 	NodeName       *string              `json:"node_name,omitempty"`
// 	ConnectedAt    uint64               `json:"connected_at"`
// 	RegisteredAt   *uint64              `json:"registered_at,omitempty"`
// 	LastHeartbeat  *uint64              `json:"last_heartbeat,omitempty"`
// 	Attributes     ConnectionAttributes `json:"attributes,omitempty"`
// 	QUICConn       *quicGo.Conn         `json:"-"`
// 	Claims         jwt.MapClaims        `json:"-"`
// 	Authentication AuthenticationType   `json:"authentication"`
// }
export interface ConnRegistryData {
  node_name?: string;
  connected_at: number;
  registered_at?: number;
  last_heartbeat?: number;
  attributes?: ConnectionAttributes;
  authentication: AuthenticationType;
}

export type ConnEntry = {
  node_id: string;
  registered_at: number;
  entry: ConnRegistryData;
  rtt?: number;
};

export enum OfferType {
  Offer = "offer",
  Answer = "answer",
}

export interface SDPOfferPayload {
  type: OfferType;
  offer_json: string;
  from_node_id: string;
  to_node_id: string;
}

export interface ICEOfferPayload {
  offer_json: string;
  from_node_id: string;
  to_node_id: string;
}

export type ChatMessageFileLoading = {
  // real number in [0, 1], is the ratio of the actual receiver acknowledged bytes to the total bytes of the file
  progress: number;

  // onopen event of DC will trigger this to be true
  started?: boolean;

  // this will be set to true when all octets of the file are acknowledged by the receiver
  completed?: boolean;
};

export type ChatMessageFile = {
  url?: string;
  name?: string;
  size?: number;
  type?: string;
  loading?: ChatMessageFileLoading;

  // the identifier of the DC that actually transmit the file data,
  // file senders sends this to the file receiver in advance before it creates the file transfer DC.
  dcId?: string;
};

export enum ChatMessagePingDirection {
  Ping = "ping",
  Pong = "pong",
}

export type ChatMessagePing = {
  direction: ChatMessagePingDirection;
  seq: number;
};

export type ChatMessageAmend = {
  messageId: string;

  // this should be the output of JSON.stringify(newMessageObject) where newMessageObject is an object of type ChatMessage
  newMessageJSON: string;
};

export type ChatMessage = {
  // message uuid, globally unique, to prevent a message from being queued multiple times.
  messageId: string;
  fromNodeId: string;
  toNodeId: string;
  image?: ChatMessageFile;
  video?: ChatMessageFile;
  file?: ChatMessageFile;
  ping?: ChatMessagePing;
  amend?: ChatMessageAmend;
  message: string;
  messageMIME?: string;
  timestamp: number;
};

export type ConnTrackStatusEntry = {
  // todo
  disconnected?: boolean;
  connecting?: boolean;
  messages?: ChatMessage[];
  rtt?: number;
};

// key is the node_id of remote peer
export type ConnTrackStatus = Record<string, ConnTrackStatusEntry>;

export enum PredefinedDCLabel {
  Chat = "chat",
  File = "file",
  Ping = "ping",
  // todo: ping, image, video, etc.
}

export type PingStateRef = {
  seq: number;
  timer?: NodeJS.Timeout;
  txMap: Record<number, number>;
};

export type ConnTrackEntry = {
  peerConnection: RTCPeerConnection;
  remoteOffers: RTCSessionDescriptionInit[];
  queuedICEOffers: RTCIceCandidateInit[];

  // this is the chat DC
  dataChannel?: RTCDataChannel | null;

  // key is the value of RTCDataChannel.id, all file DCs shared label PredefinedDCLabel.File
  fileDataChannels?: Record<string, RTCDataChannel>;

  pingSeqRef?: PingStateRef;
};
