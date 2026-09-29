package dnsproxy

import (
	"fmt"
	"strings"

	"github.com/miekg/dns"
)

func validateQuery(query *dns.Msg) error {
	if query == nil || query.Response || query.Opcode != dns.OpcodeQuery || len(query.Question) != 1 {
		return fmt.Errorf("DNS requires one standard query question")
	}
	question := query.Question[0]
	if _, valid := dns.IsDomainName(question.Name); !valid || question.Qclass != dns.ClassINET {
		return fmt.Errorf("invalid DNS question")
	}
	if question.Qtype == dns.TypeAXFR || question.Qtype == dns.TypeIXFR || query.IsTsig() != nil {
		return fmt.Errorf("DNS transfers and TSIG are unsupported")
	}
	encoded, err := query.Pack()
	if err != nil {
		return err
	}
	if len(encoded) > dns.MaxMsgSize {
		return fmt.Errorf("DNS message too large")
	}
	return nil
}

func validateAnswer(query, answer *dns.Msg) error {
	if answer == nil || !answer.Response || answer.Id != query.Id || answer.Opcode != query.Opcode || len(answer.Question) != 1 {
		return fmt.Errorf("DNS response header mismatch")
	}
	wanted, actual := query.Question[0], answer.Question[0]
	if !strings.EqualFold(wanted.Name, actual.Name) || wanted.Qtype != actual.Qtype || wanted.Qclass != actual.Qclass {
		return fmt.Errorf("DNS response question mismatch")
	}
	if answer.Truncated {
		return fmt.Errorf("truncated DNS response; configure an explicit fallback")
	}
	if answer.Rcode != dns.RcodeSuccess && answer.Rcode != dns.RcodeNameError {
		return fmt.Errorf("DNS response rcode %d", answer.Rcode)
	}
	return nil
}
