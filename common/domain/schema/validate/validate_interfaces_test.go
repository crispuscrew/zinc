package validate

import (
	"testing"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

func TestNetworkInterfaces(test *testing.T) {
	for _, interfaces := range [][]schema.NetworkInterface{
		{{}}, {{ID: "../bad"}}, {{ID: " wan"}}, {{ID: "wan"}, {ID: "wan"}},
		{{ID: "wan", MacAddress: "02:aa:bb:cc:dd:ee"}, {ID: "lan", MacAddress: "02:AA:BB:CC:DD:EE"}},
	} {
		cfg := baseCfg()
		cfg.NetworkMeta.Interfaces = interfaces
		requireError(test, cfg, "NetworkMeta.Interfaces")
	}
	for _, address := range []string{"02:1a:2b:3c:4d", "02:1a:2b:3c:4d:zz", "2:1a:2b:3c:4d:5e", "01:1a:2b:3c:4d:5e", "00:00:00:00:00:00", "ff:ff:ff:ff:ff:ff", "02-1a-2b-3c-4d-5e", "02:1a:2b:3c:4d:5e,mac=other"} {
		cfg := baseCfg()
		cfg.NetworkMeta.Interfaces = []schema.NetworkInterface{{ID: "wan", MacAddress: address}}
		requireError(test, cfg, "MacAddress")
	}
	for _, address := range []string{"", "02:1a:2b:3c:4d:5e", "00:1A:2B:3C:4D:5E"} {
		cfg := baseVM()
		cfg.NetworkMeta.Interfaces = []schema.NetworkInterface{{ID: "wan", MacAddress: address}, {ID: "lan"}}
		if err := Validate(cfg); err != nil {
			test.Fatal(err)
		}
	}
}
