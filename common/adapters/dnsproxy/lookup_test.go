package dnsproxy

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
	"github.com/miekg/dns"
)

func cname(owner, target string) dns.RR {
	return &dns.CNAME{Hdr: dns.RR_Header{Name: owner, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60}, Target: target}
}

func TestLookupCNAMEAndAnswerOwnership(t *testing.T) {
	for _, inline := range []bool{false, true} {
		t.Run(fmt.Sprint(inline), func(t *testing.T) {
			endpoint := startClassic(t, schema.DNSUDP, nil, func(query *dns.Msg) *dns.Msg {
				answer := testAnswer(query)
				if query.Question[0].Name == "example.test." {
					answer.Answer[0].Header().Name = "unrelated.test."
					answer.Answer = append(answer.Answer, cname("example.test.", "alias.test."))
					if inline {
						alias := query.Copy()
						alias.Question[0].Name = "alias.test."
						answer.Answer = append(answer.Answer, testAnswer(alias).Answer...)
					}
				}
				return answer
			})
			addresses, err := Lookup(testMeta(schema.DNSUDP, endpoint), "example.test")
			if err != nil || len(addresses) != 2 || addresses[0].String() != "192.0.2.7" || addresses[1].String() != "2001:db8::7" {
				t.Fatalf("lookup: %v %v", addresses, err)
			}
		})
	}
}

func TestLookupRejectsCNAMECyclesLimitsConflictsAndPartialResults(t *testing.T) {
	for _, mode := range []string{"cycle", "limit", "conflict", "mixed", "partial", "inconsistent"} {
		t.Run(mode, func(t *testing.T) {
			var count atomic.Int32
			endpoint := startClassic(t, schema.DNSUDP, nil, func(query *dns.Msg) *dns.Msg {
				answer := testAnswer(query)
				owner := query.Question[0].Name
				switch mode {
				case "cycle":
					answer.Answer = []dns.RR{cname(owner, owner)}
				case "limit":
					answer.Answer = []dns.RR{cname(owner, fmt.Sprintf("alias%d.test.", count.Add(1)))}
				case "conflict":
					answer.Answer = []dns.RR{cname(owner, "first.test."), cname(owner, "second.test.")}
				case "mixed":
					answer.Answer = append(answer.Answer, cname(owner, "alias.test."))
				case "partial":
					if query.Question[0].Qtype == dns.TypeAAAA {
						answer.Rcode = dns.RcodeServerFailure
					}
				case "inconsistent":
					if query.Question[0].Qtype == dns.TypeAAAA {
						answer.Rcode, answer.Answer = dns.RcodeNameError, nil
					}
				}
				return answer
			})
			addresses, err := Lookup(testMeta(schema.DNSUDP, endpoint), "example.test")
			if err == nil || len(addresses) != 0 {
				t.Fatalf("accepted %s: %v %v", mode, addresses, err)
			}
		})
	}
}

func TestLookupNXDOMAINAndNODATA(t *testing.T) {
	for _, rcode := range []int{dns.RcodeNameError, dns.RcodeSuccess} {
		var count atomic.Int32
		endpoint := startClassic(t, schema.DNSUDP, nil, func(query *dns.Msg) *dns.Msg {
			count.Add(1)
			return new(dns.Msg).SetRcode(query, rcode)
		})
		addresses, err := Lookup(testMeta(schema.DNSUDP, endpoint), "example.test")
		if err != nil || len(addresses) != 0 {
			t.Fatalf("negative answer: %v %v", addresses, err)
		}
		if rcode == dns.RcodeNameError && count.Load() != 1 {
			t.Fatal("continued after NXDOMAIN")
		}
		if rcode == dns.RcodeSuccess && count.Load() != 2 {
			t.Fatal("did not query both families")
		}
	}
}

func TestBootstrapAddressPriority(t *testing.T) {
	endpoint := startClassic(t, schema.DNSTCP, nil, testAnswer)
	meta := bootstrapMeta(schema.DNSTCP, endpoint)
	meta.ResolversByPriority[0].BootstrapIPs = []string{"127.0.0.2", "127.0.0.1"}
	if _, err := testResolver(t, meta).Exchange(context.Background(), testQuery()); err != nil {
		t.Fatal(err)
	}
}
