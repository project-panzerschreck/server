package ramalama

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
)

type ServeArgs struct {
	Model    string // required
	Port     int
	Alias    *string
	RpcNodes []RpcNode
}

type RpcNode struct {
	Host string
}

func (c Ramalama) ServeCommand(ctx context.Context, args ServeArgs) *exec.Cmd {
	cliArgs := slices.Concat(c.Command[1:], []string{})

	nodes := ""
	sep := ""
	for _, node := range args.RpcNodes {
		nodes += sep + node.Host
		sep = ","
	}

	cliArgs = append(cliArgs, "-ngl", "99", "--rpc", nodes)

	if args.Alias != nil {
		cliArgs = append(cliArgs, "-n", *args.Alias)
	}

	cliArgs = append(cliArgs, "--port", fmt.Sprint(args.Port))

	cliArgs = append(cliArgs, "--model", args.Model)

	return exec.CommandContext(ctx, c.Command[0], cliArgs...)
}
