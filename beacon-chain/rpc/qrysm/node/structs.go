package node

type AddrRequest struct {
	Addr string `json:"addr"`
}

type PeersResponse struct {
	Peers []*Peer `json:"Peers"`
}

type Peer struct {
	PeerID             string `json:"peer_id"`
	Qnr                string `json:"qnr"`
	LastSeenP2PAddress string `json:"last_seen_p2p_address"`
	State              string `json:"state"`
	Direction          string `json:"direction"`
}
