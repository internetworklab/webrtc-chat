package framing

import (
	pkgconnreg "example.com/webrtcserver/pkg/connreg"
)

type MessagePayload struct {
	NodeId   string                      `json:"node_id,omitempty"`
	Register *pkgconnreg.RegisterPayload `json:"register,omitempty"`
	Echo     *pkgconnreg.EchoPayload     `json:"echo,omitempty"`

	// emit when some node changes its name
	Rename *pkgconnreg.RenamePayload `json:"rename,omitempty"`

	// emit when node goes online
	Online *pkgconnreg.NodeGoesOnline `json:"online,omitempty"`

	// SDP and ICE offers are concepts from WebRTC
	SDPOffer *pkgconnreg.SDPOfferPayload `json:"sdp_offer,omitempty"`
	ICEOffer *pkgconnreg.ICEOfferPayload `json:"ice_offer,omitempty"`

	AttributesAnnouncement *pkgconnreg.AttributesAnnouncementPayload `json:"attributes_announcement,omitempty"`
}
