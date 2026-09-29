package dnsproxy

import (
	"fmt"

	"github.com/miekg/dns"
)

const paddingBlock = 128 // RFC 8467 query padding policy.

func prepareQUIC(query *dns.Msg) error {
	if query.IsEdns0() == nil {
		query.SetEdns0(dns.MaxMsgSize, false)
	}
	option := query.IsEdns0()
	retained := option.Option[:0]
	for _, value := range option.Option {
		if value.Option() != dns.EDNS0PADDING && value.Option() != dns.EDNS0TCPKEEPALIVE {
			retained = append(retained, value)
		}
	}
	padding := new(dns.EDNS0_PADDING)
	option.Option = append(retained, padding)
	encoded, err := query.Pack()
	if err != nil {
		return err
	}
	length := (paddingBlock - len(encoded)%paddingBlock) % paddingBlock
	if len(encoded)+length > dns.MaxMsgSize {
		return fmt.Errorf("DNS QUIC query exceeds padded message limit")
	}
	padding.Padding = make([]byte, length)
	return nil
}

func checkQUICOptions(answer *dns.Msg) error {
	if option := answer.IsEdns0(); option != nil {
		for _, value := range option.Option {
			if value.Option() == dns.EDNS0TCPKEEPALIVE {
				return fmt.Errorf("TCP keepalive is forbidden over DNS QUIC")
			}
		}
	}
	return nil
}

func restoreOptions(query, answer *dns.Msg) {
	retained := answer.Extra[:0]
	for _, record := range answer.Extra {
		if option, valid := record.(*dns.OPT); valid {
			if query.IsEdns0() == nil {
				continue
			}
			options := option.Option[:0]
			for _, value := range option.Option {
				if value.Option() != dns.EDNS0PADDING {
					options = append(options, value)
				}
			}
			option.Option = options
		}
		retained = append(retained, record)
	}
	answer.Extra = retained
}
