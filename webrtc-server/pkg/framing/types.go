package framing

import (
	pkgconnreg "example.com/webrtcserver/pkg/connreg"
)

type MessagePayload struct {
	NodeId                 string                                    `json:"node_id,omitempty"`
	Register               *pkgconnreg.RegisterPayload               `json:"register,omitempty"`
	Echo                   *pkgconnreg.EchoPayload                   `json:"echo,omitempty"`
	AttributesAnnouncement *pkgconnreg.AttributesAnnouncementPayload `json:"attributes_announcement,omitempty"`
}
