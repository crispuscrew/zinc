package dnsproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/crispuscrew/zinc/common/domain/schema"
)

type listenFlags []string

func (values *listenFlags) String() string { return strings.Join(*values, ",") }
func (values *listenFlags) Set(value string) error {
	*values = append(*values, value)
	return nil
}

// Run is a CLI helper for a runner's dns-proxy subcommand. The caller owns signals.
// Flags: --config <DNSMeta JSON>, --control-socket <path>, repeated --listen <IP[:port]>.
func Run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("dns-proxy", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "DNSMeta JSON file")
	control := flags.String("control-socket", "", "private Unix status socket")
	var addresses listenFlags
	flags.Var(&addresses, "listen", "explicit UDP/TCP listener IP or IP:port (repeatable)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *configPath == "" || *control == "" || len(addresses) == 0 {
		return fmt.Errorf("usage: dns-proxy --config DNS.json --control-socket PATH --listen IP[:port]")
	}
	file, err := os.Open(*configPath)
	if err != nil {
		return err
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, 128*1024+1))
	if err != nil {
		return err
	}
	if len(encoded) > 128*1024 {
		return fmt.Errorf("oversized DNS configuration")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var meta schema.DNSMeta
	if err := decoder.Decode(&meta); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing DNS configuration")
	}
	return Serve(ctx, meta, addresses, *control)
}
